package runtimeproof_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/trainer"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func missingTrainingLogsFixture(t *testing.T) (*computeCleanupFixture, *biz.ExecutionOperations, biz.ExecutionRuntime, *biz.CleanupMissingTrainingLogsReview, *unstructured.Unstructured) {
	t.Helper()
	f := newComputeCleanupClosingFixture(t, false, "DEADLINE")
	observer := &configuredCleanupObserver{TrainingRuntime: trainer.New(f.kube), err: biz.ErrRuntimeNotReady}
	f.step.proof.WithTrainingRuntime(observer)
	ctx := context.Background()
	for range 3 {
		if result, err := f.worker(t).ReconcileOnce(ctx); err != nil || result.Unresolved != 1 {
			t.Fatal("original three failed captures and exhausted cap", result, err)
		}
	}
	operations := retryOperations(t, f)
	_, _ = operations.Reconcile(ctx, f.step.f.execution.TenantID, f.step.f.execution.ExecutionID)
	state, err := lifecycle.New(f.step.open()).GetRuntime(ctx, f.step.f.execution.TenantID, f.step.f.execution.ExecutionID)
	if err != nil || state.ComputeCleanup == nil || state.ComputeCleanup.Attempts != 3 || state.ComputeCleanup.LastError != "ATTEMPTS_EXHAUSTED" || state.ComputeCleanup.LogsReady || len(state.ComputeCleanup.Requested) != 0 {
		t.Fatal("original unavailable-log failure scope", state.ComputeCleanup, err)
	}
	review := &biz.CleanupMissingTrainingLogsReview{ExecutionID: f.step.f.execution.ExecutionID, CreateOperationID: f.step.f.execution.OperationID, CloseOperationID: state.CloseOperationID, AutomaticCleanupPlanSHA256: state.ComputeCleanup.PlanHash}
	var lostPod *unstructured.Unstructured
	for key, pod := range f.objects {
		if pod.GetKind() == "Pod" {
			lostPod = pod.DeepCopy()
			for _, fact := range state.Observation.Resources {
				if fact.UID == string(pod.GetUID()) {
					if fact.ExitCode == nil || *fact.ExitCode != 143 {
						t.Fatal("real deadline exit is not 143", fact)
					}
					review.MissingPods = append(review.MissingPods, biz.MissingTrainingLogPod{PodUID: fact.UID, PersistedExitCode: *fact.ExitCode})
				}
			}
			// The historical bug lost this original log source to API/GC.
			delete(f.objects, key)
			delete(f.step.pods, pod.GetName())
		}
	}
	observer.err = nil
	return f, operations, state, review, lostPod
}

