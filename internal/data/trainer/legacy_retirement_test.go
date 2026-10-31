package trainer_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/trainer"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/admissionfixture"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

type forbiddenLegacyLogReader struct{ calls int }

func (r *forbiddenLegacyLogReader) ReadTrainingLogs(context.Context, biz.TrainingLogSource, biz.TrainingLogOptions) (biz.TrainingLogResult, error) {
	r.calls++
	return biz.TrainingLogResult{}, fmt.Errorf("maintenance must not read logs")
}

func TestLegacyRetirementTrainingIsReadOnlyAndKeepsExactOriginalExit(t *testing.T) {
	for _, attack := range []string{"none", "uid", "container-id", "exit-code", "restart", "running", "extra-worker", "foreign-finalizer", "foreign-job-owner", "extra-owner", "resource-version", "namespace", "training-plan", "not-allowlisted"} {
		t.Run(attack, func(t *testing.T) {
			f, execution, owner, state, scope := legacyTrainingFixture(t)
			pod := &unstructured.Unstructured{Object: f.pod}
			switch attack {
			case "uid":
				pod.SetUID("replacement-pod")
			case "container-id":
			case "exit-code":
			case "running":
				f.pod["status"] = map[string]any{"phase": "Running", "containerStatuses": []any{map[string]any{"name": "node", "containerID": scope.TrainingContainerID, "restartCount": int64(0), "state": map[string]any{"running": map[string]any{}}}}}
			case "foreign-finalizer":
				pod.SetFinalizers([]string{exitEvidenceFinalizer, "foreign.example/keep"})
			case "foreign-job-owner":
				refs := pod.GetOwnerReferences()
				refs[0].UID = "foreign-job-uid"
				pod.SetOwnerReferences(refs)
			case "extra-owner":
				refs := pod.GetOwnerReferences()
				refs = append(refs, refs[0])
				pod.SetOwnerReferences(refs)
			case "resource-version":
				pod.SetResourceVersion("")
			case "namespace":
				f.namespace["metadata"].(map[string]any)["uid"] = "replacement-namespace"
			case "training-plan":
				state.Training = &biz.TrainingPlan{}
			case "not-allowlisted":
				execution.ExecutionID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
			}
			statuses, _, _ := unstructured.NestedSlice(f.pod, "status", "containerStatuses")
			if attack == "exit-code" {
				statuses[0].(map[string]any)["state"].(map[string]any)["terminated"].(map[string]any)["exitCode"] = int64(0)
				_ = unstructured.SetNestedSlice(f.pod, statuses, "status", "containerStatuses")
			}
			if attack == "container-id" {
				statuses[0].(map[string]any)["containerID"] = "containerd://other"
				_ = unstructured.SetNestedSlice(f.pod, statuses, "status", "containerStatuses")
			}
			if attack == "restart" {
				statuses[0].(map[string]any)["restartCount"] = int64(1)
				_ = unstructured.SetNestedSlice(f.pod, statuses, "status", "containerStatuses")
			}
			objects := []runtime.Object{}
			for _, object := range []map[string]any{f.namespace, f.train, f.jobset, f.job, f.pod, f.podGroup} {
				objects = append(objects, &unstructured.Unstructured{Object: object})
			}
			if attack == "extra-worker" {
				other := pod.DeepCopy()
				other.SetName("other-original-job-worker")
				other.SetUID(types.UID("other-worker-uid"))
				objects = append(objects, other)
			}
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Group: "batch", Version: "v1", Resource: "jobs"}: "JobList", {Version: "v1", Resource: "pods"}: "PodList"}, objects...)
			logs := &forbiddenLegacyLogReader{}
			adapter := trainer.New(client).WithTrainingLogs(logs)
			observed, err := adapter.ObserveLegacyRetirementTraining(context.Background(), execution, owner, state)
			if attack == "none" {
				if err != nil || !observed.WritersAbsent || observed.Outcome != "FAILED" || observed.SpecificationMismatch {
					for _, act := range client.Actions() {
						t.Log(act.GetVerb(), act.GetResource().Resource, act.GetNamespace())
					}
					t.Fatalf("exact original failed exit not observed: %+v %v", observed, err)
				}
				pods := 0
				for _, r := range observed.Resources {
					if r.Kind == "Pod" {
						pods++
						if r.UID != scope.TrainingPodUID || r.ExitCode == nil || *r.ExitCode != 143 || !r.APIObjectPresent || r.TrainingLog != nil {
							t.Fatal("original exit was replaced or fake log captured", r)
						}
					}
				}
				if pods != 1 {
					t.Fatal("worker cardinality", pods)
				}
			} else if err == nil && observed.WritersAbsent {
				t.Fatalf("%s authorized retirement: %+v", attack, observed)
			}
			if logs.calls != 0 {
				t.Fatal("maintenance read unavailable logs", logs.calls)
			}
			for _, action := range client.Actions() {
				if action.GetVerb() != "get" && action.GetVerb() != "list" {
					t.Fatal("read-only retirement mutated Kubernetes", action.GetVerb(), action.GetResource())
				}
			}
			// Ordinary ObserveTraining retains its log and finalizer contract.
			if attack == "none" {
				_, ordinaryErr := adapter.ObserveTraining(context.Background(), f.plan, *state.TrainingHandle, nil)
				if ordinaryErr == nil || logs.calls != 1 {
					t.Fatal("maintenance changed ordinary capture requirement", ordinaryErr, logs.calls)
				}
			}
		})
	}
}

