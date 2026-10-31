// The one-time command is deliberately outside the service composition root.
// It is fixed to three approved old executions and never enables v3 in v4.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/accelerator"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/kfp"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/runtimeproof"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/trainer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var ids = []string{"1cce950b-d6e9-453e-aa11-fffd8846224b", "4a1a92f7-4d94-439d-a8d7-1833f38c7dbc", "eca4d47b-5911-4f0f-8b45-89c1be775925"}

const schema = "ani.modeldev.legacy-retirement.plan.v1"

type config struct {
	DatabaseURLFile       string             `json:"database_url_file"`
	ApprovalFile          string             `json:"approval_file"`
	DNSAddress            string             `json:"dns_address"`
	ControlPodUID         string             `json:"control_pod_uid"`
	ConfigMapUID          string             `json:"configmap_uid"`
	ClusterID             string             `json:"cluster_id"`
	Context               string             `json:"context"`
	APIServer             string             `json:"api_server"`
	KubeSystemUID         string             `json:"kube_system_uid"`
	Kubernetes            connection         `json:"kubernetes"`
	Pipeline              connection         `json:"pipeline"`
	LogEvidenceObservedAt time.Time          `json:"log_evidence_observed_at"`
	Governance            accelerator.Config `json:"governance"`
}
type connection struct{ Endpoint, CAFile, TokenFile string }
type item struct {
	OriginalGPURelease                           *govReleaseProof `json:",omitempty"`
	ExecutionID                                  string
	Owner                                        biz.RunAuthorityCandidate
	OwnerRevision, CloseGeneration, CommandFence uint64
	CloseReviewReason                            string
	ArchiveSHA256, ProofSHA256                   string
	OriginalStepPods                             []biz.RuntimeResource
	OriginalTrainingResources                    []biz.RuntimeResource
	LogDisposition                               []biz.LegacyTrainingLogDisposition
	Closed                                       bool
	Ready                                        bool
	Blocker                                      string
}
type plan struct {
	Schema                     string
	ObservedAt                 time.Time
	ReadOnly                   bool
	ApprovalSHA256             string
	Identity                   identity
	DatabaseName, DatabaseUser string
	Entries                    []item
	Ready                      bool
}
type identity struct{ Hostname, SSHUser, Context, APIServer, KubeSystemUID, ClusterID, NamespaceName, NamespaceUID, ControlPodUID, ConfigMapUID string }

