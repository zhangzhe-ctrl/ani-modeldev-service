package submission

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	submissionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission/sqlc"
)

// ReadAuthorityInTransaction shares the caller's snapshot/identity lock and
// validates the exact same association as GetRunAuthority.
func ReadAuthorityInTransaction(ctx context.Context, transaction pgx.Tx, tenant, execution string) (biz.RunAuthority, error) {
	tenantID, err := databaseID(tenant)
	if err != nil {
		return biz.RunAuthority{}, err
	}
	executionID, err := databaseID(execution)
	if err != nil || transaction == nil || ctx == nil {
		return biz.RunAuthority{}, biz.ErrInvalidAdmission
	}
	row, err := submissionsql.New(transaction).GetRunAuthorityRow(ctx, submissionsql.GetRunAuthorityRowParams{TenantID: tenantID, ExecutionID: executionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.RunAuthority{}, biz.ErrExecutionNotFound
	}
	if err != nil {
		return biz.RunAuthority{}, biz.ErrPersistence
	}
	dispatch, err := ReadInTransaction(ctx, transaction, tenant, execution)
	if err != nil {
		return biz.RunAuthority{}, err
	}
	return runAuthorityFromRow(row, dispatch)
}
