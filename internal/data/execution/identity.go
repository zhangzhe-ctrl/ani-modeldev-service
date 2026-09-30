package execution

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	executionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution/sqlc"
)

// lockIdentity starts the transaction shared by Admission and close receipts.
// The caller must commit or roll back the returned transaction. Immutable
// identity facts are compared under the same lock before either side writes.
func (r *Repository) lockIdentity(ctx context.Context, identity executionsql.InsertExecutionIdentityParams) (pgx.Tx, *executionsql.Queries, error) {
	transaction, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, nil, biz.ErrPersistence
	}
	queries := executionsql.New(transaction)
	if err := queries.InsertExecutionIdentity(ctx, identity); err != nil {
		rollbackExecutionTransaction(transaction)
		return nil, nil, biz.ErrPersistence
	}
	stored, err := queries.LockExecutionIdentity(ctx, executionsql.LockExecutionIdentityParams{
		TenantID: identity.TenantID, ExecutionID: identity.ExecutionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		rollbackExecutionTransaction(transaction)
		return nil, nil, biz.ErrAdmissionConflict
	}
	if err != nil {
		rollbackExecutionTransaction(transaction)
		return nil, nil, biz.ErrPersistence
	}
	if stored.TenantID != identity.TenantID || stored.ExecutionID != identity.ExecutionID || stored.OperationID != identity.OperationID || stored.SpecHash != identity.SpecHash {
		rollbackExecutionTransaction(transaction)
		return nil, nil, biz.ErrAdmissionConflict
	}
	return transaction, queries, nil
}

func rollbackExecutionTransaction(transaction pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = transaction.Rollback(ctx)
}
