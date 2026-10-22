package lifecycle_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
)

func TestReviewedResourceDisposalRetainsOriginalGPUScopeAndMissingLogFailure(t *testing.T) {
	a := gpuReleaseAdmission(t)
	execution := biz.Execution{Admission: a}
	now := time.Now().UTC()
	closed, disposed := now.Add(-time.Minute), now.Add(-time.Second)
	code := int32(143)
	handle := biz.TrainingHandle{NamespaceUID: a.Snapshot.Environment.NamespaceUID, TrainJobUID: "original-train", PVCUID: "original-workspace"}
	auto := &biz.ComputeCleanupReceipt{CleanupReceipt: biz.CleanupReceipt{Phase: "NEEDS_REVIEW", StartedAt: closed}, Attempts: 3, LastError: "ATTEMPTS_EXHAUSTED"}
	before, _ := json.Marshal(auto)
	state := biz.ExecutionRuntime{CloseGeneration: 1, CloseOperationID: uuid.NewString(), CloseReason: "DEADLINE", ClosedAt: &closed, CloseEvidence: &biz.ManagedCloseEvidence{}, Training: &biz.TrainingPlan{}, TrainingHandle: &handle, Observation: &biz.TrainingRuntimeObservation{Handle: handle, WritersAbsent: true, Resources: []biz.RuntimeResource{{APIVersion: "v1", Kind: "Pod", Namespace: a.Snapshot.Environment.NamespaceName, Name: "original-worker", UID: "original-worker-uid", Terminal: true, ExitCode: &code}}}, ComputeCleanup: auto, StepExitCleanup: &biz.StepExitCleanupReceipt{CompletedAt: &closed}, ReviewedComputeCleanup: &biz.CleanupReceipt{Phase: "APPLIED", StartedAt: closed, CompletedAt: disposed}}
	scope := biz.GPUReleaseScope{Ref: biz.GPUReleaseRef{TenantID: a.TenantID, OwnerService: biz.GPUOwnerService, ResourceID: a.ExecutionID, CreateOperationID: a.OperationID}, CloseOperationID: state.CloseOperationID, ClusterID: a.Snapshot.Environment.ClusterID, SealedAt: now, Pods: []biz.GPUReleasePodRef{{ClusterID: a.Snapshot.Environment.ClusterID, Namespace: a.Snapshot.Environment.NamespaceName, Name: "original-worker", UID: "original-worker-uid", ContainerName: "node"}}}
	state.GPURelease = &biz.GPUReleaseState{Scope: scope}
	state.GPURelease.ScopeSHA256, _ = biz.GPUReleaseScopeSHA256(scope)
	if err := lifecycle.ValidGPUReleaseFacts(execution, state, now); err != nil {
		t.Fatal("independently derived disposal could not retain original GPU scope", err)
	}
	if state.GPURelease.Observation != nil || state.GPURelease.Outbox != nil {
		t.Fatal("operator review invented ACC evidence or a release event")
	}
	state.ReviewedComputeCleanup.CompletedAt = now.Add(time.Second)
	if lifecycle.ValidGPUReleaseFacts(execution, state, now) == nil {
		t.Fatal("seal preceded actual committed disposal")
	}
	state.ReviewedComputeCleanup = nil
	if lifecycle.ValidGPUReleaseFacts(execution, state, now) == nil {
		t.Fatal("unreviewed log failure permitted GPU sealing")
	}
	after, _ := json.Marshal(auto)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("GPU resource settlement rewrote the failed log audit")
	}
}
