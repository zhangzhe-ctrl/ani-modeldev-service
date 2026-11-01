package main

// This is one original ADR0002 event replay, not a runtime reader extension or
// an aggregate/outbox repair. The ordinary native reader remains mandatory.
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/accelerator"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
)

const f216ID = "f2168fbf-d7c9-4e96-8cc6-784bb1546083"
const f216Operation = "49a62211-703b-4adf-abae-70cf2af78e7e"
const f216Spec = "60df838e3e66d376e7c8c8de51c9ef630aa6aaa6b627823c9867f1da2fdca232"
const f216Receipt = "be5d029e-ccab-5463-9678-f370890c0702"
const f216Payload = "65034ab014827a45ac1b74c55128ca2f26ae7aee92ccce7a8678ba87898fdca4"

var f216Event = biz.GPUResourceReleasedEvent{ReleaseEventID: "9aff1a0e-5c66-4ba1-88cc-3470db8388a5", CreateOperationID: f216Operation, ResourceID: f216ID, Reason: "RESOURCE_RELEASED"}

type f216Account struct {
	ID                                 int64
	QuotaCode                          string
	OccupiedUnits, AllOwnerChargeUnits int64
	RowSHA256                          string
}
type f216Ledger struct {
	Identity                                              DatabaseIdentity
	ReceiptRowSHA256, ChargeRowSHA256, OperationRowSHA256 string
	Accounts                                              []f216Account
	ActiveOriginalLeases                                  int64
}
type f216Plan struct {
	Schema                             string
	ModelDev                           DatabaseIdentity
	ExecutionID, OperationID, SpecHash string
	OwnerRevision, CloseGeneration     uint64
	MDRowSHA256                        map[string]string
	Ledger                             f216Ledger
	LedgerSHA256                       string
	Event                              biz.GPUResourceReleasedEvent
	ManifestSHA256                     string
}
type f216Journal struct {
	Schema, State                         string
	Plan                                  f216Plan
	PlanByteSHA256, RootGateSHA256        string
	Event                                 biz.GPUResourceReleasedEvent
	ACK                                   *biz.GPUReleaseACK
	BeforeLedgerSHA256, AfterLedgerSHA256 string
	ObservedAt                            time.Time
}
type f216Gate struct {
	Schema                                            string
	ObservedAt, ExpiresAt                             time.Time
	PlanByteSHA256                                    string
	SSH, Context, APIServer, ClusterID, KubeSystemUID string
	StoppedSources                                    []StoppedSource
	Probe                                             HTTPProbe
}

