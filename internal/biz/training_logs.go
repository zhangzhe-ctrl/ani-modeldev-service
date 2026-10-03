package biz

import (
    "context"
    "errors"
    "time"

    "github.com/google/uuid"
)

var ErrTrainingLogsUnavailable = errors.New("TRAINING_LOGS_UNAVAILABLE")
var ErrTrainingLogNotFound = errors.New("TRAINING_LOG_NOT_FOUND")

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

// TrainingLogSourceFor resolves only durable observations of the fixed CPU
// trainer container. A log ID can select a recorded segment, never a Pod name.
func TrainingLogSourceFor(record QueryRecord, logID string) (TrainingLogSource, error) {
    state := record.Runtime
    if state.Observation == nil || state.Training == nil || state.TrainingHandle == nil ||
        state.Observation.Handle != *state.TrainingHandle || state.Observation.ObservedAt.IsZero() {
        return TrainingLogSource{}, ErrTrainingLogsUnavailable
    }
    resources := make(map[string]RuntimeResource, len(state.Observation.Resources))
    for _, resource := range state.Observation.Resources {
        if resource.UID == "" { return TrainingLogSource{}, ErrTrainingLogsUnavailable }
        if _, duplicate := resources[resource.UID]; duplicate { return TrainingLogSource{}, ErrTrainingLogsUnavailable }
        resources[resource.UID] = resource
    }
    var selected *TrainingLogSource
    pods := 0
    namespace := record.Execution.Snapshot.Environment.NamespaceName
    for _, pod := range state.Observation.Resources {
        if pod.Kind != "Pod" { continue }
        pods++
        id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(record.Execution.ExecutionID+":"+pod.UID+":node")).String()
        if logID != "" && logID != id { continue }
        if selected != nil || pod.APIVersion != "v1" || pod.Namespace != namespace || !pod.APIObjectPresent {
            return TrainingLogSource{}, ErrTrainingLogsUnavailable
        }
        owner, found := resources[pod.OwnerUID]
        if !found || owner.Kind != "Job" || owner.APIVersion != "batch/v1" || owner.Namespace != namespace {
            return TrainingLogSource{}, ErrTrainingLogsUnavailable
        }
        set, found := resources[owner.OwnerUID]
        if !found || set.Kind != "JobSet" || set.APIVersion != "jobset.x-k8s.io/v1alpha2" || set.Namespace != namespace {
            return TrainingLogSource{}, ErrTrainingLogsUnavailable
        }
        train, found := resources[set.OwnerUID]
        if !found || train.UID != state.TrainingHandle.TrainJobUID || train.Kind != "TrainJob" ||
            train.APIVersion != "trainer.kubeflow.org/v1alpha1" || train.Namespace != namespace || train.Name != state.Training.Name ||
            state.TrainingHandle.NamespaceUID != record.Execution.Snapshot.Environment.NamespaceUID {
            return TrainingLogSource{}, ErrTrainingLogsUnavailable
        }
        selected = &TrainingLogSource{LogID:id, Namespace:namespace, NamespaceUID:state.TrainingHandle.NamespaceUID,
            PodName:pod.Name, PodUID:pod.UID, OwnerUID:owner.UID, OwnerName:owner.Name,
            OwnerKind:owner.Kind, OwnerAPIVersion:owner.APIVersion, ContainerName:"node"}
    }
    if logID == "" && pods != 1 { return TrainingLogSource{}, ErrTrainingLogsUnavailable }
    if selected == nil { return TrainingLogSource{}, ErrTrainingLogNotFound }
    return *selected, nil
}
