package submission

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	submissionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission/sqlc"
)

// RecordSubmissionConfirmed stores an internal observation of the original
// attempt. It cannot authenticate a KFP response or establish Run authority.
// Multiple observed Runs remain separate facts, including after close.
func (repository *Repository) RecordSubmissionConfirmed(ctx context.Context, permit biz.PipelineSendPermit, observation biz.PipelineSubmissionObservation, observedAt time.Time) (biz.PipelineConfirmationReceipt, error) {
	if observation.State != biz.PipelineSubmissionConfirmed {
		return biz.PipelineConfirmationReceipt{}, biz.ErrInvalidAdmission
	}
	runID, err := databaseID(observation.RunID)
	if err != nil {
		return biz.PipelineConfirmationReceipt{}, err
	}
	locked, err := repository.lockObservation(ctx, permit, observedAt)
	if err != nil {
		return biz.PipelineConfirmationReceipt{}, err
	}
	defer rollback(locked.transaction)
	inserted, err := locked.queries.InsertConfirmedPipelineRun(ctx, submissionsql.InsertConfirmedPipelineRunParams{
		TenantID: locked.row.TenantID, ExecutionID: locked.row.ExecutionID, AttemptID: locked.row.AttemptID, PlanHash: locked.row.PlanHash,
		RunID: runID, ObservedAt: pgtype.Timestamptz{Time: observedAt.UTC(), Valid: true},
	})
	if err != nil || inserted < 0 || inserted > 1 {
		return biz.PipelineConfirmationReceipt{}, biz.ErrPersistence
	}
	row := locked.row
	if locked.dispatch.State != biz.PipelineDispatchConfirmed {
		row, err = locked.queries.MarkSubmissionConfirmed(ctx, submissionsql.MarkSubmissionConfirmedParams{
			TenantID: row.TenantID, ExecutionID: row.ExecutionID, AttemptID: row.AttemptID, PlanHash: row.PlanHash,
		})
		if err != nil {
			return biz.PipelineConfirmationReceipt{}, biz.ErrPersistence
		}
	}
	dispatch, err := readDispatch(ctx, locked.queries, row, locked.admission)
	if err != nil {
		return biz.PipelineConfirmationReceipt{}, err
	}
	found := false
	for _, run := range dispatch.ConfirmedRuns {
		if run.RunID == runID.String() {
			found = true
			break
		}
	}
	if !found {
		return biz.PipelineConfirmationReceipt{}, biz.ErrPersistence
	}
	// A failed or unknown commit cannot acknowledge persistence. A distinct
	// second Run is committed and reported as a conflict, never rolled back or
	// selected as an authoritative replacement for an earlier observation.
	if err := locked.transaction.Commit(ctx); err != nil {
		return biz.PipelineConfirmationReceipt{}, biz.ErrPersistence
	}
	return biz.PipelineConfirmationReceipt{Dispatch: dispatch, ConflictingRuns: len(dispatch.ConfirmedRuns) > 1}, nil
}
