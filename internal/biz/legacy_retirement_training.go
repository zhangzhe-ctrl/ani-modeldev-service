package biz

import "time"

type LegacyRetirementTrainingExitKind string

const (
	LegacyRetirementOriginalFailure      LegacyRetirementTrainingExitKind = "ORIGINAL_FAILURE"
	LegacyRetirementOriginalSuspended143 LegacyRetirementTrainingExitKind = "ORIGINAL_SUSPENDED_EXIT_143"
)

const LegacyRetirementStopFenceSource = "ORIGINAL_PERSISTED_RUNTIME_CLOSE"

// This is the actual pre-overlay runtime row, not a replacement for the
// current command fence or its CAS. The repository compares it under its lock.
type LegacyRetirementStopFence struct {
	Source      string
	Generation  uint64
	Reason      string
	RequestedAt time.Time
}

func ValidLegacyRetirementStopFence(execution Execution, owner RunAuthorityCandidate, state ExecutionRuntime, fence LegacyRetirementStopFence) bool {
	if !LegacyRetirementScopeMatches(execution, owner) || fence.Source != LegacyRetirementStopFenceSource || fence.Generation == 0 || fence.Generation > state.CloseGeneration || fence.RequestedAt.IsZero() || state.CloseRequestedAt.IsZero() || fence.RequestedAt.After(state.CloseRequestedAt) {
		return false
	}
	if fence.Generation == state.CloseGeneration {
		return fence.Reason == state.CloseReason && fence.RequestedAt.Equal(state.CloseRequestedAt) && (fence.Reason == "STEP_FAILED" || fence.Reason == "USER_STOP" || fence.Reason == "DEADLINE")
	}
	scope, _ := LookupLegacyRetirementScope(execution.ExecutionID)
	return scope.TrainingPodUID != "" && fence.Generation == 1 && fence.Reason == "STEP_FAILED" && state.CloseGeneration == 2 && state.CloseReason == "USER_STOP"
}

// This classifies an observation; it never rewrites its original Outcome.
// Only the two fixed original workers can use the suspended-exit branch. The
// repository still requires the full original graph, writer proof and CAS.
func ClassifyLegacyRetirementTrainingExit(execution Execution, owner RunAuthorityCandidate, state ExecutionRuntime, observation TrainingRuntimeObservation, fence LegacyRetirementStopFence) (LegacyRetirementTrainingExitKind, error) {
	scope, ok := LookupLegacyRetirementScope(execution.ExecutionID)
	if !ok || scope.TrainingPodUID == "" || !ValidLegacyRetirementStopFence(execution, owner, state, fence) || state.Training == nil || state.TrainingHandle == nil || observation.Handle != *state.TrainingHandle || !observation.WritersAbsent || observation.ObservedAt.IsZero() {
		return "", ErrRuntimeNotReady
	}
	kind := LegacyRetirementOriginalFailure
	if observation.Outcome == "UNKNOWN" {
		kind = LegacyRetirementOriginalSuspended143
	} else if observation.Outcome != "FAILED" {
		return "", ErrRuntimeNotReady
	}
	counts := map[string]int{}
	for _, resource := range observation.Resources {
		counts[resource.Kind]++
		if resource.Kind == "Pod" {
			if resource.UID != scope.TrainingPodUID || !resource.APIObjectPresent || !resource.Terminal || resource.ExitCode == nil || *resource.ExitCode != 143 || resource.TrainingLog != nil || resource.InitializationAbort != nil || resource.SandboxTermination != nil {
				return "", ErrRuntimeNotReady
			}
			continue
		}
		if kind != LegacyRetirementOriginalSuspended143 || resource.Kind == "PodGroup" {
			continue
		}
		if resource.Kind != "TrainJob" && resource.Kind != "JobSet" && resource.Kind != "Job" {
			return "", ErrRuntimeNotReady
		}
		ack := resource.Suspension
		// Match the original generation-1 create / generation-2 suspend and
		// post-fence acknowledgement, including the controllers that omit
		// observedGeneration. Worker exit retention remains mandatory.
		if !resource.APIObjectPresent || !resource.CreationDisabled || ack == nil || ack.Generation != 2 || ack.ObservedGeneration != 0 && ack.ObservedGeneration != ack.Generation || ack.AcknowledgedAt.IsZero() || ack.AcknowledgedAt.Before(fence.RequestedAt.Truncate(time.Second)) || ack.AcknowledgedAt.After(observation.ObservedAt) || resource.Kind == "Job" && !ack.WorkerExitRetention {
			return "", ErrRuntimeNotReady
		}
	}
	if counts["Pod"] != 1 || counts["TrainJob"] != 1 || counts["JobSet"] != 1 || counts["Job"] != 1 || counts["PodGroup"] > 1 {
		return "", ErrRuntimeNotReady
	}
	return kind, nil
}
