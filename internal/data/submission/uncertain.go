package submission

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
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
	tenantID, err := databaseID(permit.TenantID)
	if err != nil {
		return biz.PipelineDispatch{}, err
	}
	executionID, err := databaseID(permit.ExecutionID)
	if err != nil {
		return biz.PipelineDispatch{}, err
	}
	attemptID, err := databaseID(permit.AttemptID)
	if err != nil {
		return biz.PipelineDispatch{}, err
	}
	if len(permit.PlanHash) != 64 || strings.ToLower(permit.PlanHash) != permit.PlanHash {
		return biz.PipelineDispatch{}, biz.ErrInvalidAdmission
	}
	if _, err := hex.DecodeString(permit.PlanHash); err != nil {
		return biz.PipelineDispatch{}, biz.ErrInvalidAdmission
	}
	observedAt = observedAt.UTC()
	if !validUncertaintyTime(observedAt) {
		return biz.PipelineDispatch{}, biz.ErrInvalidAdmission
	}
	if repository == nil || repository.pool == nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	transaction, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	defer rollback(transaction)
	queries := submissionsql.New(transaction)
	identity, err := queries.LockExecutionIdentity(ctx, submissionsql.LockExecutionIdentityParams{TenantID: tenantID, ExecutionID: executionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.PipelineDispatch{}, biz.ErrExecutionNotFound
	}
	if err != nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	row, err := queries.GetPipelineDispatch(ctx, submissionsql.GetPipelineDispatchParams{TenantID: tenantID, ExecutionID: executionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.PipelineDispatch{}, biz.ErrExecutionNotFound
	}
	if err != nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	if row.TenantID != identity.TenantID || row.ExecutionID != identity.ExecutionID || row.OperationID != identity.OperationID || row.SpecHash != identity.SpecHash {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	if row.AttemptID != attemptID || row.PlanHash != permit.PlanHash {
		return biz.PipelineDispatch{}, biz.ErrAdmissionConflict
	}
	admission, err := queries.GetAdmission(ctx, submissionsql.GetAdmissionParams{TenantID: tenantID, ExecutionID: executionID})
	if err != nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	dispatch, err := dispatchFromRow(row, admission)
	if err != nil {
		return biz.PipelineDispatch{}, err
	}
	if observedAt.Before(dispatch.ReservedAt) {
		return biz.PipelineDispatch{}, biz.ErrInvalidAdmission
	}
	if dispatch.State == biz.PipelineDispatchSubmitting {
		row, err = queries.MarkSubmissionUncertain(ctx, submissionsql.MarkSubmissionUncertainParams{
			TenantID: tenantID, ExecutionID: executionID, AttemptID: attemptID, PlanHash: permit.PlanHash,
			ObservedAt: pgtype.Timestamptz{Time: observedAt, Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return biz.PipelineDispatch{}, biz.ErrAdmissionConflict
		}
		if err != nil {
			return biz.PipelineDispatch{}, biz.ErrPersistence
		}
		dispatch, err = dispatchFromRow(row, admission)
		if err != nil {
			return biz.PipelineDispatch{}, err
		}
	}
	// A replay never refreshes the original observation time. A failed or
	// unknown commit cannot be returned as an acknowledged observation.
	if err := transaction.Commit(ctx); err != nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	return dispatch, nil
}

func validUncertaintyTime(value time.Time) bool {
	value = value.UTC()
	return !value.IsZero() && value.Year() >= 1 && value.Year() <= 9999 && value.Nanosecond()%1000 == 0
}
