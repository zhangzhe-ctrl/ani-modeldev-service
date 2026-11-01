package lifecycle

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
)

// This type is composed only by the one-time maintenance command. New(pool)
// never enables archival closure or unrecoverable-log cleanup.
type LegacyRetirementRepository struct{ *Repository }

func NewLegacyRetirement(pool *pgxpool.Pool) *LegacyRetirementRepository {
	return &LegacyRetirementRepository{Repository: &Repository{pool: pool, legacyRetirement: true}}
}

// GetLegacyRetirementRecord returns a single repeatable-read, read-only view.
// It is also the restart path after ordinary readers reject the maintenance
// marker. No exception fact or permission is created by this read.
func (repository *LegacyRetirementRepository) GetLegacyRetirementRecord(ctx context.Context, tenant, execution string) (biz.Execution, biz.ExecutionRuntime, biz.PipelineDispatch, biz.RunAuthorityCandidate, uint64, error) {
	zero := func(err error) (biz.Execution, biz.ExecutionRuntime, biz.PipelineDispatch, biz.RunAuthorityCandidate, uint64, error) {
		return biz.Execution{}, biz.ExecutionRuntime{}, biz.PipelineDispatch{}, biz.RunAuthorityCandidate{}, 0, err
	}
	scope, allowed := biz.LookupLegacyRetirementScope(execution)
	if !allowed || tenant != scope.TenantID || repository == nil || repository.Repository == nil || repository.pool == nil || !repository.legacyRetirement || ctx == nil {
		return zero(biz.ErrInvalidAdmission)
	}
	tenantID, _ := databaseID(tenant)
	executionID, _ := databaseID(execution)
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return zero(biz.ErrPersistence)
	}
	defer rollback(tx)
	current, err := repository.readRuntime(ctx, tx, tenantID, executionID)
	if err != nil {
		return zero(err)
	}
	dispatch, err := submission.ReadInTransaction(ctx, tx, tenant, execution)
	if err != nil {
		return zero(err)
	}
	authority, err := submission.ReadAuthorityInTransaction(ctx, tx, tenant, execution)
	candidate := authority.RunAuthorityCandidate
	if errors.Is(err, biz.ErrExecutionNotFound) && current.state.CloseAuthority != nil {
		candidate, err = *current.state.CloseAuthority, nil
	}
	if err != nil || !biz.LegacyRetirementScopeMatches(current.execution, candidate) || len(dispatch.ConfirmedRuns) != 1 || dispatch.ConfirmedRuns[0].RunID != candidate.RunID || dispatch.AttemptID != candidate.AttemptID || dispatch.PlanHash != candidate.PlanHash {
		return zero(biz.ErrRunAuthorityConflict)
	}
	if err = tx.Commit(ctx); err != nil {
		return zero(biz.ErrPersistence)
	}
	return current.execution, current.state, dispatch, candidate, current.commandFence, nil
}

// ConfirmLegacyRetirement commits the explicit exception journal and verified
// terminal facts in one original aggregate transaction. It creates no new
// generation, authority, Run, resource, release event or cleanup attempt.
func (repository *LegacyRetirementRepository) ConfirmLegacyRetirement(ctx context.Context, request biz.LegacyRetirementRequest) (biz.ExecutionRuntime, error) {
	if repository == nil || repository.Repository == nil || !repository.legacyRetirement || request.ApprovalSHA256 != biz.LegacyRetirementApprovalSHA256 || !cpup01.ValidAuditActor(request.Actor) {
		return biz.ExecutionRuntime{}, biz.ErrInvalidAdmission
	}
	state, _, err := repository.mutate(ctx, request.Authority, qualifyLegacyRetirement(ctx, request))
	return state, err
}

// ValidateLegacyRetirement reuses every close qualification inside a real
// repeatable-read READ ONLY transaction. It changes only its in-memory copy;
// the apply path must repeat these checks under the original identity lock.
func (repository *LegacyRetirementRepository) ValidateLegacyRetirement(ctx context.Context, request biz.LegacyRetirementRequest) error {
	if repository == nil || repository.Repository == nil || repository.pool == nil || !repository.legacyRetirement || ctx == nil || request.ApprovalSHA256 != biz.LegacyRetirementApprovalSHA256 || !cpup01.ValidAuditActor(request.Actor) {
		return biz.ErrInvalidAdmission
	}
	tenantID, err := databaseID(request.Authority.TenantID)
	if err != nil {
		return err
	}
	executionID, err := databaseID(request.Authority.ExecutionID)
	if err != nil {
		return err
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return biz.ErrPersistence
	}
	defer rollback(tx)
	current, err := repository.readRuntime(ctx, tx, tenantID, executionID)
	if err != nil {
		return err
	}
	authority, err := submission.ReadAuthorityInTransaction(ctx, tx, request.Authority.TenantID, request.Authority.ExecutionID)
	observed := authority.RunAuthorityCandidate
	if errors.Is(err, biz.ErrExecutionNotFound) && current.state.CloseAuthority != nil {
		observed, err = *current.state.CloseAuthority, nil
	}
	if err != nil || observed != request.Authority {
		return biz.ErrRunAuthorityConflict
	}
	if _, err = qualifyLegacyRetirement(ctx, request)(current); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return biz.ErrPersistence
	}
	return nil
}