func f216Native(execution biz.Execution, state biz.ExecutionRuntime, runtime row) bool {
	// This is the original runtimeNeverMaterialized branch, narrowed to the one
	// fixed native CLOSED aggregate. No new close fact, cleanup or scope is made.
	return execution.TenantID == legacyTenant && execution.ExecutionID == f216ID && execution.OperationID == f216Operation && execution.SpecHash == f216Spec &&
		execution.Snapshot.ComputeOrigin.DeviceMode == "gpu" && execution.Snapshot.GPU != nil && execution.Snapshot.GPU.ValidateFor(execution.Snapshot.ComputeOrigin) == nil &&
		state.OwnerRevision == 7 && state.CloseGeneration == 1 && state.ClosedAt != nil && !state.ClosedAt.IsZero() && state.CloseReason == "STEP_FAILED" && state.CloseOperationID == "5af00435-04dc-46ca-b9b9-166647ae9903" && state.CloseEvidence != nil && !state.CloseEvidence.NoDispatch && state.CloseReviewReason == "" &&
		state.Workspace == nil && state.Training == nil && state.TrainingHandle == nil && state.Observation == nil && state.ComputeCleanup == nil && state.StepExitCleanup == nil && state.Publication == nil && state.RuntimeTemplate == nil && state.LegacyRetirement == nil && state.GPURelease == nil && null(runtime, "gpu_release_sealed_at") && null(runtime, "gpu_release_scope_sha256")
}
func readF216Native(ctx context.Context, tx pgx.Tx, locked bool) (f216Plan, error) {
	p := f216Plan{Schema: "ani.modeldev.original-f216-settlement.plan.v1", ExecutionID: f216ID, OperationID: f216Operation, SpecHash: f216Spec, Event: f216Event, MDRowSHA256: map[string]string{}}
	var err error
	p.ModelDev, err = databaseIdentity(ctx, tx)
	if err != nil {
		return p, err
	}
	if locked {
		for _, table := range []string{"modeldev_execution_identities", "modeldev_executions", "modeldev_execution_runtimes"} {
			if _, err = tx.Exec(ctx, "SELECT 1 FROM "+quoted(p.ModelDev.Schema, table)+" WHERE tenant_id=$1::uuid AND execution_id=$2::uuid FOR SHARE", legacyTenant, f216ID); err != nil {
				return p, err
			}
		}
	}
	var execution biz.Execution
	var runtime row
	for _, table := range []string{"modeldev_execution_identities", "modeldev_executions", "modeldev_execution_runtimes"} {
		rows, raw, err := queryRows(ctx, tx, "SELECT to_jsonb(t) FROM "+quoted(p.ModelDev.Schema, table)+" t WHERE tenant_id=$1::uuid AND execution_id=$2::uuid", legacyTenant, f216ID)
		if err != nil || len(rows) != 1 {
			return p, fmt.Errorf("F216_ORIGINAL_NATIVE_ROW_MISSING")
		}
		p.MDRowSHA256[table] = bytesSHA(raw[0])
		if table == "modeldev_executions" {
			execution, err = admitted(rows[0])
			if err != nil {
				return p, fmt.Errorf("F216_ORIGINAL_NORMAL_ADMISSION_READER_REJECTED")
			}
		}
		if table == "modeldev_execution_runtimes" {
			runtime = rows[0]
		}
	}
	state, err := lifecycle.ReadInTransaction(ctx, tx, legacyTenant, f216ID)
	if err != nil {
		return p, fmt.Errorf("F216_ORIGINAL_NORMAL_LIFECYCLE_READER_REJECTED")
	}
	if !f216Native(execution, state, runtime) {
		return p, fmt.Errorf("F216_ORIGINAL_RUNTIME_NEVER_MATERIALIZED_QUALIFICATION_REQUIRED")
	}
	p.OwnerRevision = state.OwnerRevision
	p.CloseGeneration = state.CloseGeneration
	return p, nil
}
func (c *CleanupCommand) readF216Ledger(ctx context.Context) (f216Ledger, error) {
	var v f216Ledger
	tx, err := beginCleanup(ctx, c.Governance, false)
	if err != nil {
		return v, err
	}
	defer rollbackCleanup(tx)
	v.Identity, err = databaseIdentity(ctx, tx)
	if err != nil {
		return v, err
	}
	schema := v.Identity.Schema
	rows, raw, err := queryRows(ctx, tx, "SELECT to_jsonb(t) FROM "+quoted(schema, "sys_quota_release_receipts")+" t WHERE tenant_id=2 AND receipt_id=$1", f216Receipt)
	if err != nil || len(rows) != 1 {
		return v, fmt.Errorf("F216_ORIGINAL_RELEASE_RECEIPT_REQUIRED")
	}
	r := rows[0]
	if text(r, "owner_service") != "ani-modeldev-service" || text(r, "release_event_id") != f216Event.ReleaseEventID || text(r, "payload_hash") != f216Payload || bytesSHA([]byte(text(r, "payload_json"))) != f216Payload {
		return v, fmt.Errorf("F216_ORIGINAL_RELEASE_EVENT_OR_PAYLOAD_DIFFERS")
	}
	var payload row
	if json.Unmarshal([]byte(text(r, "payload_json")), &payload) != nil || text(payload, "schema") != "resource-release-v1" || text(payload, "owner_service") != "ani-modeldev-service" || text(payload, "release_event_id") != f216Event.ReleaseEventID || text(payload, "create_operation_id") != f216Operation || text(payload, "resource_id") != f216ID || text(payload, "reason") != f216Event.Reason {
		return v, fmt.Errorf("F216_ORIGINAL_RELEASE_CANONICAL_DIFFERS")
	}
	v.ReceiptRowSHA256 = bytesSHA(raw[0])
	rows, raw, err = queryRows(ctx, tx, "SELECT to_jsonb(t) FROM "+quoted(schema, "sys_quota_charges")+" t WHERE tenant_id=2 AND operation_id=$1", f216Operation)
	if err != nil || len(rows) != 1 {
		return v, fmt.Errorf("F216_ORIGINAL_SINGLE_CHARGE_REQUIRED")
	}
	r = rows[0]
	if text(r, "charge_id") != "e1681b15-43c7-42a3-bb2a-c3984dc3caf7" || text(r, "quota_code") != "gpu.shared_memory_mib" || integer(r, "original_units") != 24570 || integer(r, "released_units") != 24570 || !null(r, "deleted_at") {
		return v, fmt.Errorf("F216_ORIGINAL_FULL_RELEASE_REQUIRED")
	}
	v.ChargeRowSHA256 = bytesSHA(raw[0])
	rows, raw, err = queryRows(ctx, tx, "SELECT to_jsonb(t) FROM "+quoted(schema, "sys_quota_operations")+" t WHERE tenant_id=2 AND operation_id=$1", f216Operation)
	if err != nil || len(rows) != 1 {
		return v, fmt.Errorf("F216_ORIGINAL_OPERATION_REQUIRED")
	}
	r = rows[0]
	if text(r, "owner_service") != "ani-modeldev-service" || text(r, "resource_tenant_id") != legacyTenant || text(r, "resource_id") != f216ID || text(r, "dispatch_state") != "ACKED" || !null(r, "lease_until") {
		return v, fmt.Errorf("F216_ORIGINAL_OPERATION_NOT_SETTLED")
	}
	v.OperationRowSHA256 = bytesSHA(raw[0])
	rows, raw, err = queryRows(ctx, tx, "SELECT to_jsonb(t) FROM "+quoted(schema, "sys_quota_accounts")+" t WHERE tenant_id=2 AND id IN(2,6) ORDER BY id")
	if err != nil || len(rows) != 2 {
		return v, fmt.Errorf("F216_ORIGINAL_ACCOUNT_SCOPE_REQUIRED")
	}
	for i, a := range rows {
		account := f216Account{ID: integer(a, "id"), QuotaCode: text(a, "quota_code"), OccupiedUnits: integer(a, "occupied_units"), RowSHA256: bytesSHA(raw[i])}
		if err = tx.QueryRow(ctx, "SELECT coalesce(sum(original_units-released_units),0)::bigint FROM "+quoted(schema, "sys_quota_charges")+" WHERE tenant_id=2 AND quota_code=$1 AND deleted_at IS NULL", account.QuotaCode).Scan(&account.AllOwnerChargeUnits); err != nil || account.AllOwnerChargeUnits != account.OccupiedUnits || account.OccupiedUnits < 0 {
			return v, fmt.Errorf("F216_ORIGINAL_ACCOUNT_INVARIANT_REQUIRED")
		}
		if i == 0 && (account.ID != 2 || account.QuotaCode != "gpu.physical.count") || i == 1 && (account.ID != 6 || account.QuotaCode != "gpu.shared_memory_mib") {
			return v, fmt.Errorf("F216_ORIGINAL_ACCOUNT_IDENTITY_DIFFERS")
		}
		v.Accounts = append(v.Accounts, account)
	}
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM "+quoted(schema, "sys_modeldev_acceptances")+" WHERE tenant_id=2 AND id=ANY($1::bigint[]) AND (lease_until IS NOT NULL OR close_lease_until IS NOT NULL)", c.Original.AcceptanceIDs).Scan(&v.ActiveOriginalLeases); err != nil || v.ActiveOriginalLeases != 0 {
		return v, fmt.Errorf("F216_ORIGINAL_PUBLISHER_LEASE_PRESENT")
	}
	return v, nil
}
func (c *CleanupCommand) f216Plan(ctx context.Context, tx pgx.Tx, locked bool) (f216Plan, error) {
	p, err := readF216Native(ctx, tx, locked)
	if err != nil {
		return p, err
	}
	p.Ledger, err = c.readF216Ledger(ctx)
	if err != nil {
		return p, err
	}
	p.LedgerSHA256 = digest(p.Ledger)
	p.ManifestSHA256 = digest(p)
	return p, nil
}
func checkF216Gate(raw []byte, planByteSHA string) error {
	var gate f216Gate
	if strictJSON(raw, &gate) != nil {
		return fmt.Errorf("F216_FRESH_ROOT_STOP_GATE_INVALID")
	}
	now := time.Now().UTC()
	if gate.Schema != "ani.modeldev.original-f216-stop-gate.v1" || gate.PlanByteSHA256 != planByteSHA || gate.ObservedAt.IsZero() || gate.ObservedAt.After(now) || !gate.ExpiresAt.After(now.Add(15*time.Second)) || gate.ExpiresAt.After(gate.ObservedAt.Add(5*time.Minute)) || gate.SSH != "kubercloud@100.64.128.12" || gate.Context != "kubernetes-admin@ani-kcn-full" || gate.APIServer != "https://lb.kubesphere.local:6443" || gate.ClusterID != "f39bf6e2-f3ed-4191-ada3-caeebf86c220" || gate.KubeSystemUID != "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24" {
		return fmt.Errorf("F216_FRESH_ROOT_IDENTITY_OR_SCOPE_DIFFERS")
	}
	components := map[string]bool{}
	for _, s := range gate.StoppedSources {
		if s.State != "STOPPED" || s.SourceUID == "" || s.SourceRevision == "" || s.ObservedAt.Before(gate.ObservedAt) || s.ObservedAt.After(now) || !proofValid(s.Proof) || components[s.Component] {
			return fmt.Errorf("F216_STOPPED_SOURCE_PROOF_INVALID")
		}
		components[s.Component] = true
	}
	if len(components) != 3 || !components["MODELDEV_WORKERS"] || !components["GOVERNANCE_CREATE_PUBLISHER"] || !components["LEGACY_REPLAY_AND_PUBLICATION"] {
		return fmt.Errorf("F216_ORIGINAL_WRITERS_NOT_STOPPED")
	}
	p := gate.Probe
	if p.Method != "POST" || p.Path != "/api/v1/modeldev/executions" || p.Status != 503 || p.RequestID == "" || p.ObservedAt.Before(gate.ObservedAt) || p.ObservedAt.After(now) || !proofValid(p.Proof) {
		return fmt.Errorf("F216_REAL_AUTHENTICATED_503_UNKNOWN")
	}
	return nil
}
func validF216Journal(j f216Journal, p f216Plan) bool {
	return j.Schema == "ani.modeldev.original-f216-settlement.journal.v1" && j.State == "ORIGINAL_EVENT_ALREADY_APPLIED_ZERO_DELTA" && validSHA(j.PlanByteSHA256) && validSHA(j.RootGateSHA256) && p.LedgerSHA256 == digest(p.Ledger) && reflect.DeepEqual(j.Plan, p) && j.Event == f216Event && j.ACK != nil && j.ACK.ReleaseEventID == f216Event.ReleaseEventID && j.ACK.CreateOperationID == f216Operation && j.ACK.ResourceID == f216ID && j.ACK.Status == "ALREADY_APPLIED" && j.BeforeLedgerSHA256 == p.LedgerSHA256 && j.AfterLedgerSHA256 == p.LedgerSHA256 && !j.ObservedAt.IsZero() && !j.ObservedAt.After(time.Now().UTC())
}
func persistF216(path string, j f216Journal) error {
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".next"
	fd, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = fd.Write(append(raw, '\n')); err == nil {
		err = fd.Sync()
	}
	closeErr := fd.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (c *CleanupCommand) settleF216(ctx context.Context, reviewed f216Plan, planByteSHA, gateSHA, journalPath string, reporter biz.GPUReleaseReporter) (f216Journal, error) {
	j := f216Journal{Schema: "ani.modeldev.original-f216-settlement.journal.v1", State: "RESULT_UNKNOWN", Plan: reviewed, PlanByteSHA256: planByteSHA, RootGateSHA256: gateSHA, Event: f216Event, BeforeLedgerSHA256: reviewed.LedgerSHA256}
	tx, err := c.ModelDev.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return j, err
	}
	defer rollbackCleanup(tx)
	actual, err := c.f216Plan(ctx, tx, true)
	if err != nil || !reflect.DeepEqual(actual, reviewed) {
		return j, fmt.Errorf("F216_REVIEWED_PLAN_DRIFTED")
	}
	if !filepath.IsAbs(journalPath) {
		return j, fmt.Errorf("F216_PRIVATE_JOURNAL_ABSOLUTE_PATH_REQUIRED")
	}
	f, err := os.OpenFile(journalPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return j, err
	}
	if err = f.Close(); err != nil {
		return j, err
	}
	if err = persistF216(journalPath, j); err != nil {
		return j, err
	}
	ack, err := reporter.ReportGPUResourceReleased(ctx, f216Event)
	if err != nil {
		return j, fmt.Errorf("F216_RPC_RESULT_UNKNOWN_REUSE_ORIGINAL_EVENT_ONLY")
	}
	j.ACK = &ack
	j.ObservedAt = time.Now().UTC()
	if err = persistF216(journalPath, j); err != nil {
		return j, err
	}
	after, err := c.f216Plan(ctx, tx, true)
	if err != nil {
		return j, err
	}
	j.AfterLedgerSHA256 = after.LedgerSHA256
	if !reflect.DeepEqual(after, reviewed) || ack.ReleaseEventID != f216Event.ReleaseEventID || ack.CreateOperationID != f216Operation || ack.ResourceID != f216ID || ack.Status != "ALREADY_APPLIED" {
		_ = persistF216(journalPath, j)
		return j, fmt.Errorf("F216_ORIGINAL_ACK_AND_ZERO_DELTA_NOT_PROVED")
	}
	if err = tx.Commit(ctx); err != nil {
		return j, fmt.Errorf("F216_ORIGINAL_NATIVE_CAS_TRANSACTION_UNKNOWN")
	}
	j.State = "ORIGINAL_EVENT_ALREADY_APPLIED_ZERO_DELTA"
	if err = persistF216(journalPath, j); err != nil {
		return j, err
	}
	return j, nil
}
func (c *CleanupCommand) runF216(ctx context.Context, mode, configFile, planFile, planByteSHA, gateFile, gateSHA, journalPath string, out io.Writer) error {
	if err := verifyF216Cluster(ctx); err != nil {
		return err
	}
	if mode == "plan" {
		tx, err := beginCleanup(ctx, c.ModelDev, false)
		if err != nil {
			return err
		}
		defer rollbackCleanup(tx)
		p, err := c.f216Plan(ctx, tx, false)
		if err != nil {
			return err
		}
		if !f216ProductionDatabases(p) {
			return fmt.Errorf("F216_ORIGINAL_DATABASE_IDENTITY_DIFFERS")
		}
		return json.NewEncoder(out).Encode(p)
	}
	if mode != "apply" {
		return fmt.Errorf("F216_MODE_INVALID")
	}
	raw, err := readBounded(planFile, 1<<20, false)
	if err != nil || bytesSHA(raw) != planByteSHA {
		return fmt.Errorf("F216_ROOT_REVIEWED_PLAN_BYTE_SHA_REQUIRED")
	}
	var p f216Plan
	if strictJSON(raw, &p) != nil || !f216ProductionDatabases(p) {
		return fmt.Errorf("F216_PLAN_OR_ORIGINAL_DATABASE_IDENTITY_INVALID")
	}
	check := p
	check.ManifestSHA256 = ""
	if p.Schema != "ani.modeldev.original-f216-settlement.plan.v1" || digest(check) != p.ManifestSHA256 {
		return fmt.Errorf("F216_PLAN_HASH_DIFFERS")
	}
	gate, err := readBounded(gateFile, 1<<20, false)
	if err != nil || bytesSHA(gate) != gateSHA || checkF216Gate(gate, planByteSHA) != nil {
		return fmt.Errorf("F216_FRESH_ROOT_GATE_REQUIRED")
	}
	configRaw, err := readBounded(configFile, 64<<10, true)
	if err != nil {
		return err
	}
	var config accelerator.Config
	if strictJSON(configRaw, &config) != nil || config.Endpoint != "ani-governance-quota.ani-system.svc:9002" || config.ServerName != accelerator.GovernanceDNSName || config.Timeout <= 0 || config.Timeout > 10*time.Second {
		return fmt.Errorf("F216_ORIGINAL_OWNER_MTLS_CONFIGURATION_REQUIRED")
	}
	client, close, err := accelerator.NewQuotaClient(config)
	if err != nil {
		return err
	}
	defer close()
	j, err := c.settleF216(ctx, p, planByteSHA, gateSHA, journalPath, client)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(j)
}

