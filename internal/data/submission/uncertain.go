package submission

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	submissionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission/sqlc"
)

// MarkSubmissionUncertain records the original attempt's observation under
// the shared identity lock. Close or deadline cannot discard a late outcome,
// and neither a new observation nor its replay authorizes another send.
func (repository *Repository) MarkSubmissionUncertain(ctx context.Context, permit biz.PipelineSendPermit, observedAt time.Time) (biz.PipelineDispatch, error) {
	locked, err := repository.lockObservation(ctx, permit, observedAt)
	if err != nil {
		return biz.PipelineDispatch{}, err
	}
	defer rollback(locked.transaction)
	dispatch := locked.dispatch
	if dispatch.State == biz.PipelineDispatchSubmitting || dispatch.State == biz.PipelineDispatchNotSent {
		row, err := locked.queries.MarkSubmissionUncertain(ctx, submissionsql.MarkSubmissionUncertainParams{
			TenantID: locked.row.TenantID, ExecutionID: locked.row.ExecutionID, AttemptID: locked.row.AttemptID, PlanHash: locked.row.PlanHash,
			ObservedAt: pgtype.Timestamptz{Time: observedAt.UTC(), Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return biz.PipelineDispatch{}, biz.ErrAdmissionConflict
		}
		if err != nil {
			return biz.PipelineDispatch{}, biz.ErrPersistence
		}
		dispatch, err = readDispatch(ctx, locked.queries, row, locked.admission)
		if err != nil {
			return biz.PipelineDispatch{}, err
		}
		dispatch.OwnerRevision, err = advanceOwnerRevision(ctx, locked.queries, row.TenantID, row.ExecutionID)
		if err != nil {
			return biz.PipelineDispatch{}, err
		}
	}
	// Replay preserves the first uncertainty time; a confirmed dispatch also
	// retains every handle without downgrade or a new uncertainty timestamp.
	if err := locked.transaction.Commit(ctx); err != nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	return dispatch, nil
}
