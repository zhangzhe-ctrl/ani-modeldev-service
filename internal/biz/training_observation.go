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

// TrainJobObservation reports controller conditions only. Complete does not
// prove successful Pod exit, artifact publication, or absence of active writers.
type TrainJobObservation struct {
	NamespaceUID string
	TrainJobUID  string
	Generation   int64
	Suspended    TrainingConditionStatus
	Complete     TrainingConditionStatus
	Failed       TrainingConditionStatus
	ObservedAt   time.Time
}
