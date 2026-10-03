package biz

import (
	"context"
	"errors"
)

var (
	ErrManagedStepUnauthorized = errors.New("MANAGED_STEP_UNAUTHORIZED")
	ErrManagedStepUnavailable = errors.New("MANAGED_STEP_UNAVAILABLE")
)

// ManagedStepAssociation is a claim, never proof. Both the workload identity
// and the KFP control API must independently confirm it on every callback.
type ManagedStepAssociation struct {
	RunID, NamespaceName, NamespaceUID string
	WorkflowName, WorkflowUID string
	PodName, PodUID string
}

type BeginManagedExecutionRequest struct {
	TenantID, OperationID, ExecutionID, SpecHash string
	Association ManagedStepAssociation
}

type BeginManagedExecutionResult struct {
	Authority RunAuthorityReceipt
	States ExecutionStates
}

type RunAuthorityRepository interface {
	Get(context.Context, string, string) (PipelineDispatch, error)
	BindRunAuthority(context.Context, RunAuthorityCandidate) (RunAuthorityReceipt, error)
}

type ManagedWorkloadVerifier interface {
	Verify(context.Context, string, PipelineDispatchPlan, ManagedStepAssociation) error
}

type ManagedRunVerifier interface {
	VerifyManagedRun(context.Context, PipelineDispatchPlan, ManagedStepAssociation, string) error
}

// ManagedSteps handles KFP callbacks; it never advances the normal workflow.
// The token is used transiently for TokenReview and is never persisted.
type ManagedSteps struct {
	repository RunAuthorityRepository
	executions ExecutionRepository
	workloads ManagedWorkloadVerifier
	runs ManagedRunVerifier
}

func NewManagedSteps(repository RunAuthorityRepository, executions ExecutionRepository, workloads ManagedWorkloadVerifier, runs ManagedRunVerifier) (*ManagedSteps, error) {
	if repository == nil || executions == nil || workloads == nil || runs == nil {
		return nil, ErrManagedStepUnavailable
	}
	return &ManagedSteps{repository: repository, executions: executions, workloads: workloads, runs: runs}, nil
}

func (steps *ManagedSteps) Begin(ctx context.Context, token string, request BeginManagedExecutionRequest) (BeginManagedExecutionResult, error) {
	return BeginManagedExecutionResult{}, errors.New("MANAGED_STEP_NOT_IMPLEMENTED")
}
