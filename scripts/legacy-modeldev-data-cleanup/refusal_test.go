package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
)

func containsBlocker(p CleanupPlan, prefix string) bool {
	for _, b := range p.Blockers {
		if strings.HasPrefix(b, prefix) {
			return true
		}
	}
	return false
}
func TestUnreleasedChargeAndUnclosedExecutionRefuseDeletion(t *testing.T) {
	for _, kind := range []string{"unreleased", "unclosed"} {
		t.Run(kind, func(t *testing.T) {
			f := newCleanupFixture(t)
			f.qualifyRoot(t)
			ctx := context.Background()
			expected := "GOV_CHARGE_NOT_FULLY_RELEASED"
			if kind == "unreleased" {
				if _, err := f.gov.Exec(ctx, `UPDATE sys_quota_charges SET released_units=9 WHERE id=701;UPDATE sys_quota_accounts SET occupied_units=4 WHERE id=6`); err != nil {
					t.Fatal(err)
				}
			} else {
				expected = "MD_EXECUTION_NOT_CLOSED"
				if _, err := f.md.Exec(ctx, `UPDATE modeldev_execution_runtimes SET closed_at=NULL WHERE execution_id=$1::uuid`, f.admission.ExecutionID); err != nil {
					t.Fatal(err)
				}
			}
			p, err := f.command.Plan(ctx, "governance", nil)
			if err != nil || !containsBlocker(p, expected) {
				t.Fatalf("missing %s: %v %v", expected, p.Blockers, err)
			}
			if _, err = f.command.Apply(ctx, p, nil); err == nil {
				t.Fatal("ineligible rows deleted")
			}
			f.requireOriginalCounts(t)
			f.assertProtected(t)
		})
	}
}
func TestOriginalGPUOutboxWithoutACKRefusesDeletion(t *testing.T) {
	f := newCleanupFixture(t, true)
	ctx := context.Background()
	if _, err := lifecycle.New(f.runtime).SealGPURelease(ctx, f.admission.TenantID, f.admission.ExecutionID); err != nil {
		t.Fatal(err)
	}
	f.qualifyRoot(t)
	p, err := f.command.Plan(ctx, "governance", nil)
	if err != nil || !containsBlocker(p, "MD_ORIGINAL_GPU_OUTBOX_ACK_MISSING:") {
		t.Fatalf("unacknowledged original outbox accepted: %v %v", p.Blockers, err)
	}
	if _, err = f.command.Apply(ctx, p, nil); err == nil {
		t.Fatal("missing original ACK deleted")
	}
	f.requireOriginalCounts(t)
}
func TestUnknownAcceptanceDoesNotInferQualificationFromRetryBlocked(t *testing.T) {
	f := newCleanupFixture(t)
	ctx := context.Background()
	const unknown = "bbbbbbbb-1111-4111-8111-111111111111"
	const op = "cccccccc-1111-4111-8111-111111111111"
	if _, err := f.gov.Exec(ctx, `INSERT INTO sys_modeldev_acceptances(id,tenant_id,resource_tenant_id,operation_id,execution_id,execution_spec_hash,dispatch_state,attempt_count,retry_blocked,close_dispatch_state) VALUES(802,2,$1,$2,$3,$4,'UNKNOWN',0,true,'IDLE')`, f.command.Original.TenantID, op, unknown, strings.Repeat("e", 64)); err != nil {
		t.Fatal(err)
	}
	f.command.Original.AcceptanceIDs = append(f.command.Original.AcceptanceIDs, 802)
	f.command.Original.UnknownExecutionIDs = []string{unknown}
	f.command.Original.OriginalAcceptances = append(f.command.Original.OriginalAcceptances, mustRow(map[string]any{"id": 802, "tenant_id": 2, "resource_tenant_id": f.command.Original.TenantID, "operation_id": op, "execution_id": unknown, "execution_spec_hash": strings.Repeat("e", 64), "dispatch_state": "UNKNOWN"}))
	f.qualifyRoot(t)
	p, err := f.command.Plan(ctx, "governance", nil)
	if err != nil || !containsBlocker(p, "ROOT_EXTERNAL_QUALIFICATION_UNKNOWN:"+unknown) {
		t.Fatalf("UNKNOWN retry_blocked bypassed owner qualification: %v %v", p.Blockers, err)
	}
	if _, err = f.command.Apply(ctx, p, nil); err == nil {
		t.Fatal("unqualified unknown acceptance deleted")
	}
	f.requireOriginalCounts(t)
}
func TestReferenceChangeAndRowMismatchAbortWholeStage(t *testing.T) {
	for _, kind := range []string{"reference", "rowcount"} {
		t.Run(kind, func(t *testing.T) {
			f := newCleanupFixture(t)
			f.qualifyRoot(t)
			ctx := context.Background()
			p, err := f.command.Plan(ctx, "governance", nil)
			if err != nil || len(p.Blockers) != 0 {
				t.Fatal(p.Blockers, err)
			}
			if kind == "reference" {
				_, err = f.gov.Exec(ctx, `CREATE TABLE unexpected_reference(id bigint PRIMARY KEY,operation_id bigint REFERENCES sys_quota_operations(id));INSERT INTO unexpected_reference VALUES(1,901)`)
			} else {
				_, err = f.gov.Exec(ctx, `DELETE FROM sys_quota_release_receipts WHERE id=601`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.command.Apply(ctx, p, nil); err == nil {
				t.Fatal("changed FK/row set silently adopted")
			}
			f.requireOriginalCounts(t)
			f.assertProtected(t)
		})
	}
}
func waitFor(t *testing.T, check func() bool) {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for !check() {
		if time.Now().After(end) {
			t.Fatal("expected original PostgreSQL lock wait was not observed")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func TestConcurrentOriginalRevisionChangeIsRecheckedAfterDrain(t *testing.T) {
	f := newCleanupFixture(t)
	f.qualifyRoot(t)
	ctx := context.Background()
	p, err := f.command.Plan(ctx, "governance", nil)
	if err != nil || len(p.Blockers) != 0 {
		t.Fatal(p.Blockers, err)
	}
	writer, err := f.md.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackCleanup(writer)
	if _, err = writer.Exec(ctx, `UPDATE modeldev_execution_identities SET owner_revision=owner_revision+1 WHERE execution_id=$1::uuid`, f.admission.ExecutionID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := f.command.Apply(ctx, p, nil); done <- e }()
	waitFor(t, func() bool {
		var yes bool
		_ = f.md.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation=to_regclass($1) AND mode='ShareRowExclusiveLock' AND NOT granted)`, quoted(f.mdSchema, "modeldev_execution_identities")).Scan(&yes)
		return yes
	})
	if err = writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("concurrent original owner revision adopted")
	}
	f.requireOriginalCounts(t)
	f.assertProtected(t)
}
func TestCancellationAfterEarlierDeletesRollsBackTheWholeGovTransaction(t *testing.T) {
	f := newCleanupFixture(t)
	f.qualifyRoot(t)
	ctx := context.Background()
	p, err := f.command.Plan(ctx, "governance", nil)
	if err != nil || len(p.Blockers) != 0 {
		t.Fatal(p.Blockers, err)
	}
	writer, err := f.gov.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackCleanup(writer)
	var id int64
	if err = writer.QueryRow(ctx, `SELECT id FROM sys_quota_operations WHERE id=901 FOR UPDATE`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	call, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := f.command.Apply(call, p, nil); done <- e }()
	// This is the third DELETE in the approved Gov order, after acceptance and
	// charge DELETEs. A real row lock, rather than a callback/mock, blocks it.
	waitFor(t, func() bool {
		var yes bool
		_ = f.gov.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity a JOIN pg_locks l ON l.pid=a.pid WHERE l.locktype='transactionid' AND NOT l.granted AND position($1 in a.query)>0)`, `DELETE FROM `+quoted(f.govSchema, "sys_quota_operations")).Scan(&yes)
		return yes
	})
	cancel()
	if err = <-done; err == nil {
		t.Fatal("cancelled DELETE committed")
	}
	rollbackCleanup(writer)
	f.requireOriginalCounts(t)
	f.assertProtected(t)
	var acceptances, charges int
	if err = f.gov.QueryRow(ctx, `SELECT (SELECT count(*) FROM sys_modeldev_acceptances WHERE id=801),(SELECT count(*) FROM sys_quota_charges WHERE id=701)`).Scan(&acceptances, &charges); err != nil || acceptances != 1 || charges != 1 {
		t.Fatalf("earlier deletes escaped rollback: a=%d c=%d err=%v", acceptances, charges, err)
	}
}
func TestModelDevPhaseWithoutQualifiedGovernanceReceiptIsBlocked(t *testing.T) {
	f := newCleanupFixture(t)
	f.qualifyRoot(t)
	p, err := f.command.Plan(context.Background(), "modeldev", nil)
	if err != nil || !containsBlocker(p, "GOVERNANCE_ORIGINAL_DELETION_RECEIPT_UNVERIFIED") {
		t.Fatal(p.Blockers, err)
	}
	if _, err = f.command.Apply(context.Background(), p, nil); err == nil {
		t.Fatal("MD deleted before original Gov receipt")
	}
	f.requireOriginalCounts(t)
}

func TestSemanticReferenceOutsideOriginalIDsRefusesDeletion(t *testing.T) {
	for _, kind := range []string{"acceptance", "release_receipt"} {
		t.Run(kind, func(t *testing.T) {
			f := newCleanupFixture(t)
			f.qualifyRoot(t)
			ctx := context.Background()
			p, err := f.command.Plan(ctx, "governance", nil)
			if err != nil || len(p.Blockers) != 0 {
				t.Fatal(p.Blockers, err)
			}
			if kind == "acceptance" {
				_, err = f.gov.Exec(ctx, `INSERT INTO sys_modeldev_acceptances(id,tenant_id,resource_tenant_id,operation_id,execution_id,execution_spec_hash,dispatch_state) SELECT 802,tenant_id,resource_tenant_id,operation_id,execution_id,execution_spec_hash,'ACKED' FROM sys_modeldev_acceptances WHERE id=801`)
			} else {
				_, err = f.gov.Exec(ctx, `INSERT INTO sys_quota_release_receipts(id,tenant_id,receipt_id,owner_service,release_event_id,payload_hash,payload_json) SELECT 602,tenant_id,'additional-original-reference',owner_service,release_event_id,payload_hash,payload_json FROM sys_quota_release_receipts WHERE id=601`)
			}
			if err != nil {
				t.Fatal(err)
			}
			changed, err := f.command.Plan(ctx, "governance", nil)
			prefix := "GOV_ACCEPTANCE_REFERENCED_OUTSIDE_MANIFEST"
			if kind == "release_receipt" {
				prefix = "GOV_RELEASE_RECEIPT_REFERENCED_OUTSIDE_MANIFEST"
			}
			if err != nil || !containsBlocker(changed, prefix) {
				t.Fatal(changed.Blockers, err)
			}
			if _, err = f.command.Apply(ctx, p, nil); err == nil {
				t.Fatal("semantic reference silently left behind")
			}
			f.requireOriginalCounts(t)
			f.assertProtected(t)
		})
	}
}
