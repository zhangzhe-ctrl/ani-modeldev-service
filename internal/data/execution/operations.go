package execution

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	executionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution/sqlc"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
)

func (repository *Repository) GetOperationRecord(ctx context.Context, tenant, execution string) (biz.OperationRecord, error) {
	if ctx == nil || repository == nil || repository.pool == nil {
		return biz.OperationRecord{}, biz.ErrPersistence
	}
	transaction, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return biz.OperationRecord{}, biz.ErrPersistence
	}
	defer rollbackExecutionTransaction(transaction)
	record, err := operationRecord(ctx, transaction, tenant, execution)
	if err != nil {
		return biz.OperationRecord{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return biz.OperationRecord{}, biz.ErrPersistence
	}
	return record, nil
}

func operationRecord(ctx context.Context, transaction pgx.Tx, tenant, execution string) (biz.OperationRecord, error) {
	tenantID, err := databaseID(tenant)
	if err != nil {
		return biz.OperationRecord{}, err
	}
	executionID, err := databaseID(execution)
	if err != nil {
		return biz.OperationRecord{}, err
	}
	query, err := queryRecord(ctx, transaction, tenantID, executionID)
	if err != nil {
		return biz.OperationRecord{}, err
	}
	record := biz.OperationRecord{QueryRecord: query}
	var audit []byte
	err = transaction.QueryRow(ctx, "SELECT receipt FROM modeldev_execution_cleanup_audits WHERE tenant_id=$1 AND execution_id=$2 ORDER BY started_at DESC, plan_sha256 DESC LIMIT 1", tenantID, executionID).Scan(&audit)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return biz.OperationRecord{}, biz.ErrPersistence
	}
	if err == nil {
		var receipt biz.CleanupReceipt
		if json.Unmarshal(audit, &receipt) != nil || receipt.ExecutionID != execution {
			return biz.OperationRecord{}, biz.ErrPersistence
		}
		record.Cleanup = &receipt
	}
	dispatch, err := submission.ReadInTransaction(ctx, transaction, tenant, execution)
	if errors.Is(err, biz.ErrExecutionNotFound) {
		return record, nil
	}
	if err != nil || dispatch.OwnerRevision != query.Execution.OwnerRevision {
		return biz.OperationRecord{}, biz.ErrPersistence
	}
	record.Dispatch = &dispatch
	authority, err := submission.ReadAuthorityInTransaction(ctx, transaction, tenant, execution)
	if errors.Is(err, biz.ErrExecutionNotFound) {
		record.Authority = query.Runtime.CloseAuthority
		return record, nil
	}
	if err != nil || authority.OwnerRevision != query.Execution.OwnerRevision {
		return biz.OperationRecord{}, biz.ErrPersistence
	}
	record.Authority = &authority.RunAuthorityCandidate
	return record, nil
}

