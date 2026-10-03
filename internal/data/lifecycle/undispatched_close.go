package lifecycle

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	lifecyclesql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle/sqlc"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/pgvalue"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
)

// CloseUndispatched proves that the sole Run send permit has never existed.
// The same identity lock serializes this proof with admission and reservation;
// an external NotFound or a consumed but uncertain permit is insufficient.
func (repository *Repository) CloseUndispatched(ctx context.Context, tenant, execution string) (biz.ExecutionRuntime, error) {
	tenantID, tenantErr := databaseID(tenant)
	executionID, executionErr := databaseID(execution)
	if tenantErr != nil || executionErr != nil || ctx == nil {
		return biz.ExecutionRuntime{}, biz.ErrInvalidAdmission
	}
	if repository == nil || repository.pool == nil {
		return biz.ExecutionRuntime{}, biz.ErrPersistence
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return biz.ExecutionRuntime{}, biz.ErrPersistence
	}
	defer rollback(tx)
	queries := lifecyclesql.New(tx)
	if _, err := queries.LockRuntimeIdentity(ctx, lifecyclesql.LockRuntimeIdentityParams{TenantID: tenantID, ExecutionID: executionID}); err != nil {
		return biz.ExecutionRuntime{}, storageError(err)
	}
	current, err := readRuntime(ctx, tx, tenantID, executionID)
	if err != nil {
		return biz.ExecutionRuntime{}, err
	}
	_, err = submission.ReadInTransaction(ctx, tx, tenant, execution)
	if err == nil {
		return biz.ExecutionRuntime{}, biz.ErrRuntimeNotReady
	}
	if !errors.Is(err, biz.ErrExecutionNotFound) {
		return biz.ExecutionRuntime{}, err
	}
	state := &current.state
	if state.Workspace != nil || state.Training != nil || state.TrainingHandle != nil || state.Observation != nil || state.Publication != nil {
		return biz.ExecutionRuntime{}, biz.ErrRuntimeConflict
	}
	if state.ClosedAt != nil {
		if state.CloseEvidence == nil || !state.CloseEvidence.NoDispatch {
			return biz.ExecutionRuntime{}, biz.ErrRuntimeConflict
		}
		return *state, nil
	}
	if state.CloseGeneration == 0 {
		if current.now.Before(current.execution.Snapshot.DeadlineAt) {
			return biz.ExecutionRuntime{}, biz.ErrRuntimeNotReady
		}
		generation, err := queries.AdvanceRuntimeClose(ctx, lifecyclesql.AdvanceRuntimeCloseParams{TenantID: tenantID, ExecutionID: executionID})
		if err != nil {
			return biz.ExecutionRuntime{}, biz.ErrPersistence
		}
		value, valid := pgvalue.Uint64(generation)
		if !valid || value == 0 {
			return biz.ExecutionRuntime{}, biz.ErrPersistence
		}
		state.CloseGeneration, state.CloseReason, state.CloseRequestedAt = value, "DEADLINE", current.now
	}
	state.CloseEvidence = &biz.ManagedCloseEvidence{NoDispatch: true, ObservedAt: current.now}
	state.ClosedAt = &current.now
	revision, err := queries.AdvanceRuntimeRevision(ctx, lifecyclesql.AdvanceRuntimeRevisionParams{TenantID: tenantID, ExecutionID: executionID})
	if err != nil {
		return biz.ExecutionRuntime{}, biz.ErrPersistence
	}
	value, valid := pgvalue.Uint64(revision)
	if !valid || value == 0 {
		return biz.ExecutionRuntime{}, biz.ErrPersistence
	}
	state.OwnerRevision = value
	if err := saveRuntime(ctx, current); err != nil {
		return biz.ExecutionRuntime{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return biz.ExecutionRuntime{}, biz.ErrPersistence
	}
	return *state, nil
}
