package biz_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func TestCleanupPlanRetainsOutputsAndEvidenceAndBindsExactIdentity(t *testing.T) {
	closed := time.Date(2026, 10, 5, 2, 0, 0, 0, time.UTC)
	fixture := func() biz.OperationRecord {
		return biz.OperationRecord{QueryRecord: biz.QueryRecord{Execution: biz.Execution{Admission: biz.Admission{TenantID: "11111111-1111-4111-8111-111111111111", ExecutionID: "22222222-2222-4222-8222-222222222222", OperationID: "33333333-3333-4333-8333-333333333333", SpecHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, OwnerRevision: 9, States: biz.ExecutionStates{Close: biz.CloseStateClosed, Delivery: biz.DeliveryStatePublished}}, Runtime: biz.ExecutionRuntime{OwnerRevision: 9, ClosedAt: &closed, CloseGeneration: 1, CloseEvidence: &biz.ManagedCloseEvidence{ObservedAt: closed}, Training: &biz.TrainingPlan{}, TrainingHandle: &biz.TrainingHandle{TrainJobUID: "train-1"}, Publication: &biz.RuntimePublication{ID: "publication-1"}, Observation: &biz.TrainingRuntimeObservation{WritersAbsent: true, Resources: []biz.RuntimeResource{
			{APIVersion: "trainer.kubeflow.org/v1alpha1", Kind: "TrainJob", Name: "md-execution", UID: "train-1", Terminal: true, APIObjectPresent: true},
			{APIVersion: "jobset.x-k8s.io/v1alpha2", Kind: "JobSet", Name: "md-execution", UID: "set-1", OwnerUID: "train-1", Terminal: true, APIObjectPresent: true},
			{APIVersion: "batch/v1", Kind: "Job", Name: "md-execution-node", UID: "job-1", OwnerUID: "set-1", Terminal: true, APIObjectPresent: true},
			{APIVersion: "v1", Kind: "Pod", Name: "md-execution-node-1", UID: "pod-1", OwnerUID: "job-1", Terminal: true, APIObjectPresent: true},
		}}}}}
	}
	record := fixture()
	plan, err := biz.CleanupPlanFor(record)
	if err != nil || len(plan.Targets) != 3 || plan.Targets[0].UID != "job-1" || plan.Targets[2].UID != "train-1" || len(plan.Retained) != 5 || len(plan.PlanHash) != 64 {
		t.Fatalf("CLEANUP_PLAN_NOT_IMPLEMENTED: exact closed controller plan unavailable: %+v %v", plan, err)
	}
	replaced := fixture()
	replaced.Runtime.Observation.Resources[0].UID = "replacement-train"
	other, err := biz.CleanupPlanFor(replaced)
	if err != nil || other.PlanHash == plan.PlanHash {
		t.Fatal("replacement UID did not invalidate exact plan")
	}
	for _, mutate := range []func(*biz.OperationRecord){
		func(record *biz.OperationRecord) { record.Runtime.ClosedAt = nil },
		func(record *biz.OperationRecord) { record.Runtime.Observation.WritersAbsent = false },
		func(record *biz.OperationRecord) { record.Runtime.Publication = nil },
		func(record *biz.OperationRecord) { record.Runtime.TrainingHandle = nil },
	} {
		unsafe := fixture()
		mutate(&unsafe)
		if _, err := biz.CleanupPlanFor(unsafe); !errors.Is(err, biz.ErrCleanupBlocked) {
			t.Fatal("unsafe cleanup was planned", err)
		}
	}
}
