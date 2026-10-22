package biz

import (
	"context"

	"github.com/google/uuid"
)

func (runtime *ManagedRuntime) captureTrainingExitLogs(ctx context.Context, execution Execution, state ExecutionRuntime, observation TrainingRuntimeObservation) (TrainingRuntimeObservation, error) {
	if state.Observation != nil {
		observation.RecordedLogs = append([]RecordedTrainingLog(nil), state.Observation.RecordedLogs...)
	}
	candidate := state
	candidate.Observation = &observation
	record := QueryRecord{Execution: execution, Runtime: candidate}
	if err := ValidateTerminalTrainingLogs(record, observation.RecordedLogs); err != nil {
		return observation, err
	}
	for _, pod := range observation.Resources {
		if pod.Kind != "Pod" || !pod.Terminal || pod.ExitCode == nil {
			continue
		}
		found := false
		for _, log := range observation.RecordedLogs {
			found = found || log.Source.PodUID == pod.UID
		}
		if found {
			continue
		}
		if runtime.logs == nil {
			return observation, ErrTrainingLogsUnavailable
		}
		id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(execution.ExecutionID+":"+pod.UID+":node")).String()
		source, err := TrainingLogSourceFor(record, id)
		if err != nil {
			return observation, err
		}
		tail, err := runtime.logs.ReadTrainingLogs(ctx, source, TrainingLogOptions{TailLines: 1000, MaxBytes: 65536})
		if err != nil {
			return observation, err
		}
		log := RecordedTrainingLog{Source: source, Result: tail}
		if err := ValidateTerminalTrainingLogs(record, []RecordedTrainingLog{log}); err != nil {
			return observation, err
		}
		observation.RecordedLogs = append(observation.RecordedLogs, log)
	}
	return observation, nil
}
