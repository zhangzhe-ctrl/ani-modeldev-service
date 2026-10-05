package biz

import (
	"testing"
	"time"
)

func TestStorageCredentialAuthorityEndsWithPublishedDelivery(t *testing.T) {
	result := ManagedRuntimeResult{
		Execution: Execution{Admission: Admission{}},
		States:    ExecutionStates{Compute: ComputeStateSucceeded, Delivery: DeliveryStatePending, Close: CloseStateOpen},
		Runtime:   ExecutionRuntime{Observation: &TrainingRuntimeObservation{Outcome: "SUCCEEDED", WritersAbsent: true}},
	}
	result.Execution.Snapshot.DeadlineAt = time.Now().Add(time.Hour)
	if !storageCredentialsAllowed(result, "publish") {
		t.Fatal("successful training still needs a session to publish its first output")
	}
	result.Runtime.Publication = &RuntimePublication{ID: "persisted-publication"}
	if storageCredentialsAllowed(result, "publish") {
		t.Fatal("a persisted publication must end new writer-session issuance")
	}
	result.Runtime.Publication = nil
	result.States.Delivery = DeliveryStatePublished
	if storageCredentialsAllowed(result, "publish") {
		t.Fatal("published delivery must not refresh a writer session")
	}
}