// Reuse the existing domain/runtime/controller fixtures, changing only the
// synthetic Kubernetes envelope to the finite approved identity. No hash
// inventory, business logic, persistence or exit observer is substituted.
func legacyTrainingFixture(t *testing.T) (*trainingFixture, biz.Execution, biz.RunAuthorityCandidate, biz.ExecutionRuntime, biz.LegacyRetirementScope) {
	t.Helper()
	f := newTrainingFixture(t)
	scope, _ := biz.LookupLegacyRetirementScope("1cce950b-d6e9-453e-aa11-fffd8846224b")
	s := f.plan.Snapshot
	s.Environment.NamespaceName, s.Environment.NamespaceUID, s.Environment.ClusterID = scope.NamespaceName, scope.NamespaceUID, scope.ClusterID
	a := admissionfixture.Bind(t, biz.Admission{TenantID: scope.TenantID, OperationID: f.plan.OperationID, ExecutionID: scope.ExecutionID, Actor: "governance:user:legacy-retirement-test", Intent: conformance.IntentV1(), Snapshot: s, AcceptedAt: s.Timing.AcceptedAt})
	execution := biz.Execution{Admission: a}
	workspace := f.plan.Workspace
	workspace.NamespaceName, workspace.NamespaceUID, workspace.PVCName = scope.NamespaceName, scope.NamespaceUID, biz.ExpectedWorkspacePVCName(scope.ExecutionID)
	plan, err := biz.FreezeTrainingPlan(execution, workspace, f.plan.Timing.AdmittedAt)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = biz.BindTrainingRuntime(plan, biz.RuntimeTemplateBinding{NamespaceUID: scope.NamespaceUID, Name: a.Snapshot.RuntimePlan.Name, UID: f.plan.RuntimeBinding.UID, ObservedSpecSHA256: a.Snapshot.RuntimePlan.ExpectedSpecSHA256})
	if err != nil {
		t.Fatal(err)
	}
	f.plan = plan
	for _, object := range []map[string]any{f.namespace, f.pvc, f.jobset, f.job, f.pod} {
		u := &unstructured.Unstructured{Object: object}
		u.SetNamespace(scope.NamespaceName)
	}
	ns := &unstructured.Unstructured{Object: f.namespace}
	ns.SetName(scope.NamespaceName)
	ns.SetNamespace("")
	ns.SetUID(types.UID(scope.NamespaceUID))
	set := &unstructured.Unstructured{Object: f.jobset}
	set.SetName(plan.Name)
	refs := set.GetOwnerReferences()
	refs[0].Name = plan.Name
	set.SetOwnerReferences(refs)
	job := &unstructured.Unstructured{Object: f.job}
	job.SetName(plan.Name + "-trainer-0")
	refs = job.GetOwnerReferences()
	refs[0].Name = plan.Name
	job.SetOwnerReferences(refs)
	pod := &unstructured.Unstructured{Object: f.pod}
	pod.SetUID(types.UID(scope.TrainingPodUID))
	refs = pod.GetOwnerReferences()
	refs[0].Name = job.GetName()
	pod.SetOwnerReferences(refs)
	installFixtureRuntime(t, f)
	f.train = map[string]any{"apiVersion": "trainer.kubeflow.org/v1alpha1", "kind": "TrainJob", "metadata": map[string]any{"name": plan.Name, "namespace": scope.NamespaceName, "uid": trainingUID, "resourceVersion": "1", "generation": int64(1), "annotations": map[string]any{"modeldev.ani.io/tenant-id": plan.TenantID, "modeldev.ani.io/execution-id": plan.ExecutionID, "modeldev.ani.io/execution-spec-sha256": plan.SpecHash, "modeldev.ani.io/training-request-sha256": plan.RequestSHA256, "modeldev.ani.io/workspace-uid": plan.Workspace.PVCUID}}, "spec": map[string]any{"suspend": true}}
	f.setTerminal(t, 143)
	statuses, _, _ := unstructured.NestedSlice(f.pod, "status", "containerStatuses")
	statuses[0].(map[string]any)["containerID"] = scope.TrainingContainerID
	statuses[0].(map[string]any)["restartCount"] = int64(0)
	statuses[0].(map[string]any)["state"].(map[string]any)["terminated"].(map[string]any)["exitCode"] = int64(143)
	_ = unstructured.SetNestedSlice(f.pod, statuses, "status", "containerStatuses")
	handle := biz.TrainingHandle{NamespaceUID: scope.NamespaceUID, PVCUID: workspace.PVCUID, TrainJobUID: trainingUID}
	state := biz.ExecutionRuntime{Training: &plan, TrainingHandle: &handle, Observation: &biz.TrainingRuntimeObservation{}}
	owner := biz.RunAuthorityCandidate{TenantID: a.TenantID, ExecutionID: a.ExecutionID, OperationID: a.OperationID, SpecHash: a.SpecHash, RunID: scope.RunID, NamespaceName: scope.NamespaceName, NamespaceUID: scope.NamespaceUID, WorkflowName: scope.WorkflowName, WorkflowUID: scope.WorkflowUID, PlanHash: strings.Repeat("a", 64)}
	return f, execution, owner, state, scope
}
