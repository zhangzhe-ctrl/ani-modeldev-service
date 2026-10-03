package biz

import (
	"context"
	"errors"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

type ManagedRuntime struct {
	steps *ManagedSteps
	repository ExecutionRuntimeRepository
	trainer TrainingRuntime
	workspaces PreparedWorkspaceVerifier
	publications PublicationVerifier
}

func NewManagedRuntime(steps *ManagedSteps, repository ExecutionRuntimeRepository, trainer TrainingRuntime, workspaces PreparedWorkspaceVerifier, publications PublicationVerifier) (*ManagedRuntime, error) {
	if steps == nil || repository == nil || trainer == nil || workspaces == nil || publications == nil { return nil, ErrRuntimeNotReady }
	return &ManagedRuntime{steps: steps, repository: repository, trainer: trainer, workspaces: workspaces, publications: publications}, nil
}

type ManagedRuntimeResult struct {
	States ExecutionStates
	Execution Execution
	Authority RunAuthority
	Runtime ExecutionRuntime
	Replayed bool
}

func (runtime *ManagedRuntime) Configuration(ctx context.Context, token string, request BeginManagedExecutionRequest) (ManagedRuntimeResult, error) {
	return ManagedRuntimeResult{}, errors.New("MANAGED_RUNTIME_NOT_IMPLEMENTED")
}
func (runtime *ManagedRuntime) Prepared(ctx context.Context, token string, request BeginManagedExecutionRequest, workspace WorkspaceBinding, input cpup01.InputRef) (ManagedRuntimeResult, error) {
	return ManagedRuntimeResult{}, errors.New("MANAGED_RUNTIME_NOT_IMPLEMENTED")
}
func (runtime *ManagedRuntime) Ensure(ctx context.Context, token string, request BeginManagedExecutionRequest) (ManagedRuntimeResult, error) {
	return ManagedRuntimeResult{}, errors.New("MANAGED_RUNTIME_NOT_IMPLEMENTED")
}
func (runtime *ManagedRuntime) Status(ctx context.Context, token string, request BeginManagedExecutionRequest) (ManagedRuntimeResult, error) {
	return ManagedRuntimeResult{}, errors.New("MANAGED_RUNTIME_NOT_IMPLEMENTED")
}
func (runtime *ManagedRuntime) Publish(ctx context.Context, token string, request BeginManagedExecutionRequest, candidate RuntimePublication) (ManagedRuntimeResult, error) {
	return ManagedRuntimeResult{}, errors.New("MANAGED_RUNTIME_NOT_IMPLEMENTED")
}
func (runtime *ManagedRuntime) Close(ctx context.Context, token string, request BeginManagedExecutionRequest, reason string) (ManagedRuntimeResult, error) {
	return ManagedRuntimeResult{}, errors.New("MANAGED_RUNTIME_NOT_IMPLEMENTED")
}