type app struct {
	config                     config
	pool                       *pgxpool.Pool
	repo                       *lifecycle.LegacyRetirementRepository
	kube                       dynamic.Interface
	runs                       *kfp.Client
	proof                      *runtimeproof.Verifier
	training                   *trainer.Adapter
	archive                    *archiveReader
	identity                   identity
	databaseName, databaseUser string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
func run() error {
	var path, mode, planFile, planSHA, gateFile, gateSHA string
	flag.StringVar(&path, "config", "", "protected node-local configuration")
	flag.StringVar(&mode, "mode", "plan", "plan or apply")
	flag.StringVar(&planFile, "plan", "", "reviewed plan file (apply only)")
	flag.StringVar(&planSHA, "plan-sha256", "", "exact reviewed plan bytes SHA256")
	flag.StringVar(&gateFile, "stopped-admission-receipt", "", "fresh root-verified stop-admission receipt")
	flag.StringVar(&gateSHA, "stopped-admission-sha256", "", "exact receipt bytes SHA256")
	flag.Parse()
	if flag.NArg() != 0 || path == "" || (mode != "plan" && mode != "apply") {
		return errors.New("MAINTENANCE_ARGUMENT_INVALID")
	}
	raw, err := readBounded(path, 64<<10)
	if err != nil {
		return errors.New("MAINTENANCE_CONFIG_UNAVAILABLE")
	}
	var c config
	if strictJSON(raw, &c) != nil {
		return errors.New("MAINTENANCE_CONFIG_INVALID")
	}
	approval, err := readBounded(c.ApprovalFile, 64<<10)
	if err != nil || digest(approval) != biz.LegacyRetirementApprovalSHA256 {
		return errors.New("MAINTENANCE_APPROVAL_MISMATCH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	a, err := newApp(ctx, c, mode == "plan")
	if err != nil {
		return err
	}
	defer a.pool.Close()
	if mode == "plan" {
		p := a.buildPlan(ctx)
		return json.NewEncoder(os.Stdout).Encode(p)
	}
	reviewed, err := loadPlan(planFile, planSHA)
	if err != nil {
		return err
	}
	if err := checkAdmissionGate(gateFile, gateSHA, planSHA); err != nil {
		return err
	}
	if reviewed.Identity != a.identity {
		return errors.New("MAINTENANCE_IDENTITY_CHANGED")
	}
	return a.apply(ctx, reviewed, gateFile, gateSHA, planSHA)
}

func newApp(ctx context.Context, c config, readonly bool) (*app, error) {
	if c.Kubernetes.Endpoint != "https://kubernetes.default.svc:443" || c.Pipeline.Endpoint != "https://ani-kfp-entry.kubeflow.svc:8443" {
		return nil, errors.New("MAINTENANCE_ORIGINAL_ENDPOINT_MISMATCH")
	}
	if c.Context != "kubernetes-admin@ani-kcn-full" || c.APIServer != "https://lb.kubesphere.local:6443" || c.KubeSystemUID != "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24" || c.ClusterID != "f39bf6e2-f3ed-4191-ada3-caeebf86c220" || c.ControlPodUID == "" || c.ConfigMapUID == "" || c.LogEvidenceObservedAt.IsZero() || c.LogEvidenceObservedAt.After(time.Now()) {
		return nil, errors.New("MAINTENANCE_CLUSTER_IDENTITY_INVALID")
	}
	hostname, _ := os.Hostname()
	if hostname != "node1" {
		return nil, errors.New("MAINTENANCE_HOST_IDENTITY_INVALID")
	}
	if c.DNSAddress != "10.96.0.3:53" {
		return nil, errors.New("MAINTENANCE_DNS_IDENTITY_INVALID")
	}
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "udp", c.DNSAddress)
	}}
	db, err := readBounded(c.DatabaseURLFile, 16<<10)
	if err != nil {
		return nil, errors.New("MAINTENANCE_DATABASE_REFERENCE_UNAVAILABLE")
	}
	pc, err := pgxpool.ParseConfig(strings.TrimSpace(string(db)))
	if err != nil {
		return nil, errors.New("MAINTENANCE_DATABASE_CONFIGURATION_INVALID")
	}
	pc.MaxConns = 2
	pc.ConnConfig.ConnectTimeout = 10 * time.Second
	pc.ConnConfig.RuntimeParams["application_name"] = "modeldev-legacy-retirement-20261101"
	pc.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	if readonly {
		pc.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, errors.New("MAINTENANCE_DATABASE_UNAVAILABLE")
	}
	fail := func(reason string) (*app, error) { pool.Close(); return nil, errors.New(reason) }
	a := &app{config: c, pool: pool, repo: lifecycle.NewLegacyRetirement(pool)}
	if err := pool.QueryRow(ctx, "SELECT current_database(),current_user").Scan(&a.databaseName, &a.databaseUser); err != nil || a.databaseName != "ani_modeldev_v21" {
		return fail("MAINTENANCE_DATABASE_IDENTITY_MISMATCH")
	}
	kc := &rest.Config{Host: c.Kubernetes.Endpoint, BearerTokenFile: c.Kubernetes.TokenFile, TLSClientConfig: rest.TLSClientConfig{CAFile: c.Kubernetes.CAFile}, Timeout: 20 * time.Second, QPS: 20, Burst: 40, UserAgent: "ani-modeldev-legacy-retirement/20261101"}
	if err := validExistingToken(c.Kubernetes.TokenFile, c.ControlPodUID, false); err != nil {
		return fail("MAINTENANCE_API_TOKEN_IDENTITY_INVALID")
	}
	if readonly {
		kc.WrapTransport = func(t http.RoundTripper) http.RoundTripper { return readOnlyTransport{t} }
	}
	a.kube, err = dynamic.NewForConfig(kc)
	if err != nil {
		return fail("MAINTENANCE_KUBE_CONFIGURATION_INVALID")
	}
	a.identity, err = a.readIdentity(ctx)
	if err != nil {
		return fail("MAINTENANCE_CURRENT_CLUSTER_IDENTITY_MISMATCH")
	}
	scope, _ := biz.LookupLegacyRetirementScope(ids[0])
	_, _, dispatch, _, _, err := a.repo.GetLegacyRetirementRecord(ctx, scope.TenantID, ids[0])
	if err != nil {
		return fail("MAINTENANCE_ORIGINAL_AGGREGATE_UNREADABLE")
	}
	roots, err := readCA(c.Pipeline.CAFile)
	if err != nil {
		return fail("MAINTENANCE_PIPELINE_CA_INVALID")
	}
	tokens := fixedToken{file: c.Pipeline.TokenFile, podUID: c.ControlPodUID}
	a.runs, err = kfp.New(kfp.Config{Endpoint: c.Pipeline.Endpoint, ConnectionRef: dispatch.Plan.Environment.KFPConnectionRef, RootCAs: roots, Timeout: 20 * time.Second}, tokens)
	if err != nil {
		return fail("MAINTENANCE_PIPELINE_CONFIGURATION_INVALID")
	}
	a.archive, err = newArchiveReader(c.Pipeline, tokens)
	if err != nil {
		return fail("MAINTENANCE_ARCHIVE_CONFIGURATION_INVALID")
	}
	a.training = trainer.New(a.kube)
	a.proof = runtimeproof.New(a.kube, a.runs, nil, submission.New(pool))
	return a, nil
}

type readOnlyTransport struct{ inner http.RoundTripper }

func (t readOnlyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return nil, errors.New("MAINTENANCE_PLAN_MUTATION_REFUSED")
	}
	return t.inner.RoundTrip(r)
}
func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("bounded file invalid")
	}
	return data, nil
}
func strictJSON(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("extra JSON")
	}
	return nil
}
func digest(raw []byte) string { s := sha256.Sum256(raw); return hex.EncodeToString(s[:]) }

func isSHA256(s string) bool {
	raw, err := hex.DecodeString(s)
	return err == nil && len(raw) == 32 && strings.ToLower(s) == s
}