func TestReviewedMissingTrainingLogsCleanupPreservesFailedAutomaticAudit(t *testing.T) {
	f, operations, before, review, _ := missingTrainingLogsFixture(t)
	ctx := context.Background()
	tenant, id := f.step.f.execution.TenantID, f.step.f.execution.ExecutionID
	if _, err := operations.PlanCleanup(ctx, tenant, id); err == nil {
		t.Fatal("unreviewed missing logs authorized disposal")
	}
	plan, err := operations.PlanCleanupWithMissingLogsReview(ctx, tenant, id, review)
	if err != nil {
		t.Fatalf("REVIEWED_MISSING_LOG_MAINTENANCE_NOT_IMPLEMENTED: %v", err)
	}
	if len(plan.Targets) != 7 {
		t.Fatal("review changed original seven-UID scope", plan.Targets)
	}
	manual, err := operations.ApplyCleanupWithMissingLogsReview(ctx, tenant, id, plan.PlanHash, "governance:reviewed-log-loss-operator", review)
	if err != nil || manual.Phase != "APPLIED" || len(manual.Requested) != 7 || len(manual.ConfirmedAbsent) != 7 || len(f.deleted) != 6 || f.logReads != 0 {
		t.Fatal("reviewed exact-UID disposal did not complete without inventing logs", manual, f.deleted, f.logReads, err)
	}
	after, err := lifecycle.New(f.step.open()).GetRuntime(ctx, tenant, id)
	if err != nil || !reflect.DeepEqual(before.ComputeCleanup, after.ComputeCleanup) || after.CloseOperationID != before.CloseOperationID || after.CloseReason != "DEADLINE" || !after.ClosedAt.Equal(*before.ClosedAt) {
		t.Fatal("review overwrote original failure, deadline or close identity", err)
	}
	if after.ReviewedComputeCleanup == nil || !biz.ComputeResourcesSettledAt(after).Equal(manual.CompletedAt) || !biz.ComputeCleanupSettledAt(after.ComputeCleanup).IsZero() {
		t.Fatal("manual resource settlement was not independently derived")
	}
	raw, _ := json.Marshal(after)
	if strings.Contains(string(raw), "ReviewedComputeCleanup") {
		t.Fatal("derived audit was copied into runtime JSON")
	}
	record, err := execution.New(f.step.open()).GetOperationRecord(ctx, tenant, id)
	if err != nil || biz.ComputeCleanupApplied(record) || !biz.ReviewedComputeCleanupApplied(record) {
		t.Fatal("missing logs were falsely marked complete or disposal lost", err)
	}
	if _, err := operations.ReconcileWithActor(ctx, tenant, id, "governance:reviewed-log-loss-operator"); err != nil {
		t.Fatal("normal post-maintenance observation", err)
	}
	final, err := lifecycle.New(f.step.open()).GetRuntime(ctx, tenant, id)
	if err != nil || !reflect.DeepEqual(before.ComputeCleanup, final.ComputeCleanup) || len(f.deleted) != 6 || f.logReads != 0 {
		t.Fatal("normal reconcile retried auto/delete or fabricated logs", err)
	}
	replay, err := operations.ApplyCleanupWithMissingLogsReview(ctx, tenant, id, plan.PlanHash, "governance:reviewed-log-loss-operator", review)
	if err != nil || !reflect.DeepEqual(replay, manual) || len(f.deleted) != 6 {
		t.Fatal("reviewed apply replay changed the original audit", err)
	}
	pool := f.step.open()
	defer pool.Close()
	if _, err := pool.Exec(ctx, "UPDATE modeldev_execution_cleanup_audits SET receipt=jsonb_set(receipt,'{missing_training_logs_review,close_operation_id}',to_jsonb($3::text)) WHERE tenant_id=$1::uuid AND execution_id=$2::uuid", tenant, id, "11111111-1111-4111-8111-111111111111"); err != nil {
		t.Fatal("isolated corruption fixture", err)
	}
	if _, err := lifecycle.New(f.step.open()).GetRuntime(ctx, tenant, id); err == nil {
		t.Fatal("changed manual audit was accepted as resource settlement")
	}
}

