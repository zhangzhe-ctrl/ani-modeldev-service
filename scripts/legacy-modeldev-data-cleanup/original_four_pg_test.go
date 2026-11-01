package main

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestOriginalFourActualNativeNoDispatchClosureAndRefusals(t *testing.T) {
	path := os.Getenv("ORIGINAL_FOUR_PRIVATE_INPUT")
	if path == "" {
		t.Skip("explicit protected original-four real fixture not enabled")
	}
	raw, e := readBounded(path, 1<<20, true)
	if e != nil {
		t.Fatal("protected original-four fixture unavailable")
	}
	var captured struct {
		ModelDev   map[string][]row `json:"modeldev"`
		Governance struct {
			Acceptances []row `json:"acceptances"`
		} `json:"governance"`
	}
	if json.Unmarshal(raw, &captured) != nil {
		t.Fatal("original-four fixture shape invalid")
	}
	f := newCleanupFixture(t)
	ctx := context.Background()
	f.command.Original = OriginalScope{TenantID: legacyTenant}
	for id, w := range originalFour {
		f.command.Original.Executions = append(f.command.Original.Executions, w.ExecutionIdentity)
		f.command.Original.UnknownExecutionIDs = append(f.command.Original.UnknownExecutionIDs, id)
	}
	sort.Slice(f.command.Original.Executions, func(i, j int) bool {
		return f.command.Original.Executions[i].ExecutionID < f.command.Original.Executions[j].ExecutionID
	})
	for _, name := range []string{"modeldev_execution_identities", "modeldev_executions", "modeldev_pipeline_dispatches", "modeldev_pipeline_confirmed_runs", "modeldev_run_authorities", "modeldev_close_intents", "modeldev_execution_runtimes", "modeldev_execution_cleanup_audits"} {
		columns, e := f.md.Query(ctx, "SELECT column_name FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND is_generated='NEVER' ORDER BY ordinal_position", f.mdSchema, name)
		if e != nil {
			t.Fatal("original-four column contract unavailable")
		}
		names, selects := []string{}, []string{}
		for columns.Next() {
			var col string
			if columns.Scan(&col) != nil {
				t.Fatal("original-four column invalid")
			}
			q := pgx.Identifier{col}.Sanitize()
			names = append(names, q)
			selects = append(selects, "r."+q)
		}
		if columns.Err() != nil {
			t.Fatal("original-four column scan failed")
		}
		columns.Close()
		for _, r := range captured.ModelDev[name] {
			if _, ok := originalFour[text(r, "execution_id")]; !ok || text(r, "tenant_id") != legacyTenant {
				t.Fatal("fixture outside original four")
			}
			b, _ := json.Marshal(r)
			table := quoted(f.mdSchema, name)
			if _, e = f.md.Exec(ctx, "INSERT INTO "+table+"("+strings.Join(names, ",")+") SELECT "+strings.Join(selects, ",")+" FROM json_populate_record(NULL::"+table+",$1::json) r", b); e != nil {
				t.Fatalf("original-four table restore failed: %s", name)
			}
		}
	}
	read := func() (databaseView, []string) {
		t.Helper()
		tx, e := beginCleanup(ctx, f.md, false)
		if e != nil {
			t.Fatal(e)
		}
		defer rollbackCleanup(tx)
		v, e := snapshotDatabase(ctx, tx, f.command.Original, true)
		if e != nil {
			t.Fatal(e)
		}
		return v, f.command.qualifyMD(ctx, tx, v)
	}
	v, blockers := read()
	if len(blockers) != 0 {
		t.Fatal("actual original four normal closure refused", blockers)
	}
	if len(captured.Governance.Acceptances) != 4 {
		t.Fatal("original four Governance fixture scope differs")
	}
	for _, r := range captured.Governance.Acceptances {
		if !qualifyOriginalFourGov(r) {
			t.Fatal("actual original Create/Stop ACK/canonical identity rejected", text(r, "id"))
		}
		for _, key := range []string{"actor", "accepted_at", "intent_canonical", "snapshot_canonical", "owner_receipt_canonical", "close_receipt_canonical", "dispatch_state", "close_dispatch_state", "execution_id"} {
			bad := row{}
			for k, v := range r {
				bad[k] = v
			}
			bad[key] = json.RawMessage(`"invalid-original"`)
			if qualifyOriginalFourGov(bad) {
				t.Fatal("changed original Governance evidence accepted", key)
			}
		}
	}
	for _, r := range table(v, "modeldev_executions").rows {
		a, e := admitted(r)
		if e != nil || !qualifyOriginalFourAdmission(r, a) {
			t.Fatal("original full native admission invalid")
		}
		for _, key := range []string{"actor", "accepted_at", "intent_canonical", "snapshot_canonical"} {
			bad := row{}
			for k, v := range r {
				bad[k] = v
			}
			bad[key] = json.RawMessage(`"changed"`)
			if changed, e := admitted(bad); e == nil && qualifyOriginalFourAdmission(bad, changed) {
				t.Fatal("changed original admission adopted", key)
			}
		}
	}
	// Preserve the actual native NoDispatch NULL step/compute/outbox facts.
	target := f.command.Original.Executions[0]
	var unchanged int
	if f.md.QueryRow(ctx, `SELECT count(*) FROM modeldev_execution_runtimes WHERE tenant_id=$1::uuid AND execution_id=ANY($2::uuid[]) AND coalesce(facts->'StepExitCleanup','null'::jsonb)='null'::jsonb AND coalesce(facts->'ComputeCleanup','null'::jsonb)='null'::jsonb AND coalesce(facts->'GPURelease','null'::jsonb)='null'::jsonb`, legacyTenant, f.command.Original.UnknownExecutionIDs).Scan(&unchanged) != nil || unchanged != 4 {
		t.Fatal("native NoDispatch facts were synthesized or changed")
	}
	if _, e = f.md.Exec(ctx, `UPDATE modeldev_execution_runtimes SET closed_at=NULL WHERE tenant_id=$1::uuid AND execution_id=$2::uuid`, target.TenantID, target.ExecutionID); e != nil {
		t.Fatal("isolated closure drift failed")
	}
	if _, blocked := read(); len(blocked) == 0 {
		t.Fatal("unclosed original four accepted")
	}
}