func (c *CleanupCommand) originalF216JournalValid(ctx context.Context, tx pgx.Tx, execution biz.Execution, state biz.ExecutionRuntime, runtime row) bool {
	if !f216Native(execution, state, runtime) || c.Root == nil || c.RootSHA256 == "" || c.Root.OriginalF216Settlement == nil {
		return false
	}
	proof := *c.Root.OriginalF216Settlement
	if proof.Kind != "ORIGINAL_F216_PRIOR_GPU_RELEASE" || !proofValid(proof) {
		return false
	}
	raw, err := readBounded(proof.Path, 1<<20, false)
	if err != nil {
		return false
	}
	var journal f216Journal
	if strictJSON(raw, &journal) != nil {
		return false
	}
	native, err := readF216Native(ctx, tx, false)
	if err != nil {
		return false
	}
	p := journal.Plan
	if journal.ObservedAt.Before(*state.ClosedAt) || !c.readQualificationIdentityMatches(p.Ledger.Identity, c.Root.Databases["governance"], "governance") || !c.readQualificationIdentityMatches(p.ModelDev, native.ModelDev, "modeldev") || p.ExecutionID != f216ID || p.OperationID != f216Operation || p.SpecHash != f216Spec || p.OwnerRevision != 7 || p.CloseGeneration != 1 || !reflect.DeepEqual(p.MDRowSHA256, native.MDRowSHA256) || p.Event != f216Event || len(p.Ledger.Accounts) != 2 || p.Ledger.ActiveOriginalLeases != 0 {
		return false
	}
	for _, account := range p.Ledger.Accounts {
		if account.OccupiedUnits != account.AllOwnerChargeUnits || account.OccupiedUnits < 0 || !validSHA(account.RowSHA256) {
			return false
		}
	}
	if !validSHA(p.Ledger.ReceiptRowSHA256) || !validSHA(p.Ledger.ChargeRowSHA256) || !validSHA(p.Ledger.OperationRowSHA256) {
		return false
	}
	check := p
	check.ManifestSHA256 = ""
	return p.ManifestSHA256 == digest(check) && validF216Journal(journal, p)
}

func f216ProductionDatabases(p f216Plan) bool {
	return p.ModelDev == (DatabaseIdentity{Database: "ani_modeldev_v21", Schema: "public", Role: "modeldev_v21_runtime", ServerAddress: "10.16.0.63", Port: 5432, DatabaseOID: 56512}) && p.Ledger.Identity == (DatabaseIdentity{Database: "ani_governance", Schema: "public", Role: "gov_runtime", ServerAddress: "10.16.0.63", Port: 5432, DatabaseOID: 16484})
}
