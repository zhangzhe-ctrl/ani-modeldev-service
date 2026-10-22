package runtimeproof_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
)

func partiallyRemovedCompute(t *testing.T) (*computeCleanupFixture, *biz.ExecutionOperations, biz.ExecutionRuntime) {
	t.Helper()
	f, _, operations := exhaustedCleanup(t)
	f.loseDelete, f.loseDeleteKind = true, "Pod"
	if err := explicitRetry(t, operations, f); err == nil {
		t.Fatal("lost Pod response must retain uncertainty")
	}
	state, err := lifecycle.New(f.step.open()).GetRuntime(context.Background(), f.step.f.execution.TenantID, f.step.f.execution.ExecutionID)
	if err != nil || state.ComputeCleanup.Attempts != 4 || state.ComputeCleanup.Phase != "NEEDS_REVIEW" || !state.ComputeCleanup.LogsReady || len(state.ComputeCleanup.ConfirmedAbsent) != 3 || len(state.ComputeCleanup.Requested) != 7 || len(f.deleted) != 4 {
		t.Fatal("original fourth-attempt partial deletion fixture", state.ComputeCleanup, err)
	}
	return f, operations, state
}

func TestPartialComputeCleanupUsesIndependentExplicitMaintenanceAndReadOnlyResolution(t *testing.T) {
	f, operations, before := partiallyRemovedCompute(t)
	ctx := context.Background()
	tenant, id := f.step.f.execution.TenantID, f.step.f.execution.ExecutionID
	plan, err := operations.PlanCleanup(ctx, tenant, id)
	if err != nil {
		t.Fatalf("PARTIAL_COMPUTE_MAINTENANCE_NOT_IMPLEMENTED: %v", err)
	}
	if len(plan.Targets) != 4 {
		t.Fatal("maintenance did not preserve original unconfirmed scope", plan.Targets)
	}
	manual, err := operations.ApplyCleanup(ctx, tenant, id, plan.PlanHash, "governance:maintenance-operator-3")
	if err != nil || manual.Phase != "APPLIED" || len(manual.Requested) != 4 || len(f.deleted) != 7 {
		t.Fatal("independent normal operator maintenance did not finish exact originals", manual, f.deleted, err)
	}
	state, err := lifecycle.New(f.step.open()).GetRuntime(ctx, tenant, id)
	if err != nil || !reflect.DeepEqual(state.ComputeCleanup, before.ComputeCleanup) {
		t.Fatal("maintenance overwrote original automatic audit", err)
	}
	if err := explicitRetry(t, operations, f); err != nil {
		t.Fatal("normal read-only reconciliation", err)
	}
	final, err := lifecycle.New(f.step.open()).GetRuntime(ctx, tenant, id)
	r := final.ComputeCleanup
	if err != nil || r.Phase != "RECONCILED" || r.Attempts != 4 || r.LastError != before.ComputeCleanup.LastError || !r.CompletedAt.IsZero() || r.ReconciledAt.IsZero() || r.ReconciledFromPhase != "NEEDS_REVIEW" || len(r.ResolvedAbsent) != 7 || !reflect.DeepEqual(r.ConfirmedAbsent, before.ComputeCleanup.ConfirmedAbsent) || !reflect.DeepEqual(r.OperatorRetry, before.ComputeCleanup.OperatorRetry) || len(f.deleted) != 7 {
		t.Fatal("observation erased failed attempt or granted another budget", r, err)
	}
	record, err := execution.New(f.step.open()).GetOperationRecord(ctx, tenant, id)
	if err != nil || !biz.ComputeCleanupApplied(record) {
		t.Fatal("strict observed resolution cannot authorize original owner proof", err)
	}
	if err := f.step.proof.VerifyRecordedComputeWritersAbsent(ctx, record.Execution, *record.Authority, record.Runtime); err != nil {
		t.Fatal("original independent release proof rejected all original UIDs absent", err)
	}
	if err := explicitRetry(t, operations, f); err != nil {
		t.Fatal("completed resolution replay", err)
	}
	again, _ := lifecycle.New(f.step.open()).GetRuntime(ctx, tenant, id)
	if !reflect.DeepEqual(again.ComputeCleanup, final.ComputeCleanup) || len(f.deleted) != 7 {
		t.Fatal("resolution replay changed audit or resent DELETE")
	}
}

func TestPartialComputeMaintenanceRejectsReplacementUIDBeforeNewAuditOrDelete(t *testing.T) {
	f, operations, before := partiallyRemovedCompute(t)
	f.step.f.workflow.SetUID("replacement-workflow")
	if _, err := operations.PlanCleanup(context.Background(), f.step.f.execution.TenantID, f.step.f.execution.ExecutionID); err == nil {
		t.Fatal("replacement Workflow was authorized for maintenance")
	}
	after, err := lifecycle.New(f.step.open()).GetRuntime(context.Background(), f.step.f.execution.TenantID, f.step.f.execution.ExecutionID)
	if err != nil || !reflect.DeepEqual(after.ComputeCleanup, before.ComputeCleanup) || len(f.deleted) != 4 {
		t.Fatal("failed maintenance preflight changed original scope", err)
	}
}
