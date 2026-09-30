package biz

import "time"

// TrainJobBinding identifies the exact resource previously bound by ModelDev.
// A service caller must obtain it from durable execution facts, not a user body.
type TrainJobBinding struct {
	TenantID     string
	ExecutionID  string
	SpecSHA256   string
	Namespace    string
	NamespaceUID string
	Name         string
	UID          string
}

// TrainingConditionStatus preserves an unknown condition separately from false.
type TrainingConditionStatus string

const (
	TrainingConditionUnknown TrainingConditionStatus = "Unknown"
	TrainingConditionFalse   TrainingConditionStatus = "False"
	TrainingConditionTrue    TrainingConditionStatus = "True"
)

// TrainingConditionMetadata preserves the controller report's optional metadata.
// ObservedGeneration may originate from a JobSet; it is not proof that the
// TrainJob's current generation has been reconciled.
type TrainingConditionMetadata struct {
	Present               bool
	ObservedGeneration    int64
	HasObservedGeneration bool
}

// TrainJobObservation reports controller conditions only. Complete does not
// prove successful Pod exit, artifact publication, absence of active writers,
// or permission to create resources. Generation belongs to the TrainJob itself;
// each condition's reported generation is preserved separately without inference.
type TrainJobObservation struct {
	NamespaceUID string
	TrainJobUID  string
	Generation   int64
	Suspended    TrainingConditionStatus
	Complete     TrainingConditionStatus
	Failed       TrainingConditionStatus
	SuspendedMetadata TrainingConditionMetadata
	CompleteMetadata  TrainingConditionMetadata
	FailedMetadata    TrainingConditionMetadata
	ObservedAt   time.Time
}
