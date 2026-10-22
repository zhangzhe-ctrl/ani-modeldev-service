package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
)

// Derive only from the existing separate operator audit. The original automatic
// failure and runtime JSON are never rewritten to manufacture successful logs.
func loadReviewedComputeCleanup(ctx context.Context, current *runtimeTransaction) error {
	state := current.state
	if state.ComputeCleanup == nil || state.ComputeCleanup.LogsReady || state.ComputeCleanup.Phase != "NEEDS_REVIEW" || state.ComputeCleanup.Attempts != biz.ComputeCleanupMaximumAttempts {
		return nil
	}
	var rawPlan, rawReceipt []byte
	var hash, phase, actor string
	var started time.Time
	var completed pgtype.Timestamptz
	err := current.transaction.QueryRow(ctx, `SELECT plan,receipt,plan_sha256,phase,actor,started_at,completed_at
FROM modeldev_execution_cleanup_audits WHERE tenant_id=$1 AND execution_id=$2
ORDER BY started_at DESC,plan_sha256 DESC LIMIT 1`, current.tenantID, current.executionID).Scan(&rawPlan, &rawReceipt, &hash, &phase, &actor, &started, &completed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return biz.ErrPersistence
	}
	var plan biz.CleanupPlan
	var receipt biz.CleanupReceipt
	if json.Unmarshal(rawPlan, &plan) != nil || json.Unmarshal(rawReceipt, &receipt) != nil {
		return biz.ErrPersistence
	}
	if plan.MissingTrainingLogsReview == nil && receipt.MissingTrainingLogsReview == nil {
		return nil
	}
	if !reflect.DeepEqual(plan.MissingTrainingLogsReview, receipt.MissingTrainingLogsReview) || receipt.PlanHash != hash || plan.PlanHash != hash || receipt.Phase != phase || receipt.Actor != actor || !receipt.StartedAt.Equal(started) || receipt.ReviewedOwnerRevision != plan.OwnerRevision || receipt.StartedAt.After(current.now) {
		return biz.ErrPersistence
	}
	if phase != "APPLIED" && phase != "RECONCILED" {
		return nil
	}
	if completed.Valid != !receipt.CompletedAt.IsZero() || completed.Valid && (!completed.Time.Equal(receipt.CompletedAt) || completed.Time.After(current.now)) || phase == "APPLIED" && !completed.Valid || phase == "RECONCILED" && receipt.ReconciledAt.After(current.now) {
		return biz.ErrPersistence
	}
	owner, err := submission.ReadAuthorityInTransaction(ctx, current.transaction, current.execution.TenantID, current.execution.ExecutionID)
	candidate := owner.RunAuthorityCandidate
	if errors.Is(err, biz.ErrExecutionNotFound) && state.CloseAuthority != nil {
		candidate, err = *state.CloseAuthority, nil
	}
	if err != nil {
		return biz.ErrPersistence
	}
	execution := current.execution
	execution.States, err = biz.ProjectRuntimeStates(execution.States, state)
	if err != nil {
		return biz.ErrPersistence
	}
	record := biz.OperationRecord{QueryRecord: biz.QueryRecord{Execution: execution, Runtime: state}, Authority: &candidate, Cleanup: &receipt}
	expected, err := biz.CleanupPlanFor(record)
	if err != nil || !reflect.DeepEqual(expected, plan) || expected.PlanHash != hash || !biz.ReviewedComputeCleanupApplied(record) {
		return biz.ErrPersistence
	}
	current.state.ReviewedComputeCleanup = &receipt
	return nil
}