func TestFixedInventoryNowRequiresExactlyOriginalFiftyPlusFour(t *testing.T) {
	md, gov := os.Getenv("ORIGINAL_MD_FIXED_INVENTORY"), os.Getenv("ORIGINAL_GOV_FIXED_INVENTORY")
	if md == "" || gov == "" {
		t.Skip("explicit fixed original inventory dependencies not enabled")
	}
	s, e := loadOriginal(md, gov)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Executions) != 54 || len(s.AcceptanceIDs) != 54 || len(s.OperationIDs) != 23 || len(s.ChargeIDs) != 23 || len(s.ReceiptIDs) != 23 {
		t.Fatal("finite54 scope drift")
	}
	seen := map[string]bool{}
	for _, w := range s.Executions {
		if seen[w.ExecutionID] {
			t.Fatal("duplicate original execution")
		}
		seen[w.ExecutionID] = true
	}
	for id, w := range originalFour {
		if !seen[id] || find(s.OriginalAcceptances, "id", strconv.FormatInt(w.AcceptanceID, 10)) == nil {
			t.Fatal("original four not bound to original Governance identity")
		}
	}
}

func TestOldFiftyPlanAndPriorReceiptCannotAuthorizeFixed54(t *testing.T) {
	f := newCleanupFixture(t)
	f.qualifyRoot(t)
	ctx := context.Background()
	p, e := f.command.Plan(ctx, "governance", nil)
	if e != nil || len(p.Blockers) != 0 {
		t.Fatal(p.Blockers, e)
	}
	old := p
	old.Schema = "ani.modeldev.legacy-db-cleanup.plan.v1"
	old.ManifestSHA256 = ""
	old.ManifestSHA256 = digest(old)
	if _, e = f.command.Apply(ctx, old, nil); e == nil {
		t.Fatal("old fixed-fifty manifest adopted")
	}
	f.requireOriginalCounts(t)
	receipt, e := f.command.Apply(ctx, p, nil)
	if e != nil {
		t.Fatal(e)
	}
	receipt.Plan.Schema = "ani.modeldev.legacy-db-cleanup.plan.v1"
	receipt.Plan.ManifestSHA256 = ""
	receipt.Plan.ManifestSHA256 = digest(receipt.Plan)
	if validatePriorReceipt(&receipt, f.command.Original) == nil {
		t.Fatal("old-scope prior receipt accepted")
	}
	f.assertProtected(t)
}
