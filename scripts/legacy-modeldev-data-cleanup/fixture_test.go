package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/admissionfixture"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

type cleanupFixture struct {
	command             *CleanupCommand
	md, gov, runtime    *pgxpool.Pool
	mdSchema, govSchema string
	admission           biz.Admission
}

func newCleanupFixture(t *testing.T, gpu ...bool) *cleanupFixture {
	t.Helper()
	if os.Getenv("CPU_P01_TEST_DATABASE_ADMIN_URL") == "" {
		t.Skip("explicit isolated real PostgreSQL dependency not enabled")
	}
	tenant := "11111111-2222-4333-8444-555555555555"
	ctx := context.Background()
	open := postgres.Prepare(t)
	runtime := open()
	config, err := pgxpool.ParseConfig(os.Getenv("CPU_P01_TEST_DATABASE_ADMIN_URL"))
	if err != nil {
		t.Fatal("private admin reference invalid")
	}
	config.ConnConfig.RuntimeParams = map[string]string{"search_path": runtime.Config().ConnConfig.RuntimeParams["search_path"]}
	config.MaxConns = 3
	md, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(md.Close)
	govSchema := "legacy_gov_" + uuid.NewString()[:8]
	if _, err = md.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{govSchema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	gconfig := config.Copy()
	gconfig.ConnConfig.RuntimeParams = map[string]string{"search_path": govSchema}
	gov, err := pgxpool.NewWithConfig(ctx, gconfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = md.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{govSchema}.Sanitize()+" CASCADE")
		gov.Close()
	})
	contract := fixedContract()
	groups := map[string][]column{}
	for _, col := range contract.GovColumns {
		groups[col.Table] = append(groups[col.Table], col)
	}
	for table, cols := range groups {
		defs := []string{}
		for _, col := range cols {
			defs = append(defs, pgx.Identifier{col.Column}.Sanitize()+" "+col.Type)
		}
		defs = append(defs, "PRIMARY KEY(id)")
		if _, err = gov.Exec(ctx, "CREATE TABLE "+pgx.Identifier{table}.Sanitize()+"("+strings.Join(defs, ",")+")"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = gov.Exec(ctx, `CREATE TABLE sys_tenants(id bigint PRIMARY KEY,resource_tenant_id varchar NOT NULL,UNIQUE(id,resource_tenant_id));ALTER TABLE sys_quota_accounts ADD UNIQUE(tenant_id,quota_code);ALTER TABLE sys_quota_operations ADD UNIQUE(tenant_id,operation_id)`); err != nil {
		t.Fatal(err)
	}
	// These retained tables own the fixed incoming FKs present in the existing
	// Governance migrations; they are outside every deletion target.
	if _, err = gov.Exec(ctx, `ALTER TABLE sys_quota_operations ADD UNIQUE(tenant_id,operation_id,resource_tenant_id,owner_service,resource_id);ALTER TABLE sys_quota_definitions ADD UNIQUE(code);CREATE TABLE sys_gpu_delete_acceptances(id bigint PRIMARY KEY,tenant_id bigint NOT NULL,create_operation_id varchar NOT NULL,delete_operation_id varchar NOT NULL);CREATE TABLE sys_plan_quotas(id bigint PRIMARY KEY,quota_code varchar NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, fk := range contract.GovKeys {
		if _, err = gov.Exec(ctx, "ALTER TABLE "+pgx.Identifier{fk.Child}.Sanitize()+" ADD CONSTRAINT "+pgx.Identifier{fk.Name}.Sanitize()+" "+fk.Definition); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = gov.Exec(ctx, `INSERT INTO sys_tenants VALUES(2,'11111111-2222-4333-8444-555555555555'),(3,'99999999-1111-4111-8111-111111111111');INSERT INTO sys_quota_accounts(id,tenant_id,quota_code,occupied_units,version) VALUES(2,2,'gpu.physical.count',0,4),(6,2,'gpu.shared_memory_mib',3,8),(7,3,'gpu.shared_memory_mib',7,2);CREATE TABLE sys_users(id bigint PRIMARY KEY);CREATE TABLE sys_roles(id bigint PRIMARY KEY);CREATE TABLE sys_quota_policies(id bigint PRIMARY KEY);INSERT INTO sys_users VALUES(16);INSERT INTO sys_roles VALUES(8);INSERT INTO sys_quota_policies VALUES(4)`); err != nil {
		t.Fatal(err)
	}
	accepted := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	snapshot := conformance.SnapshotV1()
	snapshot.DeadlineAt = accepted.Add(time.Hour)
	a := biz.Admission{TenantID: tenant, ExecutionID: "11111111-1111-4111-8111-111111111111", OperationID: "22222222-2222-4222-8222-222222222222", Actor: "governance:user:42", Snapshot: snapshot, AcceptedAt: accepted, Intent: cpup01.Intent{Name: "legacy-cleanup", Kind: "GENERAL_TRAINING", ImplementationID: snapshot.Implementation.ImplementationID, ComputePresetID: snapshot.ComputeOrigin.ComputePresetID, DatasetVersionID: snapshot.Input.InputVersionID, MLP: &cpup01.MLPParameters{}}}
	if len(gpu) > 0 && gpu[0] {
		a.Snapshot = gpuCleanupSnapshot(t, a.Snapshot)
	}
	a = admissionfixture.Bind(t, a)
	if _, err = execution.New(runtime).Accept(ctx, a); err != nil {
		t.Fatal("real admission fixture", err)
	}
	if _, err = execution.New(runtime).ApplyCloseIntent(ctx, biz.CloseIntent{TenantID: a.TenantID, ExecutionID: a.ExecutionID, OperationID: a.OperationID, SpecHash: a.SpecHash, SourceGeneration: 1, Reason: biz.CloseReasonUserStop, RequestedAt: time.Now().UTC().Truncate(time.Microsecond), RequestedActor: a.Actor}); err != nil {
		t.Fatal("real close-intent fixture", err)
	}
	if _, err = lifecycle.New(runtime).CloseUndispatched(ctx, a.TenantID, a.ExecutionID); err != nil {
		t.Fatal("real undispatched-close fixture", err)
	}
	if _, err = md.Exec(ctx, `INSERT INTO modeldev_execution_identities(tenant_id,execution_id,operation_id,spec_hash) VALUES('99999999-1111-4111-8111-111111111111','33333333-3333-4333-8333-333333333333','44444444-4444-4444-8444-444444444444',repeat('b',64)),('99999999-1111-4111-8111-111111111111','55555555-5555-4555-8555-555555555555','66666666-6666-4666-8666-666666666666',repeat('c',64))`); err != nil {
		t.Fatal(err)
	}
	govOperation := "99999999-2222-4222-8222-222222222222"
	govResource := "aaaaaaaa-1111-4111-8111-111111111111"
	ack := `{"schema":"ani.governance.modeldev-owner-receipt.v1","identity":{"operation_id":"` + govOperation + `","execution_id":"` + govResource + `","execution_spec_hash":"` + strings.Repeat("d", 64) + `"}}`
	eventID := "77777777-7777-4777-8777-777777777777"
	receiptID := "88888888-8888-4888-8888-888888888888"
	payload := `{"schema":"resource-release-v1","owner_service":"ani-modeldev-service","release_event_id":"` + eventID + `","create_operation_id":"` + govOperation + `","resource_id":"` + govResource + `","reason":"RESOURCE_RELEASED"}`
	if _, err = gov.Exec(ctx, `INSERT INTO sys_modeldev_acceptances(id,tenant_id,resource_tenant_id,operation_id,execution_id,execution_spec_hash,dispatch_state,close_dispatch_state) VALUES(801,2,$1,$2,$3,$4,'ACKED','IDLE');INSERT INTO sys_quota_operations(id,tenant_id,operation_id,resource_tenant_id,resource_id,owner_service,action,dispatch_state,ack_json) VALUES(901,2,$10,$1,$11,'ani-modeldev-service','modeldev.execution.create','ACKED',$5);INSERT INTO sys_quota_charges(id,tenant_id,charge_id,operation_id,quota_code,original_units,released_units) VALUES(701,2,'original-charge',$10,'gpu.shared_memory_mib',10,10);INSERT INTO sys_quota_release_receipts(id,tenant_id,receipt_id,owner_service,release_event_id,payload_hash,payload_json) VALUES(601,2,$6,'ani-modeldev-service',$7,$8,$9)`, pgx.QueryExecModeSimpleProtocol, tenant, a.OperationID, a.ExecutionID, a.SpecHash, ack, receiptID, eventID, bytesSHA([]byte(payload)), payload, govOperation, govResource); err != nil {
		t.Fatal(err)
	}
	if _, err = gov.Exec(ctx, `INSERT INTO sys_quota_operations(id,tenant_id,operation_id,resource_tenant_id,resource_id,owner_service,action,dispatch_state) VALUES(902,2,'other-owner-op','11111111-2222-4333-8444-555555555555','other-owner-resource','ani-inference-service','inference.create','ACKED'),(903,3,'other-tenant-op','99999999-1111-4111-8111-111111111111','other-tenant-resource','ani-resource-service','vm.create','ACKED');INSERT INTO sys_quota_charges(id,tenant_id,charge_id,operation_id,quota_code,original_units,released_units) VALUES(702,2,'other-owner-charge','other-owner-op','gpu.shared_memory_mib',3,0),(703,3,'other-tenant-charge','other-tenant-op','gpu.shared_memory_mib',7,0)`); err != nil {
		t.Fatal(err)
	}
	scope := OriginalScope{TenantID: tenant, GovernanceTenantID: 2, Executions: []ExecutionIdentity{{a.TenantID, a.ExecutionID, a.OperationID, a.SpecHash}}, AcceptanceIDs: []int64{801}, OperationIDs: []int64{901}, ChargeIDs: []int64{701}, ReceiptIDs: []string{receiptID}, InventorySHA256: strings.Repeat("a", 64), GovernanceInventorySHA256: strings.Repeat("b", 64)}
	scope.OriginalAcceptances = []row{mustRow(map[string]any{"id": 801, "tenant_id": 2, "resource_tenant_id": tenant, "operation_id": a.OperationID, "execution_id": a.ExecutionID, "execution_spec_hash": a.SpecHash, "dispatch_state": "ACKED"})}
	for _, pair := range []struct {
		dest  *[]json.RawMessage
		value any
	}{{&scope.OriginalOperations, map[string]any{"id": 901, "tenant_id": 2, "operation_id": govOperation, "resource_tenant_id": tenant, "resource_id": govResource, "create_operation_id": nil, "owner_service": "ani-modeldev-service", "action": "modeldev.execution.create", "ack_json": ack}}, {&scope.OriginalCharges, map[string]any{"id": 701, "tenant_id": 2, "charge_id": "original-charge", "operation_id": govOperation, "quota_code": "gpu.shared_memory_mib", "original_units": 10, "released_units": 10}}, {&scope.OriginalReceipts, map[string]any{"tenant_id": 2, "receipt_id": receiptID, "owner_service": "ani-modeldev-service", "release_event_id": eventID, "payload_hash": bytesSHA([]byte(payload)), "payload_json": payload}}} {
		raw, _ := json.Marshal(pair.value)
		*pair.dest = append(*pair.dest, raw)
	}
	return &cleanupFixture{command: &CleanupCommand{ModelDev: md, Governance: gov, Original: scope}, md: md, gov: gov, runtime: runtime, mdSchema: config.ConnConfig.RuntimeParams["search_path"], govSchema: govSchema, admission: a}
}
func mustRow(v any) row { raw, _ := json.Marshal(v); var r row; _ = json.Unmarshal(raw, &r); return r }
func (f *cleanupFixture) qualifyRoot(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	plan, err := f.command.Plan(ctx, "governance", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ModelDev.CatalogSHA256 == "" || plan.Governance.CatalogSHA256 == "" || plan.ModelDev.Refusal != "" || plan.Governance.Refusal != "" {
		for _, part := range []struct {
			pool *pgxpool.Pool
			md   bool
		}{{f.md, true}, {f.gov, false}} {
			tx, e := part.pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			_, e = snapshotDatabase(ctx, tx, f.command.Original, part.md)
			_ = tx.Rollback(ctx)
			if e != nil {
				t.Fatalf("real schema fixture md=%v: %v", part.md, e)
			}
		}
	}
	evidence := filepath.Join(t.TempDir(), "external-boundary.json")
	if err = os.WriteFile(evidence, []byte(`{"fixture":"only Kubernetes/KFP/S3/root-stopped-source external evidence is substituted"}`), 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(evidence)
	proof := SourceProof{Path: evidence, SHA256: bytesSHA(raw)}
	now := time.Now().UTC()
	r := &RootEvidence{Schema: "ani.modeldev.legacy-cleanup-root-evidence.v1", ObservedAt: now.Add(-time.Second), ExpiresAt: now.Add(4 * time.Minute), SSH: "kubercloud@100.64.128.12", Context: "kubernetes-admin@ani-kcn-full", APIServer: "https://lb.kubesphere.local:6443", ClusterID: "f39bf6e2-f3ed-4191-ada3-caeebf86c220", KubeSystemUID: "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24", InventorySHA256: f.command.Original.InventorySHA256, GovernanceInventorySHA256: f.command.Original.GovernanceInventorySHA256, Databases: map[string]DatabaseIdentity{"modeldev": plan.ModelDev.Identity, "governance": plan.Governance.Identity}}
	for _, component := range []string{"MODELDEV_WORKERS", "GOVERNANCE_CREATE_PUBLISHER", "LEGACY_REPLAY_AND_PUBLICATION"} {
		r.StopRecords = append(r.StopRecords, StoppedSource{Component: component, SourceUID: uuid.NewString(), SourceRevision: strings.Repeat("c", 40), State: "STOPPED", ObservedAt: now, Proof: proof})
	}
	r.Probe = HTTPProbe{Method: "POST", Path: "/api/v1/modeldev/executions", RequestID: uuid.NewString(), Status: 503, ObservedAt: now, Proof: proof}
	d := ExternalDisposition{ExecutionID: f.admission.ExecutionID, OperationID: f.admission.OperationID, SpecHash: f.admission.SpecHash, Category: "CLEANED_ORIGINAL_EXTERNAL_RESOURCES"}
	for _, kind := range []string{"KUBERNETES_ORIGINAL_UIDS", "KFP_ORIGINAL_RUN_WORKFLOW", "S3_OWNED_OBJECT_VERSIONS"} {
		p := proof
		p.Kind = kind
		d.Proofs = append(d.Proofs, p)
	}
	r.Dispositions = []ExternalDisposition{d}
	for _, op := range originalRows(f.command.Original.OriginalOperations) {
		extra := d
		extra.ExecutionID = text(op, "resource_id")
		extra.OperationID = text(op, "operation_id")
		extra.SpecHash = ""
		extra.Category = "PRIOR_AUTHENTICATED_OWNER_RELEASE"
		r.Dispositions = append(r.Dispositions, extra)
	}
	f.command.Root = r
	f.command.RootSHA256 = digest(r)
}
func (f *cleanupFixture) requireOriginalCounts(t *testing.T) {
	t.Helper()
	var md, gov int
	if err := f.md.QueryRow(context.Background(), "SELECT count(*) FROM modeldev_execution_identities").Scan(&md); err != nil {
		t.Fatal(err)
	}
	if err := f.gov.QueryRow(context.Background(), "SELECT count(*) FROM sys_quota_operations").Scan(&gov); err != nil {
		t.Fatal(err)
	}
	if md != 3 || gov != 3 {
		t.Fatalf("original rows changed md=%d gov=%d", md, gov)
	}
}
func (f *cleanupFixture) assertProtected(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	checks := []struct {
		db    *pgxpool.Pool
		query string
		want  int
	}{{f.md, "SELECT count(*) FROM modeldev_execution_identities WHERE execution_id IN ('33333333-3333-4333-8333-333333333333','55555555-5555-4555-8555-555555555555')", 2}, {f.gov, "SELECT count(*) FROM sys_quota_operations WHERE id IN(902,903)", 2}, {f.gov, "SELECT count(*) FROM sys_quota_charges WHERE id IN(702,703)", 2}, {f.gov, "SELECT count(*) FROM sys_quota_accounts", 3}, {f.gov, "SELECT (SELECT count(*) FROM sys_users)+(SELECT count(*) FROM sys_roles)+(SELECT count(*) FROM sys_quota_policies)", 3}}
	for _, check := range checks {
		var n int
		if err := check.db.QueryRow(ctx, check.query).Scan(&n); err != nil || n != check.want {
			t.Fatal(fmt.Sprintf("protected rows differ count=%d wanted=%d err=%v", n, check.want, err))
		}
	}
}
