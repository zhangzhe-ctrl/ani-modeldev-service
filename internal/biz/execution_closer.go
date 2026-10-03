package biz

import (
	"context"
	"errors"
	"strings"
)

// These ports are called by the configured execution owner, after a durable
// creation fence. They do not authorize a caller-supplied Run or workspace.
type ManagedRunCloser interface {
	StopManagedRun(context.Context, PipelineDispatchPlan, RunAuthorityCandidate) error
}

type OwnerWriterVerifier interface {
	VerifyOwnerWritersAbsent(context.Context, Execution, RunAuthorityCandidate, *WorkspaceBinding) (ManagedCloseEvidence, error)
}

type UndispatchedCloser interface {
	CloseUndispatched(context.Context, string, string) (ExecutionRuntime, error)
}

type ExecutionCloser struct {
	runtime *ManagedRuntime
	runs    ManagedRunCloser
	writers OwnerWriterVerifier
}

func NewExecutionCloser(runtime *ManagedRuntime, runs ManagedRunCloser, writers OwnerWriterVerifier) (*ExecutionCloser, error) {
	if runtime == nil || runs == nil || writers == nil {
		return nil, ErrRuntimeNotReady
	}
	return &ExecutionCloser{runtime: runtime, runs: runs, writers: writers}, nil
}

// Reconcile resumes closing the original durable execution. It must never
// advance normal pipeline steps or reconstruct a creation permit.
func (closer *ExecutionCloser) Reconcile(ctx context.Context, tenant, execution string) (ManagedRuntimeResult, error) {
	if ctx == nil || !validAdmissionID(tenant) || !validAdmissionID(execution) || strings.ToLower(tenant) != tenant || strings.ToLower(execution) != execution {
		return ManagedRuntimeResult{}, ErrInvalidAdmission
	}
	if closer == nil || closer.runtime == nil || closer.runs == nil || closer.writers == nil {
		return ManagedRuntimeResult{}, ErrRuntimeNotReady
	}
	if err := ctx.Err(); err != nil {
		return ManagedRuntimeResult{}, err
	}
	runtime := closer.runtime
	admitted, err := runtime.steps.executions.Get(ctx, tenant, execution)
	if err != nil {
		return ManagedRuntimeResult{}, err
	}
	authority, err := runtime.steps.repository.GetRunAuthority(ctx, tenant, execution)
	if errors.Is(err, ErrExecutionNotFound) {
		unbound, ok := runtime.repository.(UndispatchedCloser)
		if !ok {
			return ManagedRuntimeResult{}, ErrRuntimeNotReady
		}
		state, err := unbound.CloseUndispatched(ctx, tenant, execution)
		if err != nil {
			return ManagedRuntimeResult{}, err
		}
		return runtimeResult(ManagedRuntimeResult{Execution: admitted}, state, false)
	}
	if err != nil {
		return ManagedRuntimeResult{}, err
	}
	dispatch, err := runtime.steps.repository.Get(ctx, tenant, execution)
	if err != nil {
		return ManagedRuntimeResult{}, err
	}
	plan, owner := dispatch.Plan, authority.RunAuthorityCandidate
	if owner.TenantID != admitted.TenantID || owner.ExecutionID != admitted.ExecutionID || owner.OperationID != admitted.OperationID || owner.SpecHash != admitted.SpecHash || owner.AttemptID != dispatch.AttemptID || owner.PlanHash != dispatch.PlanHash || owner.NamespaceName != admitted.Snapshot.Environment.NamespaceName || owner.NamespaceUID != admitted.Snapshot.Environment.NamespaceUID || plan.TenantID != admitted.TenantID || plan.ExecutionID != admitted.ExecutionID || plan.OperationID != admitted.OperationID || plan.SpecHash != admitted.SpecHash || plan.Environment != admitted.Snapshot.Environment {
		return ManagedRuntimeResult{}, ErrRunAuthorityConflict
	}
	state, err := runtime.repository.GetRuntime(ctx, tenant, execution)
	if err != nil {
		return ManagedRuntimeResult{}, err
	}
	result := ManagedRuntimeResult{Execution: admitted, Authority: authority, Runtime: state}
	if state.ClosedAt != nil {
		return runtimeResult(result, state, true)
	}
	if state.CloseGeneration == 0 {
		// The database clock checks the original frozen deadline while holding
		// the same identity lock as creation. Calling reconcile is not a stop.
		state, _, err = runtime.repository.RequestRuntimeClose(ctx, owner, "DEADLINE")
		if err != nil {
			return ManagedRuntimeResult{}, err
		}
	}
	result.Runtime = state
	// A stopped KFP waiter cannot stop external training by itself. Even when
	// KFP is temporarily unavailable, make the independent training stop attempt
	// after the committed fence; neither failure may imply CLOSED.
	runErr := closer.runs.StopManagedRun(ctx, plan, owner)
	if state.Training != nil {
		if state.TrainingHandle == nil {
			handle, findErr := runtime.trainer.FindTraining(ctx, *state.Training)
			if findErr != nil {
				return ManagedRuntimeResult{}, errors.Join(runErr, findErr)
			}
			state, err = runtime.repository.RecordTrainingHandle(ctx, owner, handle)
			if err != nil {
				return ManagedRuntimeResult{}, errors.Join(runErr, err)
			}
		}
		if state.Observation == nil || !state.Observation.WritersAbsent {
			if err = runtime.trainer.StopTraining(ctx, *state.Training, *state.TrainingHandle); err != nil {
				return ManagedRuntimeResult{}, errors.Join(runErr, err)
			}
		}
		result.Runtime = state
		state, err = runtime.observe(ctx, result)
		if err != nil {
			return ManagedRuntimeResult{}, errors.Join(runErr, err)
		}
		if state.Observation == nil || !state.Observation.WritersAbsent {
			return ManagedRuntimeResult{}, errors.Join(runErr, ErrRuntimeNotReady)
		}
	}
	if runErr != nil {
		return ManagedRuntimeResult{}, runErr
	}
	// Extra observed Runs cannot be erased by closing only the authority. They
	// remain a reconciliation concern, with the shared creation fence retained.
	for _, run := range dispatch.ConfirmedRuns {
		if run.RunID != owner.RunID {
			return ManagedRuntimeResult{}, ErrRunAuthorityConflict
		}
	}
	evidence, err := closer.writers.VerifyOwnerWritersAbsent(ctx, admitted, owner, state.Workspace)
	if err != nil {
		return ManagedRuntimeResult{}, err
	}
	observation := TrainingRuntimeObservation{}
	if state.Observation != nil {
		observation = *state.Observation
	}
	state, err = runtime.repository.ConfirmRuntimeClosed(ctx, owner, state.CloseGeneration, observation, evidence)
	if err != nil {
		return ManagedRuntimeResult{}, err
	}
	return runtimeResult(result, state, false)
}
