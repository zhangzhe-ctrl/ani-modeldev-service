package trainer

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const trainingExitEvidenceFinalizer = "modeldev.ani.io/training-exit-evidence"

// ReleaseRecordedTrainingExits rechecks the production observation boundary and
// releases only original terminal Pods whose bounded logs are already durable.
func (a *Adapter) ReleaseRecordedTrainingExits(ctx context.Context, plan biz.TrainingPlan, handle biz.TrainingHandle, recorded biz.TrainingRuntimeObservation) error {
	state := biz.ExecutionRuntime{Training: &plan, TrainingHandle: &handle, Observation: &recorded}
	record := biz.QueryRecord{Execution: biz.Execution{Admission: biz.Admission{ExecutionID: plan.ExecutionID, Snapshot: plan.Snapshot}}, Runtime: state}
	if recorded.Handle != handle || recorded.ObservedAt.IsZero() || biz.ValidateTerminalTrainingLogs(record, recorded.RecordedLogs) != nil {
		return biz.ErrRuntimeConflict
	}
	_, err := a.observeTraining(ctx, plan, handle, recorded.Resources, &recorded)
	return err
}

// Observation offers exit facts without releasing the source of their logs.
// Only the explicit post-commit call above may remove our finalizer.
func (a *Adapter) releaseRecordedPodExits(ctx context.Context, plan biz.TrainingPlan, handle biz.TrainingHandle, resources map[string]biz.RuntimeResource, objects map[string]*unstructured.Unstructured, history []biz.RuntimeResource, recorded *biz.TrainingRuntimeObservation) (bool, error) {
	pending := false
	for uid, fact := range resources {
		pod := objects[uid]
		if fact.Kind != "Pod" || pod == nil {
			continue
		}
		finalizers, _, err := unstructured.NestedStringSlice(pod.Object, "metadata", "finalizers")
		if err != nil {
			return false, biz.ErrRuntimeConflict
		}
		remaining := make([]string, 0, len(finalizers))
		retained := false
		for _, finalizer := range finalizers {
			if finalizer == trainingExitEvidenceFinalizer {
				retained = true
			} else {
				remaining = append(remaining, finalizer)
			}
		}
		if !retained {
			continue
		}
		if !fact.Terminal || fact.ExitCode == nil || !recordedPodExit(fact, history) {
			if recorded != nil {
				return false, biz.ErrRuntimeConflict
			}
			pending = true
			continue
		}
		logged := false
		if recorded != nil {
			for _, log := range recorded.RecordedLogs {
				logged = logged || log.Source.PodUID == fact.UID && log.Source.PodName == fact.Name && log.Source.Namespace == fact.Namespace && log.Source.OwnerUID == fact.OwnerUID
			}
			if !logged {
				return false, biz.ErrTrainingLogsUnavailable
			}
		}
		if recorded != nil && plan.Snapshot.ComputeOrigin.DeviceMode == "gpu" {
			if a.ownerSigner == nil {
				return false, biz.ErrRuntimeNotReady
			}
			if err := a.ownerSigner.signPod(ctx, plan, handle, pod, false); err != nil {
				return false, err
			}
		}
		job := objects[fact.OwnerUID]
		jobFact, ok := resources[fact.OwnerUID]
		if !ok || job == nil || jobFact.Kind != "Job" || jobFact.APIVersion != "batch/v1" || jobFact.Namespace != plan.Workspace.NamespaceName || !controllerIs(pod, "batch/v1", "Job", jobFact.Name, jobFact.UID) {
			return false, biz.ErrRuntimeConflict
		}
		set := objects[jobFact.OwnerUID]
		setFact, ok := resources[jobFact.OwnerUID]
		if !ok || set == nil || setFact.Kind != "JobSet" || setFact.APIVersion != "jobset.x-k8s.io/v1alpha2" || setFact.Namespace != plan.Workspace.NamespaceName || !controllerIs(job, setFact.APIVersion, setFact.Kind, setFact.Name, setFact.UID) || !controllerIs(set, "trainer.kubeflow.org/v1alpha1", "TrainJob", plan.Name, handle.TrainJobUID) {
			return false, biz.ErrRuntimeConflict
		}
		if recorded == nil {
			continue
		}
		if pod.GetResourceVersion() == "" {
			return false, biz.ErrRuntimeConflict
		}
		owners, _, _ := unstructured.NestedSlice(pod.Object, "metadata", "ownerReferences")
		patch, err := json.Marshal([]map[string]any{
			{"op": "test", "path": "/metadata/uid", "value": fact.UID},
			{"op": "test", "path": "/metadata/resourceVersion", "value": pod.GetResourceVersion()},
			{"op": "test", "path": "/metadata/ownerReferences", "value": owners},
			{"op": "test", "path": "/metadata/finalizers", "value": finalizers},
			{"op": "replace", "path": "/metadata/finalizers", "value": remaining},
		})
		if err != nil {
			return false, biz.ErrTrainingUnavailable
		}
		result, err := a.client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(plan.Workspace.NamespaceName).Patch(ctx, fact.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return false, biz.ErrTrainingUnavailable
		}
		if result == nil || result.GetAPIVersion() != fact.APIVersion || result.GetKind() != fact.Kind || result.GetNamespace() != fact.Namespace || result.GetName() != fact.Name || string(result.GetUID()) != fact.UID || !reflect.DeepEqual(result.GetOwnerReferences(), pod.GetOwnerReferences()) {
			return false, biz.ErrRuntimeConflict
		}
		resultFinalizers, _, err := unstructured.NestedStringSlice(result.Object, "metadata", "finalizers")
		if err != nil || !slices.Equal(resultFinalizers, remaining) {
			return false, biz.ErrRuntimeConflict
		}
		terminal, code := podTermination(result)
		if !terminal || code == nil || *code != *fact.ExitCode {
			return false, biz.ErrRuntimeConflict
		}
	}
	return pending, nil
}

func recordedPodExit(current biz.RuntimeResource, history []biz.RuntimeResource) bool {
	found := false
	for _, prior := range history {
		if prior.UID != current.UID {
			continue
		}
		if found || prior.APIVersion != current.APIVersion || prior.Kind != current.Kind || prior.Namespace != current.Namespace || prior.Name != current.Name || prior.OwnerUID != current.OwnerUID || !prior.Terminal || prior.ExitCode == nil || *prior.ExitCode != *current.ExitCode {
			return false
		}
		found = true
	}
	return found
}
