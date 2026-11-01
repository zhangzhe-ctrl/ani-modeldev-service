package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

type fixedToken struct{ file, podUID string }

func (t fixedToken) BearerToken(ctx context.Context, tenant string, environment cpup01.EnvironmentBindingSnapshot) (string, error) {
	scope, _ := biz.LookupLegacyRetirementScope(ids[0])
	if ctx.Err() != nil || tenant != scope.TenantID || environment.ClusterID != scope.ClusterID || environment.NamespaceName != scope.NamespaceName || environment.NamespaceUID != scope.NamespaceUID {
		return "", biz.ErrRuntimeConflict
	}
	if err := validExistingToken(t.file, t.podUID, true); err != nil {
		return "", err
	}
	raw, err := readBounded(t.file, 16384)
	return strings.TrimSpace(string(raw)), err
}
func validExistingToken(file, podUID string, kfp bool) error {
	raw, err := readBounded(file, 16384)
	if err != nil {
		return biz.ErrRuntimeNotReady
	}
	fields := strings.Split(strings.TrimSpace(string(raw)), ".")
	if len(fields) != 3 {
		return biz.ErrRuntimeConflict
	}
	b, err := base64.RawURLEncoding.DecodeString(fields[1])
	if err != nil {
		return biz.ErrRuntimeConflict
	}
	var c struct {
		Sub        string   `json:"sub"`
		Aud        []string `json:"aud"`
		Exp        int64    `json:"exp"`
		Kubernetes struct {
			Pod struct {
				UID string `json:"uid"`
			} `json:"pod"`
		} `json:"kubernetes.io"`
	}
	if json.Unmarshal(b, &c) != nil || c.Sub != "system:serviceaccount:ani-system:modeldev-control" || c.Kubernetes.Pod.UID != podUID || c.Exp <= time.Now().Unix()+10 {
		return biz.ErrRuntimeConflict
	}
	if kfp && (len(c.Aud) != 1 || c.Aud[0] != "pipelines.kubeflow.org") {
		return biz.ErrRuntimeConflict
	}
	return nil
}
func readCA(file string) (*x509.CertPool, error) {
	raw, err := readBounded(file, 1<<20)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, errors.New("CA invalid")
	}
	return pool, nil
}

type archiveReader struct {
	endpoint string
	http     *http.Client
	tokens   fixedToken
}

