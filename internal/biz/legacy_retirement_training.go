package biz

import "time"

type LegacyRetirementTrainingExitKind string

const (
	LegacyRetirementOriginalFailure      LegacyRetirementTrainingExitKind = "ORIGINAL_FAILURE"
	LegacyRetirementOriginalSuspended143 LegacyRetirementTrainingExitKind = "ORIGINAL_SUSPENDED_EXIT_143"
)

// This classifies an observation; it never rewrites its original Outcome.
// Only the two fixed original workers can use the suspended-exit branch. The
// repository still requires the full original graph, writer proof and CAS.
func ClassifyLegacyRetirementTrainingExit(execution Execution, owner RunAuthorityCandidate, state ExecutionRuntime, observation TrainingRuntimeObservation) (LegacyRetirementTrainingExitKind, error) {
	scope, ok := LookupLegacyRetirementScope(execution.ExecutionID)
	if !ok || scope.TrainingPodUID == "" || !LegacyRetirementScopeMatches(execution, owner) || state.Training == nil || state.TrainingHandle == nil || observation.Handle != *state.TrainingHandle || !observation.WritersAbsent || observation.ObservedAt.IsZero() {
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
		if !resource.APIObjectPresent || !resource.CreationDisabled || ack == nil || ack.Generation != 2 || ack.ObservedGeneration != 0 && ack.ObservedGeneration != ack.Generation || state.CloseRequestedAt.IsZero() || ack.AcknowledgedAt.IsZero() || ack.AcknowledgedAt.Before(state.CloseRequestedAt.Truncate(time.Second)) || ack.AcknowledgedAt.After(observation.ObservedAt) || resource.Kind == "Job" && !ack.WorkerExitRetention {
			return "", ErrRuntimeNotReady
		}
	}
	if counts["Pod"] != 1 || counts["TrainJob"] != 1 || counts["JobSet"] != 1 || counts["Job"] != 1 || counts["PodGroup"] > 1 {
		return "", ErrRuntimeNotReady
	}
	return kind, nil
}
