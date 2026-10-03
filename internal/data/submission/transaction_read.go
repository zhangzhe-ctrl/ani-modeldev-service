package submission

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	submissionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission/sqlc"
)

// ReadInTransaction reads and validates all durable submission facts within the
// caller's aggregate transaction. The caller must hold the execution identity
// lock or use one RepeatableRead snapshot, and owns commit/rollback. This never
// issues a send permit. Only an absent dispatch yields ErrExecutionNotFound;
// inconsistent persisted facts yield ErrPersistence.
func ReadInTransaction(ctx context.Context, transaction pgx.Tx, tenant, execution string) (biz.PipelineDispatch, error) {
	tenantID, err := databaseID(tenant)
	if err != nil {
		return biz.PipelineDispatch{}, err
	}
	executionID, err := databaseID(execution)
	if err != nil {
		return biz.PipelineDispatch{}, err
	}
	return readInTransaction(ctx, transaction, tenantID, executionID)
}

func readInTransaction(ctx context.Context, transaction pgx.Tx, tenantID, executionID pgtype.UUID) (biz.PipelineDispatch, error) {
	if transaction == nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	queries := submissionsql.New(transaction)
	row, err := queries.GetPipelineDispatch(ctx, submissionsql.GetPipelineDispatchParams{TenantID: tenantID, ExecutionID: executionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.PipelineDispatch{}, biz.ErrExecutionNotFound
	}
	if err != nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	admission, err := queries.GetAdmission(ctx, submissionsql.GetAdmissionParams{TenantID: tenantID, ExecutionID: executionID})
	if err != nil {
		return biz.PipelineDispatch{}, biz.ErrPersistence
	}
	return readDispatch(ctx, queries, row, admission)
}