func TestReviewedMissingTrainingLogsRejectsChangedScopeBeforeAudit(t *testing.T) {
	f, operations, before, review, lostPod := missingTrainingLogsFixture(t)
	ctx := context.Background()
	tenant, id := f.step.f.execution.TenantID, f.step.f.execution.ExecutionID
	clone := func() *biz.CleanupMissingTrainingLogsReview {
		b, _ := json.Marshal(review)
		var v biz.CleanupMissingTrainingLogsReview
		_ = json.Unmarshal(b, &v)
		return &v
	}
	for _, change := range []func(*biz.CleanupMissingTrainingLogsReview){
		func(r *biz.CleanupMissingTrainingLogsReview) { r.ExecutionID = "11111111-1111-4111-8111-111111111111" },
		func(r *biz.CleanupMissingTrainingLogsReview) {
			r.CreateOperationID = "11111111-1111-4111-8111-111111111111"
		},
		func(r *biz.CleanupMissingTrainingLogsReview) {
			r.CloseOperationID = "11111111-1111-4111-8111-111111111111"
		},
		func(r *biz.CleanupMissingTrainingLogsReview) { r.AutomaticCleanupPlanSHA256 = strings.Repeat("a", 64) },
		func(r *biz.CleanupMissingTrainingLogsReview) { r.MissingPods[0].PodUID = "foreign-original" },
		func(r *biz.CleanupMissingTrainingLogsReview) { r.MissingPods[0].PersistedExitCode = 0 },
		func(r *biz.CleanupMissingTrainingLogsReview) { r.MissingPods = append(r.MissingPods, r.MissingPods[0]) },
		func(r *biz.CleanupMissingTrainingLogsReview) { r.MissingPods = nil },
	} {
		invalid := clone()
		change(invalid)
		if _, err := operations.PlanCleanupWithMissingLogsReview(ctx, tenant, id, invalid); err == nil {
			t.Fatal("changed missing-log review authorized a plan")
		}
	}
	plan, err := operations.PlanCleanupWithMissingLogsReview(ctx, tenant, id, review)
	if err != nil {
		t.Fatal(err)
	}
	invalid := clone()
	invalid.MissingPods[0].PersistedExitCode = 0
	if _, err := operations.ApplyCleanupWithMissingLogsReview(ctx, tenant, id, plan.PlanHash, "governance:reviewed-log-loss-operator", invalid); err == nil {
		t.Fatal("apply changed reviewed exit")
	}
	lostPod.SetUID("55555555-5555-4555-8555-555555555555")
	path := "/api/v1/namespaces/" + lostPod.GetNamespace() + "/pods/" + lostPod.GetName()
	f.objects[path] = lostPod
	f.step.pods[lostPod.GetName()] = lostPod
	if _, err := operations.PlanCleanupWithMissingLogsReview(ctx, tenant, id, review); err == nil {
		t.Fatal("same-name replacement became proof of missing original log")
	}
	record, err := execution.New(f.step.open()).GetOperationRecord(ctx, tenant, id)
	if err != nil || record.Cleanup != nil || !reflect.DeepEqual(before.ComputeCleanup, record.Runtime.ComputeCleanup) || len(f.deleted) != 0 || f.logReads != 0 {
		t.Fatal("invalid review changed original audit or resources", err)
	}
}

func TestReviewedMissingTrainingLogsUnknownApplyResolvesByReadsOnly(t *testing.T) {
	f, operations, before, review, _ := missingTrainingLogsFixture(t)
	ctx := context.Background()
	tenant, id := f.step.f.execution.TenantID, f.step.f.execution.ExecutionID
	plan, err := operations.PlanCleanupWithMissingLogsReview(ctx, tenant, id, review)
	if err != nil {
		t.Fatal(err)
	}
	f.loseDelete, f.loseDeleteKind = true, "TrainingRuntime"
	manual, err := operations.ApplyCleanupWithMissingLogsReview(ctx, tenant, id, plan.PlanHash, "governance:reviewed-log-loss-operator", review)
	if err == nil || manual.Phase != "NEEDS_REVIEW" || len(f.deleted) != 6 {
		t.Fatal("lost final response falsely completed apply", manual, err)
	}
	if _, err := operations.ApplyCleanupWithMissingLogsReview(ctx, tenant, id, plan.PlanHash, "governance:reviewed-log-loss-operator", review); err == nil || len(f.deleted) != 6 {
		t.Fatal("unknown apply was replayed", err)
	}
	if _, err := operations.ReconcileWithActor(ctx, tenant, id, "governance:reviewed-log-loss-operator"); err != nil {
		t.Fatal("normal read-only reconciliation of exact absent originals", err)
	}
	record, err := execution.New(f.step.open()).GetOperationRecord(ctx, tenant, id)
	if err != nil || record.Cleanup == nil || record.Cleanup.Phase != "RECONCILED" || !biz.ReviewedComputeCleanupApplied(record) || !reflect.DeepEqual(before.ComputeCleanup, record.Runtime.ComputeCleanup) || len(f.deleted) != 6 || f.logReads != 0 {
		t.Fatal("resolution rewrote auto audit or reissued effects", record.Cleanup, err)
	}
}