func qualifyLegacyRetirement(ctx context.Context, request biz.LegacyRetirementRequest) func(*runtimeTransaction) (bool, error) {
	return func(current *runtimeTransaction) (bool, error) {
		if !biz.LegacyRetirementScopeMatches(current.execution, request.Authority) || request.CloseGeneration == 0 || request.CommandFence != request.CloseGeneration || request.CloseGeneration != current.state.CloseGeneration || request.CommandFence != current.commandFence || current.state.CloseReason == "NATURAL_TERMINAL" || current.state.CloseRequestedAt.IsZero() || current.state.CloseReviewReason == "MULTIPLE_RUNS" {
			return false, biz.ErrRuntimeConflict
		}
		record := &biz.LegacyRetirementRecord{Authority: request.Authority, ExpectedOwnerRevision: request.ExpectedOwnerRevision, CloseGeneration: request.CloseGeneration, CommandFence: request.CommandFence, ApprovalSHA256: request.ApprovalSHA256, Actor: request.Actor, Archive: request.Archive, UnrecoverableLogs: append([]biz.LegacyTrainingLogDisposition{}, request.UnrecoverableLogs...), TrainingExitKind: request.TrainingExitKind, RecordedAt: current.now}
		if prior := current.state.LegacyRetirement; prior != nil {
			record.RecordedAt = prior.RecordedAt
			if !reflect.DeepEqual(prior, record) || !reflect.DeepEqual(current.state.CloseEvidence, &request.Evidence) {
				return false, biz.ErrRuntimeConflict
			}
			if current.state.Observation != nil {
				merged, err := mergeObservation(current.state, request.Observation)
				if err != nil || !reflect.DeepEqual(current.state.Observation, &merged) {
					return false, biz.ErrRuntimeConflict
				}
			} else if !reflect.DeepEqual(request.Observation, biz.TrainingRuntimeObservation{}) {
				return false, biz.ErrRuntimeConflict
			}
			return false, nil
		}
		if request.ExpectedOwnerRevision == 0 || request.ExpectedOwnerRevision != current.state.OwnerRevision || current.state.ClosedAt != nil || current.state.StepExitCleanup != nil || current.state.ComputeCleanup != nil || current.state.GPURelease != nil || !runtimeCreationSettled(current.state) {
			return false, biz.ErrRuntimeConflict
		}
		// Re-derive the frozen dispatch and exactly one original confirmed Run
		// under the same lock; CloseAuthority replay alone proves no closure.
		dispatch, err := submission.ReadInTransaction(ctx, current.transaction, request.Authority.TenantID, request.Authority.ExecutionID)
		if err != nil || dispatch.State != biz.PipelineDispatchConfirmed || len(dispatch.ConfirmedRuns) != 1 || dispatch.ConfirmedRuns[0].RunID != request.Authority.RunID || dispatch.AttemptID != request.Authority.AttemptID || dispatch.PlanHash != request.Authority.PlanHash || dispatch.Plan.OperationID != request.Authority.OperationID || dispatch.Plan.SpecHash != request.Authority.SpecHash || dispatch.Plan.Environment != current.execution.Snapshot.Environment {
			return false, biz.ErrRunAuthorityConflict
		}
		if !validSandboxTerminationScope(current.state, request.Evidence) || !validExecutionCloseEvidence(current.execution, request.Authority, current.state.CloseReason, request.Evidence) || !finetuningStartsTerminated(current.state, request.Authority, request.Evidence) || request.Evidence.ObservedAt.After(current.now) {
			return false, biz.ErrRuntimeNotReady
		}
		if current.state.Training != nil && current.state.TrainingRejection == nil {
			merged, err := mergeObservation(current.state, request.Observation)
			if err != nil || !merged.WritersAbsent || merged.ObservedAt.After(current.now) {
				return false, biz.ErrRuntimeNotReady
			}
			current.state.Observation = &merged
		} else if !reflect.DeepEqual(request.Observation, biz.TrainingRuntimeObservation{}) {
			return false, biz.ErrRuntimeConflict
		}
		current.state.CloseEvidence = &request.Evidence
		current.state.LegacyRetirement = record
		if !biz.LegacyRetirementQualification(current.execution, request.Authority, current.state) {
			return false, biz.ErrRuntimeNotReady
		}
		current.state.CloseReviewReason = ""
		closed := current.now
		current.state.ClosedAt = &closed
		return true, nil
	}
}