func newArchiveReader(c connection, t fixedToken) (*archiveReader, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, biz.ErrRuntimeConflict
	}
	roots, err := readCA(c.CAFile)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, DisableCompression: true, MaxConnsPerHost: 2}
	return &archiveReader{endpoint: strings.TrimSuffix(c.Endpoint, "/"), tokens: t, http: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (r *archiveReader) get(ctx context.Context, scope biz.LegacyRetirementScope, environment cpup01.EnvironmentBindingSnapshot) ([]byte, error) {
	token, err := r.tokens.BearerToken(ctx, scope.TenantID, environment)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.endpoint+"/apis/v1beta1/runs/"+scope.RunID, nil)
	if err != nil {
		return nil, biz.ErrRuntimeConflict
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	response, err := r.http.Do(request)
	if err != nil {
		return nil, biz.ErrRuntimeNotReady
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || response.StatusCode != 200 {
		return nil, biz.ErrRuntimeNotReady
	}
	var detail struct {
		Run struct {
			ID         string `json:"id"`
			References []struct {
				Key          struct{ Type, ID string } `json:"key"`
				Relationship string                    `json:"relationship"`
			} `json:"resource_references"`
		} `json:"run"`
		Runtime struct {
			Workflow string `json:"workflow_manifest"`
		} `json:"pipeline_runtime"`
	}
	if json.Unmarshal(raw, &detail) != nil || detail.Run.ID != scope.RunID || digest([]byte(detail.Runtime.Workflow)) != scope.ArchiveSHA256 {
		return nil, biz.ErrRuntimeConflict
	}
	ns := 0
	for _, ref := range detail.Run.References {
		if ref.Key.Type == "NAMESPACE" {
			ns++
			if ref.Key.ID != scope.NamespaceName {
				return nil, biz.ErrRuntimeConflict
			}
		}
	}
	if ns != 1 {
		return nil, biz.ErrRuntimeConflict
	}
	return []byte(detail.Runtime.Workflow), nil
}

// The SSH entry identity is checked using the user's existing kubectl identity.
// Business Kube/KFP reads and mutations always use the original control token.
func (a *app) readIdentity(ctx context.Context) (identity, error) {
	get := func(args ...string) ([]byte, error) {
		bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		command := exec.CommandContext(bounded, "kubectl", append([]string{"--request-timeout=15s"}, args...)...)
		raw, err := command.Output()
		if err != nil || len(raw) > 2<<20 {
			return nil, biz.ErrRuntimeNotReady
		}
		return raw, nil
	}
	contextRaw, err := get("config", "current-context")
	if err != nil || strings.TrimSpace(string(contextRaw)) != a.config.Context {
		return identity{}, biz.ErrRuntimeConflict
	}
	raw, err := get("config", "view", "--minify", "-o", "json")
	if err != nil {
		return identity{}, err
	}
	var view struct {
		Clusters []struct {
			Cluster struct {
				Server string `json:"server"`
			} `json:"cluster"`
		} `json:"clusters"`
	}
	if json.Unmarshal(raw, &view) != nil || len(view.Clusters) != 1 || view.Clusters[0].Cluster.Server != a.config.APIServer {
		return identity{}, biz.ErrRuntimeConflict
	}
	object := func(args ...string) (map[string]any, error) {
		raw, err := get(append(args, "-o", "json")...)
		if err != nil {
			return nil, err
		}
		var obj map[string]any
		if json.Unmarshal(raw, &obj) != nil {
			return nil, biz.ErrRuntimeConflict
		}
		return obj, nil
	}
	uid := func(obj map[string]any) string {
		meta, _ := obj["metadata"].(map[string]any)
		id, _ := meta["uid"].(string)
		return id
	}
	ks, err := object("get", "ns", "kube-system")
	if err != nil || uid(ks) != a.config.KubeSystemUID {
		return identity{}, biz.ErrRuntimeConflict
	}
	apiService, err := object("-n", "default", "get", "svc", "kubernetes")
	if err != nil || !validOriginalAPIService(apiService) {
		return identity{}, biz.ErrRuntimeConflict
	}
	scope, _ := biz.LookupLegacyRetirementScope(ids[0])
	ns, err := object("get", "ns", scope.NamespaceName)
	if err != nil || uid(ns) != scope.NamespaceUID {
		return identity{}, biz.ErrRuntimeConflict
	}
	control, err := object("-n", "ani-system", "get", "pod", "ani-modeldev-6995bd77c4-z2gxk")
	if err != nil || uid(control) != a.config.ControlPodUID {
		return identity{}, biz.ErrRuntimeConflict
	}
	spec, _ := control["spec"].(map[string]any)
	if spec["serviceAccountName"] != "modeldev-control" || spec["nodeName"] != "node1" {
		return identity{}, biz.ErrRuntimeConflict
	}
	cm, err := object("-n", "ani-system", "get", "cm", "modeldev-trainimg-main-20261029b")
	if err != nil || uid(cm) != a.config.ConfigMapUID {
		return identity{}, biz.ErrRuntimeConflict
	}
	secret, err := object("-n", "ani-system", "get", "secret", "ani-resource-private")
	if err != nil {
		return identity{}, err
	}
	data, _ := secret["data"].(map[string]any)
	encoded, _ := data["config.yaml"].(string)
	private, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return identity{}, biz.ErrRuntimeConflict
	}
	re := regexp.MustCompile(`(?m)^\s*cluster_id:\s*["']?([a-f0-9-]{36})`)
	match := re.FindSubmatch(private)
	if len(match) != 2 || string(match[1]) != a.config.ClusterID {
		return identity{}, biz.ErrRuntimeConflict
	}
	hostname, _ := os.Hostname()
	return identity{Hostname: hostname, SSHUser: "kubercloud", Context: a.config.Context, APIServer: a.config.APIServer, KubeSystemUID: a.config.KubeSystemUID, ClusterID: a.config.ClusterID, NamespaceName: scope.NamespaceName, NamespaceUID: scope.NamespaceUID, ControlPodUID: a.config.ControlPodUID, ConfigMapUID: a.config.ConfigMapUID, APIServiceUID: originalAPIServiceUID, APIServiceIP: originalAPIServiceIP}, nil
}
