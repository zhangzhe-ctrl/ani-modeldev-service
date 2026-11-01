package main

import (
	"context"
	"testing"
)

// The approved seam is the maintenance command's plan/apply boundary over real
// PostgreSQL. External cleanup is supplied only through explicit root receipts.
func TestApplyWithoutRootQualificationPreservesOriginalRows(t *testing.T) {
	fixture := newCleanupFixture(t)
	plan, err := fixture.command.Plan(context.Background(), "governance", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsBlocker(plan, "ROOT_EVIDENCE_UNKNOWN") {
		t.Fatal("unknown external qualification must be BLOCKED")
	}
	if _, err = fixture.command.Apply(context.Background(), plan, nil); err == nil {
		t.Fatal("unqualified deletion accepted")
	}
	fixture.requireOriginalCounts(t)
}

func TestExactTwoPhaseDeletionPreservesOtherTenantOwnerAndAccounts(t *testing.T) {
	f := newCleanupFixture(t)
	f.qualifyRoot(t)
	ctx := context.Background()
	plan, err := f.command.Plan(ctx, "governance", nil)
	if err != nil || len(plan.Blockers) != 0 {
		t.Fatalf("qualified first-phase plan: %+v %v", plan.Blockers, err)
	}
	receipt, err := f.command.Apply(ctx, plan, nil)
	if err != nil || !receipt.Applied {
		t.Fatalf("first-phase delete %+v %v", receipt, err)
	}
	if receipt.Deleted["sys_modeldev_acceptances"] != 1 || receipt.Deleted["sys_quota_charges"] != 1 || receipt.Deleted["sys_quota_operations"] != 1 || receipt.Deleted["sys_quota_release_receipts"] != 1 {
		t.Fatalf("exact original delete counts %+v", receipt.Deleted)
	}
	f.assertProtected(t)
	f.command.GovernanceReceiptSHA256 = digest(receipt)
	next, err := f.command.Plan(ctx, "modeldev", &receipt)
	if err != nil || len(next.Blockers) != 0 {
		t.Fatalf("qualified MD phase: %+v %v", next.Blockers, err)
	}
	removed, err := f.command.Apply(ctx, next, &receipt)
	if err != nil || !removed.Applied {
		t.Fatalf("second-phase delete: %+v %v", removed, err)
	}
	if removed.Deleted["modeldev_executions"] != 1 || removed.Deleted["modeldev_execution_identities"] != 1 || removed.Deleted["modeldev_execution_runtimes"] != 1 || removed.Deleted["modeldev_close_intents"] != 1 {
		t.Fatalf("nine-table exact counts %+v", removed.Deleted)
	}
	f.assertProtected(t)
}
