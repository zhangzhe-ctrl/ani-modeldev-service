package submission

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/pgvalue"
	submissionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission/sqlc"
)

func readOwnerRevision(ctx context.Context, queries *submissionsql.Queries, tenantID, executionID pgtype.UUID) (uint64, error) {
	value, err := queries.GetOwnerRevision(ctx, submissionsql.GetOwnerRevisionParams{TenantID: tenantID, ExecutionID: executionID})
	if err != nil {
		return 0, biz.ErrPersistence
	}
	return positiveOwnerRevision(value)
}

// Call only while holding the execution identity lock, after establishing a
// new fact in this transaction. Saturation fails the entire transaction; it
// cannot leave a new observation at the previous aggregate version.
func advanceOwnerRevision(ctx context.Context, queries *submissionsql.Queries, tenantID, executionID pgtype.UUID) (uint64, error) {
	value, err := queries.AdvanceOwnerRevision(ctx, submissionsql.AdvanceOwnerRevisionParams{TenantID: tenantID, ExecutionID: executionID})
	if err != nil {
		return 0, biz.ErrPersistence
	}
	return positiveOwnerRevision(value)
}

func positiveOwnerRevision(value pgtype.Numeric) (uint64, error) {
	revision, ok := pgvalue.Uint64(value)
	if !ok || revision == 0 {
		return 0, biz.ErrPersistence
	}
	return revision, nil
}
