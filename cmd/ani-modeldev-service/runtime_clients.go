package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/kfp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type runtimeClients struct {
	certificate tls.Certificate
	binding     biz.PipelineDispatchBinding
	kube        dynamic.Interface
	runs        *kfp.Client
	store       *s3.Client
	close       func()
}

func loadRuntimeClients(config *conf.ManagedRuntime) (runtimeClients, error) {
	failure := func() (runtimeClients, error) {
		return runtimeClients{}, errors.New("managed runtime materials unavailable or invalid")
	}
	bindingBytes, err := readCommandMaterial(config.BindingFile, 1<<20)
	if err != nil {
		return failure()
	}
	digest := sha256.Sum256(bindingBytes)
	if hex.EncodeToString(digest[:]) != config.BindingSha256 {
		return failure()
	}
	var binding biz.PipelineDispatchBinding
	if decodeRuntimeMaterial(bindingBytes, &binding) != nil || binding.Validate() != nil {
		return failure()
	}
	certificatePEM, err := readCommandMaterial(config.CertificateFile, 1<<20)
	if err != nil {
		return failure()
	}
	keyPEM, err := readCommandMaterial(config.PrivateKeyFile, 1<<20)
	if err != nil {
		return failure()
	}
	certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		return failure()
	}
	kubeCA, err := runtimeRoots(config.Kubernetes.CaFile)
	if err != nil {
		return failure()
	}
	_ = kubeCA // Parse now; client-go consumes the same configured CA file.
	if _, err := runtimeToken(config.Kubernetes.TokenFile); err != nil {
		return failure()
	}
	provider := mountedPipelineToken{binding: binding, file: config.Pipeline.TokenFile}
	if _, err := provider.BearerToken(context.Background(), binding.TenantID, binding.Environment); err != nil {
		return failure()
	}
	pipelineCA, err := runtimeRoots(config.Pipeline.CaFile)
	if err != nil {
		return failure()
	}
	storageCA, err := runtimeRoots(config.ObjectStorage.CaFile)
	if err != nil {
		return failure()
	}
	credentials := mountedStorageCredentials(config.ObjectStorage.CredentialsFile)
	if _, err := credentials.Retrieve(context.Background()); err != nil {
		return failure()
	}
	timeout := config.ApiTimeout.AsDuration()
	kubeConfig := &rest.Config{
		Host: config.Kubernetes.Endpoint, BearerTokenFile: config.Kubernetes.TokenFile,
		TLSClientConfig: rest.TLSClientConfig{CAFile: config.Kubernetes.CaFile}, Timeout: timeout,
		Proxy:     func(*http.Request) (*url.URL, error) { return nil, nil },
		UserAgent: "ani-modeldev-service", QPS: 5, Burst: 10,
	}
	kubeHTTP, err := rest.HTTPClientFor(kubeConfig)
	if err != nil {
		return failure()
	}
	kubeHTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	kube, err := dynamic.NewForConfigAndClient(kubeConfig, kubeHTTP)
	if err != nil {
		kubeHTTP.CloseIdleConnections()
		return failure()
	}
	runs, err := kfp.New(kfp.Config{ConnectionRef: binding.Environment.KFPConnectionRef, Endpoint: config.Pipeline.Endpoint, RootCAs: pipelineCA, Timeout: timeout}, provider)
	if err != nil {
		kubeHTTP.CloseIdleConnections()
		return failure()
	}
	storageTransport := &http.Transport{
		Proxy: nil, DialContext: (&net.Dialer{Timeout: timeout}).DialContext,
		TLSClientConfig:     &tls.Config{RootCAs: storageCA, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout, MaxConnsPerHost: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
	}
	storageHTTP := &http.Client{Transport: storageTransport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	store := s3.New(s3.Options{
		Region: config.ObjectStorage.Region, BaseEndpoint: aws.String(config.ObjectStorage.Endpoint), UsePathStyle: true,
		HTTPClient: storageHTTP, Credentials: credentials, RetryMaxAttempts: 1,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return runtimeClients{certificate: certificate, binding: binding, kube: kube, runs: runs, store: store, close: func() { kubeHTTP.CloseIdleConnections(); storageHTTP.CloseIdleConnections() }}, nil
}

type mountedPipelineToken struct {
	binding biz.PipelineDispatchBinding
	file    string
}

func (provider mountedPipelineToken) BearerToken(ctx context.Context, tenant string, environment cpup01.EnvironmentBindingSnapshot) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if tenant != provider.binding.TenantID || environment != provider.binding.Environment {
		return "", errors.New("pipeline credential binding mismatch")
	}
	return runtimeToken(provider.file)
}

var runtimeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

func runtimeToken(file string) (string, error) {
	data, err := readCommandMaterial(file, 16<<10)
	if err != nil {
		return "", errors.New("runtime credential unavailable")
	}
	token := strings.TrimSpace(string(data))
	if !runtimeTokenPattern.MatchString(token) {
		return "", errors.New("runtime credential invalid")
	}
	return token, nil
}

func mountedStorageCredentials(file string) aws.CredentialsProvider {
	return aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		if err := ctx.Err(); err != nil {
			return aws.Credentials{}, err
		}
		data, err := readCommandMaterial(file, 16<<10)
		if err != nil {
			return aws.Credentials{}, errors.New("storage credential unavailable")
		}
		var value struct {
			AccessKeyID     string `json:"access_key_id"`
			SecretAccessKey string `json:"secret_access_key"`
			SessionToken    string `json:"session_token,omitempty"`
		}
		if decodeRuntimeMaterial(data, &value) != nil || value.AccessKeyID == "" || value.SecretAccessKey == "" || strings.ContainsAny(value.AccessKeyID+value.SecretAccessKey+value.SessionToken, " \t\r\n") {
			return aws.Credentials{}, errors.New("storage credential invalid")
		}
		// Read on every SDK retrieval so mounted rotation does not require an
		// application restart. Never consult ambient AWS environment variables.
		return aws.Credentials{AccessKeyID: value.AccessKeyID, SecretAccessKey: value.SecretAccessKey, SessionToken: value.SessionToken, Source: "mounted-runtime-material"}, nil
	})
}

func runtimeRoots(file string) (*x509.CertPool, error) {
	data, err := readCommandMaterial(file, 1<<20)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		return nil, errors.New("runtime CA invalid")
	}
	return roots, nil
}

func decodeRuntimeMaterial(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("runtime material invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("runtime material invalid")
	}
	return nil
}
