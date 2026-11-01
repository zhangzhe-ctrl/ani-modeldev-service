// originalabe is a single-execution plan/absence observer. No listener, worker,
// new retry, Kubernetes write or KFP client is installed.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	owner "github.com/zhangzhe-ctrl/ani-modeldev-service/scripts/legacy-modeldev-data-cleanup/originalabe"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type config struct {
	Schema         string `json:"schema"`
	DSNFile        string `json:"dsn_file"`
	CAFile         string `json:"ca_file"`
	TokenFile      string `json:"token_file"`
	Endpoint       string `json:"endpoint"`
	ServerName     string `json:"server_name"`
	DialAddress    string `json:"dial_address"`
	IdentityFile   string `json:"identity_file"`
	IdentitySHA256 string `json:"identity_sha256"`
}
type identity struct {
	PostgreSQLServiceUID   string    `json:"postgresql_service_uid"`
	PostgreSQLServiceVIP   string    `json:"postgresql_service_vip"`
	PostgreSQLServicePort  int       `json:"postgresql_service_port"`
	ObservedAt             time.Time `json:"observed_at"`
	SSH                    string    `json:"ssh"`
	Host                   string    `json:"host"`
	Context                string    `json:"context"`
	APIServer              string    `json:"apiserver"`
	ClusterID              string    `json:"cluster_id"`
	KubeSystemUID          string    `json:"kube_system_uid"`
	NamespaceUID           string    `json:"namespace_uid"`
	ServiceAccountUID      string    `json:"service_account_uid"`
	TransportPodUID        string    `json:"transport_pod_uid"`
	APIServiceUID          string    `json:"api_service_uid"`
	APIServiceVIP          string    `json:"api_service_vip"`
	APIServicePort         int       `json:"api_service_port"`
	Database               string    `json:"database"`
	Role                   string    `json:"role"`
	OldWorkerReplicas      int       `json:"old_worker_replicas"`
	OldWorkerPods          int       `json:"old_worker_pods"`
	GovernanceCreateLeases int       `json:"governance_create_leases"`
}

func regular(path string, max int64) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("PRIVATE_FILE_READ_FAILED")
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() > max {
		return nil, errors.New("PRIVATE_FILE_INVALID")
	}
	// O_NOFOLLOW is not needed for immutable pod source: only copied, private
	// ordinary files are accepted; refuse the path itself being a symlink.
	l, e := os.Lstat(path)
	if e != nil || l.Mode()&os.ModeSymlink != 0 || !os.SameFile(st, l) {
		return nil, errors.New("PRIVATE_FILE_INVALID")
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}
func decode(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return errors.New("STRICT_JSON_INVALID")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("STRICT_JSON_TRAILING")
	}
	return nil
}

type getOnly struct{ base http.RoundTripper }

