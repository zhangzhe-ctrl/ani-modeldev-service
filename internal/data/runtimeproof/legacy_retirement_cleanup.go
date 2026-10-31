package runtimeproof

import (
	"context"
	"reflect"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// LegacyRetirementBoundary is explicitly composed by the one-time command.
// The ordinary Verifier interfaces never recognize archive or unavailable logs.
// Every call remains bound to the committed journal, raw official archive and
// original owner, while sharing the exact current writer and delete guards.
type LegacyRetirementBoundary struct {
	verifier  *Verifier
	execution biz.Execution
	owner     biz.RunAuthorityCandidate
	state     biz.ExecutionRuntime
}

func NewLegacyRetirementBoundary(verifier *Verifier, execution biz.Execution, owner biz.RunAuthorityCandidate, state biz.ExecutionRuntime, archive []byte) (*LegacyRetirementBoundary, error) {
	scope, ok := biz.LookupLegacyRetirementScope(execution.ExecutionID)
	if !ok || verifier == nil || !biz.LegacyRetirementQualification(execution, owner, state) || state.ClosedAt == nil || contentSHA(archive) != scope.ArchiveSHA256 {
		return nil, biz.ErrCleanupBlocked
	}
	if _, err := parseLegacyRetirementWorkflow(archive, scope); err != nil {
		return nil, err
	}
	return &LegacyRetirementBoundary{verifier: verifier, execution: execution, owner: owner, state: state}, nil
}

func (b *LegacyRetirementBoundary) matches(execution biz.Execution, owner biz.RunAuthorityCandidate) bool {
	return b != nil && b.verifier != nil && reflect.DeepEqual(execution, b.execution) && owner == b.owner && biz.LegacyRetirementQualification(execution, owner, b.state)
}

func (b *LegacyRetirementBoundary) VerifyOwnerWritersAbsent(ctx context.Context, execution biz.Execution, owner biz.RunAuthorityCandidate, workspace *biz.WorkspaceBinding) (biz.ManagedCloseEvidence, error) {
	if !b.matches(execution, owner) || b.state.CloseEvidence == nil {
		return biz.ManagedCloseEvidence{}, biz.ErrCleanupBlocked
	}
	evidence := *b.state.CloseEvidence
	if err := b.VerifyRecordedOwnerWritersAbsent(ctx, execution, owner, workspace, evidence); err != nil {
		return biz.ManagedCloseEvidence{}, err
	}
	return evidence, nil
}

func (b *LegacyRetirementBoundary) VerifyRecordedOwnerWritersAbsent(ctx context.Context, execution biz.Execution, owner biz.RunAuthorityCandidate, workspace *biz.WorkspaceBinding, evidence biz.ManagedCloseEvidence) error {
	if !b.matches(execution, owner) || !reflect.DeepEqual(b.state.Workspace, workspace) || !reflect.DeepEqual(b.state.CloseEvidence, &evidence) {
		return biz.ErrCleanupBlocked
	}
	return b.verifier.verifyRecordedOwnerWritersAbsentWithArchive(ctx, execution, owner, workspace, evidence, false, true)
}

func (b *LegacyRetirementBoundary) ReconcileStepExitCleanup(ctx context.Context, execution biz.Execution, owner biz.RunAuthorityCandidate, workspace *biz.WorkspaceBinding, receipt biz.StepExitCleanupReceipt) ([]biz.RuntimeResource, error) {
	if !b.matches(execution, owner) || b.state.StepExitCleanup == nil || !reflect.DeepEqual(*b.state.StepExitCleanup, receipt) || receipt.CloseGeneration != b.state.CloseGeneration {
		return nil, biz.ErrCleanupBlocked
	}
	return b.verifier.reconcileStepExitCleanup(ctx, execution, owner, workspace, receipt, b.VerifyRecordedOwnerWritersAbsent)
}

func cleanupLogQualification(execution biz.Execution, owner biz.RunAuthorityCandidate, state biz.ExecutionRuntime, legacy bool) bool {
	if legacy {
		return biz.LegacyRetirementCleanupAuthorized(execution, owner, state)
	}
	return state.ComputeCleanup != nil && state.ComputeCleanup.LogsReady && biz.ComputeCleanupLogsComplete(execution, state, state.ComputeCleanup.Logs)
}

func (b *LegacyRetirementBoundary) ReconcileComputeCleanup(ctx context.Context, execution biz.Execution, owner biz.RunAuthorityCandidate, state biz.ExecutionRuntime) ([]biz.RuntimeResource, error) {
	if !b.matches(execution, owner) || !reflect.DeepEqual(state, b.state) || !biz.LegacyRetirementCleanupAuthorized(execution, owner, state) {
		return nil, biz.ErrCleanupBlocked
	}
	return b.verifier.reconcileComputeCleanup(ctx, execution, owner, state, true)
}

func (b *LegacyRetirementBoundary) ObserveComputeCleanupAbsence(ctx context.Context, execution biz.Execution, owner biz.RunAuthorityCandidate, state biz.ExecutionRuntime) ([]biz.RuntimeResource, error) {
	if !b.matches(execution, owner) || !reflect.DeepEqual(state, b.state) || !biz.LegacyRetirementCleanupAuthorized(execution, owner, state) {
		return nil, biz.ErrCleanupBlocked
	}
	return b.verifier.observeComputeCleanupAbsence(ctx, execution, owner, state, true)
}

func (b *LegacyRetirementBoundary) VerifyRecordedComputeWritersAbsent(ctx context.Context, execution biz.Execution, owner biz.RunAuthorityCandidate, state biz.ExecutionRuntime) error {
	if !b.matches(execution, owner) || !reflect.DeepEqual(state, b.state) || !biz.LegacyRetirementComputeCleanupApplied(execution, owner, state) {
		return biz.ErrCleanupBlocked
	}
	return b.verifier.verifyComputeCleanupGraphWithLegacy(ctx, execution, owner, state, true, true)
}
