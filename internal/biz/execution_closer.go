package biz

import "context"

// These ports are called by the configured execution owner, after a durable
// creation fence. They do not authorize a caller-supplied Run or workspace.
type ManagedRunCloser interface {
	StopManagedRun(context.Context, PipelineDispatchPlan, RunAuthorityCandidate) error
}

type OwnerWriterVerifier interface {
	VerifyOwnerWritersAbsent(context.Context, Execution, RunAuthorityCandidate, *WorkspaceBinding) (ManagedCloseEvidence, error)
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
	return ManagedRuntimeResult{}, ErrRuntimeNotReady
}
