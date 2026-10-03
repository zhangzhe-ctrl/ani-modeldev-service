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

// MarkSubmissionNotSent keeps the original attempt's first local no-send
// observation. Stronger observations and late Run handles are never erased;
// neither the original result nor a replay reconstructs permission to send.
func (repository *Repository) MarkSubmissionNotSent(ctx context.Context, permit biz.PipelineSendPermit, observedAt time.Time) (biz.PipelineDispatch, error) {
	locked, err := repository.lockObservation(ctx, permit, observedAt)
	if err != nil {
		return biz.PipelineDispatch{}, err
	}
	defer rollback(locked.transaction)
	dispatch := locked.dispatch
	if dispatch.NotSentAt == nil {
		row, err := locked.queries.MarkSubmissionNotSent(ctx, submissionsql.MarkSubmissionNotSentParams{
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
	// Replays keep the first committed no-send time even when the new input
	// time is earlier. Close/deadline do not discard this original-attempt fact.
	if err := locked.transaction.Commit(ctx); err != nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	return dispatch, nil
}
