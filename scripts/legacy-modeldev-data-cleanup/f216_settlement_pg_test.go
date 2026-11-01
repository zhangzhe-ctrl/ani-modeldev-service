package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	quotapb "github.com/zhangzhe-ctrl/ani-governance/api/quota/gen/go/quota/service/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/accelerator"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

type f216QuotaPeer struct {
	quotapb.UnimplementedQuotaReleaseServiceServer
	t     *testing.T
	calls atomic.Int32
}

func (s *f216QuotaPeer) ReportResourceReleased(ctx context.Context, r *quotapb.ReportResourceReleasedRequest) (*quotapb.ReportResourceReleasedResponse, error) {
	s.calls.Add(1)
	remote, ok := peer.FromContext(ctx)
	if !ok {
		s.t.Fatal("missing mTLS owner peer")
	}
	identity, ok := remote.AuthInfo.(credentials.TLSInfo)
	if !ok || len(identity.State.VerifiedChains) == 0 || len(identity.State.PeerCertificates) != 1 || strings.Join(identity.State.PeerCertificates[0].DNSNames, ",") != accelerator.ClientDNSName {
		s.t.Fatal("original authenticated owner identity lost")
	}
	if r.ReleaseEventId != f216Event.ReleaseEventID || r.CreateOperationId != f216Operation || r.ResourceId != f216ID || r.Reason != quotapb.ReleaseReason_RESOURCE_RELEASED || len(r.ProtoReflect().GetUnknown()) != 0 {
		s.t.Fatal("original four-field release command changed")
	}
	return &quotapb.ReportResourceReleasedResponse{ReleaseEventId: r.ReleaseEventId, CreateOperationId: r.CreateOperationId, ResourceId: r.ResourceId, Status: quotapb.ResourceReleaseStatus_ALREADY_APPLIED}, nil
}
func f216ProductionPeer(t *testing.T) (*accelerator.QuotaClient, *f216QuotaPeer) {
	t.Helper()
	certs := commandtls.NewForServer(t, accelerator.GovernanceDNSName)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certs.Server}, ClientCAs: certs.Roots, ClientAuth: tls.RequireAndVerifyClientCert})))
	boundary := &f216QuotaPeer{t: t}
	quotapb.RegisterQuotaReleaseServiceServer(server, boundary)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	files := certs.WriteServerFiles(t)
	pair := certs.ClientCertificate(t, func(c *x509.Certificate) { c.DNSNames = []string{accelerator.ClientDNSName} })
	private, err := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certfile, keyfile := filepath.Join(dir, "owner.pem"), filepath.Join(dir, "owner.key")
	if os.WriteFile(certfile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]}), 0600) != nil || os.WriteFile(keyfile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600) != nil {
		t.Fatal("synthetic private TLS material unavailable")
	}
	client, close, err := accelerator.NewQuotaClient(accelerator.Config{Endpoint: listener.Addr().String(), CAFile: files.CAFile, CertificateFile: certfile, PrivateKeyFile: keyfile, ServerName: accelerator.GovernanceDNSName, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(close)
	return client, boundary
}
func f216ActualFixture(t *testing.T) *cleanupFixture {
	t.Helper()
	path := os.Getenv("ORIGINAL_F216_PRIVATE_INPUT")
	if path == "" {
		t.Skip("protected actual original f216 fixture explicitly required")
	}
	raw, err := readBounded(path, 1<<20, true)
	if err != nil {
		t.Fatal("protected original fixture unavailable")
	}
	var captured struct{ ModelDev, Governance map[string][]row }
	if json.Unmarshal(raw, &captured) != nil {
		t.Fatal("protected fixture shape invalid")
	}
	f := newCleanupFixture(t)
	ctx := context.Background()
	// Only isolated PG schemas are reset; no live source is writable from tests.
	if _, err = f.gov.Exec(ctx, `DELETE FROM sys_modeldev_acceptances;DELETE FROM sys_quota_charges;DELETE FROM sys_quota_release_receipts;DELETE FROM sys_quota_operations;DELETE FROM sys_quota_accounts;UPDATE sys_tenants SET resource_tenant_id='64c5c35a-f554-432c-b4bc-7b4f36f08420' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	insert := func(md bool, table string, rows []row) {
		t.Helper()
		pool, schema := f.gov, f.govSchema
		if md {
			pool, schema = f.md, f.mdSchema
		}
		columns, err := pool.Query(ctx, "SELECT column_name FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND is_generated='NEVER' ORDER BY ordinal_position", schema, table)
		if err != nil {
			t.Fatal(err)
		}
		names, selects := []string{}, []string{}
		for columns.Next() {
			var name string
			if columns.Scan(&name) != nil {
				t.Fatal("column unavailable")
			}
			q := pgx.Identifier{name}.Sanitize()
			names = append(names, q)
			selects = append(selects, "r."+q)
		}
		columns.Close()
		for _, r := range rows {
			b, _ := json.Marshal(r)
			target := quoted(schema, table)
			if _, err = pool.Exec(ctx, "INSERT INTO "+target+"("+strings.Join(names, ",")+") SELECT "+strings.Join(selects, ",")+" FROM json_populate_record(NULL::"+target+",$1::json) r", b); err != nil {
				t.Fatalf("fixed actual fixture restore %s: %v", table, err)
			}
		}
	}
	for _, table := range []string{"modeldev_execution_identities", "modeldev_executions", "modeldev_pipeline_dispatches", "modeldev_pipeline_confirmed_runs", "modeldev_run_authorities", "modeldev_close_intents", "modeldev_execution_runtimes", "modeldev_execution_cleanup_audits"} {
		insert(true, table, captured.ModelDev[table])
	}
	for _, table := range []string{"sys_quota_accounts", "sys_quota_operations", "sys_quota_charges", "sys_quota_release_receipts"} {
		insert(false, table, captured.Governance[table])
	}
	return f
}
func TestOriginalF216ActualNativeAndProductionMTLSReplayZeroDelta(t *testing.T) {
	f := f216ActualFixture(t)
	ctx := context.Background()
	tx, err := beginCleanup(ctx, f.md, false)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := f.command.f216Plan(ctx, tx, false)
	rollbackCleanup(tx)
	if err != nil {
		t.Fatal("original normal reader/runtimeNeverMaterialized failed", err)
	}
	client, peer := f216ProductionPeer(t)
	file := filepath.Join(t.TempDir(), "journal.json")
	journal, err := f.command.settleF216(ctx, plan, strings.Repeat("a", 64), strings.Repeat("b", 64), file, client)
	if err != nil || !validF216Journal(journal, plan) || peer.calls.Load() != 1 {
		t.Fatalf("real owner replay/journal %s calls%d err%v", journal.State, peer.calls.Load(), err)
	}

	// DB cleanup consumes only a root byte-hash-bound independent journal. It
	// re-runs the original normal reader and all fixed original identity guards.
	proofRaw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	f.command.Root = &RootEvidence{Databases: map[string]DatabaseIdentity{"governance": plan.Ledger.Identity}, OriginalF216Settlement: &SourceProof{Kind: "ORIGINAL_F216_PRIOR_GPU_RELEASE", Path: file, SHA256: bytesSHA(proofRaw)}}
	f.command.RootSHA256 = strings.Repeat("c", 64)
	nativeTx, e := beginCleanup(ctx, f.md, false)
	if e != nil {
		t.Fatal(e)
	}
	defer rollbackCleanup(nativeTx)
	admissions, _, e := queryRows(ctx, nativeTx, "SELECT to_jsonb(t) FROM "+quoted(f.mdSchema, "modeldev_executions")+" t WHERE execution_id=$1::uuid", f216ID)
	if e != nil || len(admissions) != 1 {
		t.Fatal("original admission unavailable")
	}
	execution, e := admitted(admissions[0])
	if e != nil {
		t.Fatal(e)
	}
	states, _, e := queryRows(ctx, nativeTx, "SELECT to_jsonb(t) FROM "+quoted(f.mdSchema, "modeldev_execution_runtimes")+" t WHERE execution_id=$1::uuid", f216ID)
	if e != nil || len(states) != 1 {
		t.Fatal("original runtime unavailable")
	}
	state, e := lifecycle.ReadInTransaction(ctx, nativeTx, legacyTenant, f216ID)
	if e != nil {
		t.Fatal(e)
	}
	if !f.command.originalF216JournalValid(ctx, nativeTx, execution, state, states[0]) {
		t.Fatal("real independent ACK journal refused")
	}
	for _, kind := range []string{"wrong_ack", "wrong_event", "wrong_md_row", "before_close", "wrong_kind"} {
		bad := journal
		bad.Plan.MDRowSHA256 = map[string]string{}
		for k, v := range journal.Plan.MDRowSHA256 {
			bad.Plan.MDRowSHA256[k] = v
		}
		ack := *journal.ACK
		bad.ACK = &ack
		if kind == "wrong_ack" {
			bad.ACK.Status = "APPLIED"
		}
		if kind == "wrong_event" {
			bad.Event.Reason = "ABORTED_CLEANED"
		}
		if kind == "wrong_md_row" {
			bad.Plan.MDRowSHA256["modeldev_execution_runtimes"] = strings.Repeat("0", 64)
		}
		if kind == "before_close" {
			bad.ObservedAt = state.ClosedAt.Add(-time.Second)
		}
		badFile := filepath.Join(t.TempDir(), "bad.json")
		b, _ := json.Marshal(bad)
		if os.WriteFile(badFile, b, 0600) != nil {
			t.Fatal("journal fixture unavailable")
		}
		kindName := "ORIGINAL_F216_PRIOR_GPU_RELEASE"
		if kind == "wrong_kind" {
			kindName = "LEGACY_THREE_EXCEPTION"
		}
		f.command.Root.OriginalF216Settlement = &SourceProof{Kind: kindName, Path: badFile, SHA256: bytesSHA(b)}
		if f.command.originalF216JournalValid(ctx, nativeTx, execution, state, states[0]) {
			t.Fatal("invalid independent journal accepted", kind)
		}
	}
	rollbackCleanup(nativeTx)

	var count int
	if err = f.md.QueryRow(ctx, `SELECT count(*) FROM modeldev_execution_runtimes WHERE execution_id=$1::uuid AND coalesce(facts->'GPURelease','null'::jsonb)='null'::jsonb AND gpu_release_sealed_at IS NULL AND gpu_release_scope_sha256 IS NULL`, f216ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("normal aggregate/outbox/scope was changed %d %v", count, err)
	}
	if _, err = f.command.settleF216(ctx, plan, strings.Repeat("a", 64), strings.Repeat("b", 64), file, client); err == nil || peer.calls.Load() != 1 {
		t.Fatal("existing journal caused a repeated request")
	}
}
func TestOriginalF216DriftStopsBeforeOwnerRPC(t *testing.T) {
	f := f216ActualFixture(t)
	ctx := context.Background()
	tx, err := beginCleanup(ctx, f.md, false)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := f.command.f216Plan(ctx, tx, false)
	rollbackCleanup(tx)
	if err != nil {
		t.Fatal(err)
	}
	client, peer := f216ProductionPeer(t)
	if _, err = f.md.Exec(ctx, `UPDATE modeldev_execution_runtimes SET closed_at=NULL WHERE execution_id=$1::uuid`, f216ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.command.settleF216(ctx, plan, "reviewed", "gate", filepath.Join(t.TempDir(), "journal.json"), client); err == nil || peer.calls.Load() != 0 {
		t.Fatal("unclosed/drifted original invoked owner")
	}
}

func TestOriginalF216StopGateRejectsMissingProofStaleIdentityAndWrongPlan(t *testing.T) {
	p := filepath.Join(t.TempDir(), "actual-proof.json")
	if os.WriteFile(p, []byte("root-reviewed external observation bytes"), 0600) != nil {
		t.Fatal("proof unavailable")
	}
	proof := SourceProof{Kind: "ROOT_REVIEWED_STOP", Path: p, SHA256: bytesSHA([]byte("root-reviewed external observation bytes"))}
	observed := time.Now().UTC().Add(-time.Second)
	planSHA := strings.Repeat("a", 64)
	gate := f216Gate{Schema: "ani.modeldev.original-f216-stop-gate.v1", ObservedAt: observed, ExpiresAt: observed.Add(time.Minute), PlanByteSHA256: planSHA, SSH: "kubercloud@100.64.128.12", Context: "kubernetes-admin@ani-kcn-full", APIServer: "https://lb.kubesphere.local:6443", ClusterID: "f39bf6e2-f3ed-4191-ada3-caeebf86c220", KubeSystemUID: "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24", Probe: HTTPProbe{Method: "POST", Path: "/api/v1/modeldev/executions", RequestID: "actual-authenticated-create", Status: 503, ObservedAt: observed, Proof: proof}}
	for _, component := range []string{"MODELDEV_WORKERS", "GOVERNANCE_CREATE_PUBLISHER", "LEGACY_REPLAY_AND_PUBLICATION"} {
		gate.StoppedSources = append(gate.StoppedSources, StoppedSource{Component: component, SourceUID: "original-owner-uid", SourceRevision: "original-source", State: "STOPPED", ObservedAt: observed, Proof: proof})
	}
	for _, kind := range []string{"qualified", "missing_source", "expired", "wrong_identity", "wrong_plan", "not_real503", "missing_proof"} {
		t.Run(kind, func(t *testing.T) {
			candidate := gate
			if kind == "missing_source" {
				candidate.StoppedSources = candidate.StoppedSources[:2]
			}
			if kind == "expired" {
				candidate.ExpiresAt = observed
			}
			if kind == "wrong_identity" {
				candidate.ClusterID = "other"
			}
			if kind == "wrong_plan" {
				candidate.PlanByteSHA256 = "other"
			}
			if kind == "not_real503" {
				candidate.Probe.Status = 200
			}
			if kind == "missing_proof" {
				candidate.Probe.Proof.SHA256 = ""
			}
			raw, _ := json.Marshal(candidate)
			err := checkF216Gate(raw, planSHA)
			if kind == "qualified" && err != nil || kind != "qualified" && err == nil {
				t.Fatalf("gate %s: %v", kind, err)
			}
		})
	}
}
