package cleanup_test

import (
	"context"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/cleanup"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestLegacyRetirementCleanupPlanRequiresOriginalStopAcknowledgementsWithoutWrites(t *testing.T) {
	for _, attack := range []string{"none", "later-command", "missing-trainer-ack", "old-jobset-ack", "replaced-uid", "foreign-finalizer"} {
		t.Run(attack, func(t *testing.T) {
			scope, _ := biz.LookupLegacyRetirementScope("1cce950b-d6e9-453e-aa11-fffd8846224b")
			execution := biz.Execution{Admission: biz.Admission{TenantID: scope.TenantID, ExecutionID: scope.ExecutionID, OperationID: "ec266a51-6708-401c-a735-a4887d26185e", SpecHash: "original-spec", Snapshot: cpup01.Snapshot{Environment: cpup01.EnvironmentBindingSnapshot{ClusterID: scope.ClusterID, NamespaceName: scope.NamespaceName, NamespaceUID: scope.NamespaceUID}}}}
			owner := biz.RunAuthorityCandidate{TenantID: execution.TenantID, ExecutionID: execution.ExecutionID, OperationID: execution.OperationID, SpecHash: execution.SpecHash, NamespaceName: scope.NamespaceName, NamespaceUID: scope.NamespaceUID, RunID: scope.RunID, WorkflowName: scope.WorkflowName, WorkflowUID: scope.WorkflowUID}
			fence := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
			ack := fence.Add(time.Second).Format(time.RFC3339)
			makeObject := func(api, kind, name, uid, parent string) *unstructured.Unstructured {
				meta := map[string]any{"name": name, "namespace": scope.NamespaceName, "uid": uid, "resourceVersion": "9", "generation": int64(2)}
				if parent != "" {
					meta["ownerReferences"] = []any{map[string]any{"uid": parent, "controller": true}}
				}
				return &unstructured.Unstructured{Object: map[string]any{"apiVersion": api, "kind": kind, "metadata": meta, "spec": map[string]any{"suspend": true}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Suspended", "status": "True", "lastTransitionTime": ack}}}}}
			}
			train := makeObject("trainer.kubeflow.org/v1alpha1", "TrainJob", "training", "original-train", "")
			set := makeObject("jobset.x-k8s.io/v1alpha2", "JobSet", "training", "original-set", "original-train")
			job := makeObject("batch/v1", "Job", "training-node", "original-job", "original-set")
			pod := makeObject("v1", "Pod", "original-worker", scope.TrainingPodUID, "original-job")
			pod.SetFinalizers([]string{"modeldev.ani.io/training-exit-evidence"})
			step := makeObject("v1", "Pod", "original-step", "original-step-uid", scope.WorkflowUID)
			step.SetFinalizers([]string{"modeldev.ani.io/step-exit-evidence"})
			objects := []runtime.Object{train, set, job, pod, step}
			resources := []biz.RuntimeResource{}
			for _, o := range []*unstructured.Unstructured{train, set, job, pod} {
				parent := ""
				for _, ref := range o.GetOwnerReferences() {
					parent = string(ref.UID)
				}
				resources = append(resources, biz.RuntimeResource{APIVersion: o.GetAPIVersion(), Kind: o.GetKind(), Namespace: scope.NamespaceName, Name: o.GetName(), UID: string(o.GetUID()), OwnerUID: parent, Terminal: true, CreationDisabled: true, APIObjectPresent: true})
			}
			state := biz.ExecutionRuntime{CloseGeneration: 1, CloseReason: "DEADLINE", CloseRequestedAt: fence, Training: &biz.TrainingPlan{Name: "training"}, TrainingHandle: &biz.TrainingHandle{NamespaceUID: scope.NamespaceUID, TrainJobUID: "original-train"}}
			observation := biz.TrainingRuntimeObservation{Handle: *state.TrainingHandle, WritersAbsent: true, Outcome: "FAILED", Resources: resources}
			evidence := biz.ManagedCloseEvidence{Resources: []biz.RuntimeResource{{APIVersion: "v1", Kind: "Pod", Namespace: scope.NamespaceName, Name: step.GetName(), UID: string(step.GetUID()), OwnerUID: scope.WorkflowUID}}}
			stopFence := biz.LegacyRetirementStopFence{Source: biz.LegacyRetirementStopFenceSource, Generation: state.CloseGeneration, Reason: state.CloseReason, RequestedAt: state.CloseRequestedAt}
			switch attack {
			case "later-command":
				state.CloseGeneration, state.CloseReason, state.CloseRequestedAt = 2, "USER_STOP", fence.Add(30*time.Second)
				stopFence.Reason = "STEP_FAILED"
			case "missing-trainer-ack":
				train.Object["status"] = map[string]any{}
			case "old-jobset-ack":
				unstructured.SetNestedSlice(set.Object, []any{map[string]any{"type": "Suspended", "status": "True", "lastTransitionTime": fence.Add(-time.Second).Format(time.RFC3339)}}, "status", "conditions")
			case "replaced-uid":
				job.SetUID("replacement")
			case "foreign-finalizer":
				step.SetFinalizers([]string{"foreign.example/retain"})
			}
			kube := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), objects...)
			err := cleanup.New(kube, nil).ValidateLegacyRetirementCleanupPlan(context.Background(), execution, owner, state, evidence, observation, stopFence)
			positive := attack == "none" || attack == "later-command"
			if positive && err != nil {
				t.Fatal("actual original stop proof rejected", err)
			}
			if !positive && err == nil {
				t.Fatal("current original cleanup guard bypassed", attack)
			}
			for _, action := range kube.Actions() {
				if action.GetVerb() != "get" {
					t.Fatal("read-only plan mutated", action.GetVerb())
				}
			}
			if state.ClosedAt != nil || state.ComputeCleanup != nil || state.StepExitCleanup != nil {
				t.Fatal("plan fabricated committed close or cleanup facts")
			}
		})
	}
}