func (t getOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet || strings.Contains(r.URL.Path, "/log") {
		return nil, errors.New("KUBERNETES_MUTATION_OR_LOG_READ_FORBIDDEN")
	}
	return t.base.RoundTrip(r)
}
func client(ctx context.Context, c config, id identity) (dynamic.Interface, error) {
	if c.Endpoint != "https://kubernetes.default.svc:443" || c.ServerName != "kubernetes.default.svc" || c.DialAddress != net.JoinHostPort(id.APIServiceVIP, "443") {
		return nil, errors.New("ORIGINAL_API_TRANSPORT_INVALID")
	}
	ca, e := regular(c.CAFile, 1<<20)
	if e != nil {
		return nil, e
	}
	token, e := regular(c.TokenFile, 1<<18)
	if e != nil {
		return nil, e
	}
	parts := strings.Split(strings.TrimSpace(string(token)), ".")
	if len(parts) != 3 {
		return nil, errors.New("ORIGINAL_PROJECTED_TOKEN_INVALID")
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return nil, errors.New("ORIGINAL_PROJECTED_TOKEN_INVALID")
	}
	var claim struct {
		Sub        string `json:"sub"`
		Exp        int64  `json:"exp"`
		Kubernetes struct {
			Namespace string `json:"namespace"`
			Pod       struct {
				UID string `json:"uid"`
			} `json:"pod"`
			ServiceAccount struct {
				UID string `json:"uid"`
			} `json:"serviceaccount"`
		} `json:"kubernetes.io"`
	}
	if json.Unmarshal(raw, &claim) != nil || claim.Sub != "system:serviceaccount:ani-system:modeldev-control" || claim.Kubernetes.Namespace != "ani-system" || claim.Kubernetes.ServiceAccount.UID != id.ServiceAccountUID || claim.Kubernetes.Pod.UID != id.TransportPodUID || claim.Exp < time.Now().Add(time.Minute).Unix() {
		return nil, errors.New("ORIGINAL_PROJECTED_TOKEN_IDENTITY_INVALID")
	}
	rc := &rest.Config{Host: c.Endpoint, BearerToken: string(bytes.TrimSpace(token)), TLSClientConfig: rest.TLSClientConfig{CAData: ca, ServerName: c.ServerName}, Timeout: 15 * time.Second, QPS: 50, Burst: 100}
	rc.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		u, _ := url.Parse(c.Endpoint)
		if address != u.Host {
			return nil, errors.New("UNEXPECTED_DIAL")
		}
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, c.DialAddress)
	}
	rc.WrapTransport = func(t http.RoundTripper) http.RoundTripper { return getOnly{t} }
	return dynamic.NewForConfig(rc)
}
func validIdentity(id identity) bool {
	age := time.Since(id.ObservedAt)
	return age >= 0 && age <= 2*time.Minute && id.SSH == "kubercloud@100.64.128.12" && id.Host == "node1" && id.Context == "kubernetes-admin@ani-kcn-full" && id.APIServer == "https://lb.kubesphere.local:6443" && id.ClusterID == "f39bf6e2-f3ed-4191-ada3-caeebf86c220" && id.KubeSystemUID == "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24" && id.NamespaceUID == "94f3afc0-7548-4099-9e11-3fa7fdf6cc67" && id.ServiceAccountUID == "7100a777-b770-4f77-9509-631c8b39f1ec" && id.TransportPodUID != "" && id.APIServiceUID != "" && id.APIServiceVIP == "10.96.0.1" && id.APIServicePort == 443 && id.Database == "ani_modeldev_v21" && id.Role == "modeldev_v21_runtime" && id.OldWorkerReplicas == 0 && id.OldWorkerPods == 0 && id.GovernanceCreateLeases == 0
}
func save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	b = append(b, '\n')
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("EXCLUSIVE_OUTPUT_FAILED")
	}
	defer f.Close()
	_, e = f.Write(b)
	return e
}
func run() error {
	flags := flag.NewFlagSet("originalabe", flag.ContinueOnError)
	cfg := flags.String("config", "", "private ordinary config file")
	cfgSHA := flags.String("config-sha256", "", "exact config SHA256")
	phase := flags.String("phase", "plan", "plan or observe")
	out := flags.String("out", "", "new finite evidence file")
	planFile := flags.String("plan", "", "reviewed original plan file")
	planSHA := flags.String("plan-sha256", "", "exact reviewed plan SHA256")
	authorized := flags.Bool("authorize-observe", false, "root reviewed exact absence-only mutation")
	if e := flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	if flags.NArg() != 0 || *out == "" || (*phase != "plan" && *phase != "observe") {
		return errors.New("FLAGS_INVALID")
	}
	raw, e := regular(*cfg, 1<<20)
	if e != nil {
		return e
	}
	if len(*cfgSHA) != 64 || owner.SHA(raw) != *cfgSHA {
		return errors.New("CONFIG_SHA_MISMATCH")
	}
	var c config
	if e = decode(raw, &c); e != nil {
		return e
	}
	if c.Schema != "ani.modeldev.original-abe-config.v1" {
		return errors.New("CONFIG_SCHEMA_INVALID")
	}
	raw, e = regular(c.IdentityFile, 1<<20)
	if e != nil {
		return e
	}
	if owner.SHA(raw) != c.IdentitySHA256 {
		return errors.New("IDENTITY_SHA_MISMATCH")
	}
	var id identity
	if e = decode(raw, &id); e != nil {
		return e
	}
	if !validIdentity(id) {
		return errors.New("FRESH_FULL_IDENTITY_AND_STOP_PROOF_REQUIRED")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	kube, e := client(ctx, c, id)
	if e != nil {
		return e
	}
	dsn, e := regular(c.DSNFile, 1<<20)
	if e != nil {
		return e
	}
	pc, e := pgxpool.ParseConfig(strings.TrimSpace(string(dsn)))
	if e != nil {
		return errors.New("ORIGINAL_DSN_INVALID")
	}
	pc.MaxConns = 2
	if id.PostgreSQLServiceUID != "a211aa89-9521-4570-836f-b9dbb1755e79" || id.PostgreSQLServiceVIP != "10.96.133.128" || id.PostgreSQLServicePort != 5432 || pc.ConnConfig.Port != 5432 {
		return errors.New("ORIGINAL_POSTGRESQL_SERVICE_MISMATCH")
	}
	originalDatabaseHost := pc.ConnConfig.Host
	pc.ConnConfig.LookupFunc = func(ctx context.Context, host string) ([]string, error) {
		if host != originalDatabaseHost {
			return nil, errors.New("UNEXPECTED_DATABASE_HOST")
		}
		return []string{id.PostgreSQLServiceVIP}, nil
	}

	if *phase == "plan" {
		pc.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	}
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		return errors.New("DATABASE_CONNECT_FAILED")
	}
	defer pool.Close()
	var database, role string
	if e = pool.QueryRow(ctx, "SELECT current_database(),current_user").Scan(&database, &role); e != nil || database != id.Database || role != id.Role {
		return errors.New("ORIGINAL_DATABASE_ROLE_MISMATCH")
	}
	r, e := execution.New(pool).GetOperationRecord(ctx, owner.TenantID, owner.ExecutionID)
	if e != nil {
		return errors.New("ORIGINAL_RECORD_READ_FAILED")
	}
	if e = owner.ValidateFixed(r); e != nil {
		return e
	}
	if *phase == "plan" {
		p, e := owner.Inspect(ctx, kube, r)
		if e != nil {
			return e
		}
		return save(*out, p)
	}
	if !*authorized || *planFile == "" || len(*planSHA) != 64 {
		return errors.New("REVIEWED_PLAN_AND_EXPLICIT_OBSERVE_REQUIRED")
	}
	raw, e = regular(*planFile, 1<<20)
	if e != nil {
		return e
	}
	if owner.SHA(raw) != *planSHA {
		return errors.New("PLAN_SHA_MISMATCH")
	}
	var p owner.Plan
	if e = decode(raw, &p); e != nil {
		return e
	}
	if p.Schema != "ani.modeldev.original-abe-compute-plan.v1" || p.TenantID != owner.TenantID || p.ExecutionID != owner.ExecutionID {
		return errors.New("FIXED_ORIGINAL_PLAN_REQUIRED")
	}
	state, e := owner.Observe(ctx, pool, kube, p)
	result := map[string]any{"schema": "ani.modeldev.original-abe-observe-result.v1", "observed_at": time.Now().UTC(), "execution_id": owner.ExecutionID, "plan_sha256": *planSHA, "original_attempts": 3, "owner_revision": state.OwnerRevision, "closed_at": state.ClosedAt, "compute_phase": "", "kubernetes_mutation": false, "kfp_requests": 0, "success": e == nil}
	if state.ComputeCleanup != nil {
		result["compute_phase"] = state.ComputeCleanup.Phase
		result["attempts"] = state.ComputeCleanup.Attempts
		result["confirmed_absent"] = len(state.ComputeCleanup.ConfirmedAbsent)
	}
	if writeErr := save(*out, result); writeErr != nil {
		return writeErr
	}
	return e
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "originalabe:", e.Error())
		os.Exit(1)
	}
}
