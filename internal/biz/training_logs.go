package biz

import (
    "context"
    "errors"
    "time"
)

var ErrTrainingLogsUnavailable = errors.New("TRAINING_LOGS_UNAVAILABLE")

type TrainingLogSource struct {
    LogID, Namespace, NamespaceUID, PodName, PodUID string
    OwnerUID, OwnerName, OwnerKind, OwnerAPIVersion string
    ContainerName string
}

type TrainingLogOptions struct { TailLines, MaxBytes uint32 }
type TrainingLogLine struct { Timestamp time.Time; Text string }
type TrainingLogResult struct {
    Lines []TrainingLogLine
    Truncated bool
    ObservedAt time.Time
}

type TrainingLogReader interface {
    ReadTrainingLogs(context.Context, TrainingLogSource, TrainingLogOptions) (TrainingLogResult, error)
}
