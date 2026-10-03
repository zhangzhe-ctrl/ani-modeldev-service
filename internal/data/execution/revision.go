package execution

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	executionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution/sqlc"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/pgvalue"
)

func currentOwnerRevision(ctx context.Context, queries *executionsql.Queries, tenantID, executionID pgtype.UUID) (uint64, error) {
	value, err := queries.GetOwnerRevision(ctx, executionsql.GetOwnerRevisionParams{TenantID: tenantID, ExecutionID: executionID})
	return committedOwnerRevision(value, err)
}

// A caller must hold the shared identity lock and have established a new fact.
// Saturation returns no row; it must roll back that fact, never wrap the counter.
func advanceOwnerRevision(ctx context.Context, queries *executionsql.Queries, tenantID, executionID pgtype.UUID) (uint64, error) {
	value, err := queries.AdvanceOwnerRevision(ctx, executionsql.AdvanceOwnerRevisionParams{TenantID: tenantID, ExecutionID: executionID})
	return committedOwnerRevision(value, err)
}

func committedOwnerRevision(value pgtype.Numeric, err error) (uint64, error) {
	if err != nil {
		return 0, biz.ErrPersistence
	}
	revision, ok := pgvalue.Uint64(value)
	if !ok || revision == 0 {
		// Zero is only the starting point of an uncommitted new identity.
		return 0, biz.ErrPersistence
	}
	return revision, nil
}