func (repository *Repository) ReserveCleanup(ctx context.Context, plan biz.CleanupPlan, actor string) (biz.CleanupReceipt, bool, error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return biz.CleanupReceipt{}, false, biz.ErrPersistence
	}
	defer rollbackExecutionTransaction(transaction)
	if err := lockOperation(ctx, transaction, plan.TenantID, plan.ExecutionID); err != nil {
		return biz.CleanupReceipt{}, false, err
	}
	record, err := operationRecord(ctx, transaction, plan.TenantID, plan.ExecutionID)
	if err != nil {
		return biz.CleanupReceipt{}, false, err
	}
	current, err := biz.CleanupPlanFor(record)
	if err != nil || current.PlanHash != plan.PlanHash {
		return biz.CleanupReceipt{}, false, biz.ErrCleanupConflict
	}
	var previous []byte
	err = transaction.QueryRow(ctx, "SELECT receipt FROM modeldev_execution_cleanup_audits WHERE tenant_id=$1::uuid AND execution_id=$2::uuid AND plan_sha256=$3", plan.TenantID, plan.ExecutionID, plan.PlanHash).Scan(&previous)
	if err == nil {
		var receipt biz.CleanupReceipt
		if json.Unmarshal(previous, &receipt) != nil {
			return biz.CleanupReceipt{}, false, biz.ErrPersistence
		}
		return receipt, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return biz.CleanupReceipt{}, false, biz.ErrPersistence
	}
	var now time.Time
	if err := transaction.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return biz.CleanupReceipt{}, false, biz.ErrPersistence
	}
	receipt := biz.CleanupReceipt{ExecutionID: plan.ExecutionID, PlanHash: plan.PlanHash, Phase: "STARTED", Actor: actor, StartedAt: now.UTC(), Requested: []biz.RuntimeResource{}, ConfirmedAbsent: []biz.RuntimeResource{}}
	planBytes, err := json.Marshal(plan)
	if err != nil {
		return biz.CleanupReceipt{}, false, biz.ErrPersistence
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return biz.CleanupReceipt{}, false, biz.ErrPersistence
	}
	_, err = transaction.Exec(ctx, "INSERT INTO modeldev_execution_cleanup_audits (tenant_id,execution_id,plan_sha256,actor,phase,plan,receipt,started_at) VALUES ($1::uuid,$2::uuid,$3,$4,'STARTED',$5::jsonb,$6::jsonb,$7)", plan.TenantID, plan.ExecutionID, plan.PlanHash, actor, planBytes, encoded, now)
	if err != nil {
		return biz.CleanupReceipt{}, false, biz.ErrPersistence
	}
	if err := transaction.Commit(ctx); err != nil {
		return biz.CleanupReceipt{}, false, biz.ErrPersistence
	}
	return receipt, false, nil
}

func (repository *Repository) ExecuteCleanup(ctx context.Context, tenant, execution, hash string, apply func(biz.OperationRecord) (biz.CleanupReceipt, error)) (biz.CleanupReceipt, error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return biz.CleanupReceipt{}, biz.ErrPersistence
	}
	defer rollbackExecutionTransaction(transaction)
	if err := lockOperation(ctx, transaction, tenant, execution); err != nil {
		return biz.CleanupReceipt{}, err
	}
	record, err := operationRecord(ctx, transaction, tenant, execution)
	if err != nil {
		return biz.CleanupReceipt{}, err
	}
	if record.Cleanup == nil || record.Cleanup.PlanHash != hash || record.Cleanup.Phase != "STARTED" || apply == nil {
		return biz.CleanupReceipt{}, biz.ErrCleanupUncertain
	}
	// The first committed STARTED row survives this transaction failing or the
	// process exiting after a DELETE request. No automatic re-send is granted.
	receipt, applyErr := apply(record)
	if receipt.ExecutionID != execution || receipt.PlanHash != hash || receipt.Actor != record.Cleanup.Actor || receipt.StartedAt != record.Cleanup.StartedAt || (receipt.Phase != "APPLIED" && receipt.Phase != "NEEDS_REVIEW") {
		return biz.CleanupReceipt{}, biz.ErrCleanupUncertain
	}
	var completed time.Time
	if err := transaction.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&completed); err != nil {
		return receipt, biz.ErrCleanupUncertain
	}
	receipt.CompletedAt = completed.UTC()
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return receipt, biz.ErrCleanupUncertain
	}
	result, err := transaction.Exec(ctx, "UPDATE modeldev_execution_cleanup_audits SET phase=$4,receipt=$5::jsonb,completed_at=$6 WHERE tenant_id=$1::uuid AND execution_id=$2::uuid AND plan_sha256=$3 AND phase='STARTED'", tenant, execution, hash, receipt.Phase, encoded, completed)
	if err != nil || result.RowsAffected() != 1 {
		return receipt, biz.ErrCleanupUncertain
	}
	if err := transaction.Commit(ctx); err != nil {
		return receipt, biz.ErrCleanupUncertain
	}
	return receipt, applyErr
}

func lockOperation(ctx context.Context, transaction pgx.Tx, tenant, execution string) error {
	tenantID, err := databaseID(tenant)
	if err != nil {
		return err
	}
	executionID, err := databaseID(execution)
	if err != nil {
		return err
	}
	_, err = executionsql.New(transaction).LockExecutionIdentity(ctx, executionsql.LockExecutionIdentityParams{TenantID: tenantID, ExecutionID: executionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.ErrExecutionNotFound
	}
	if err != nil {
		return biz.ErrPersistence
	}
	return nil
}
