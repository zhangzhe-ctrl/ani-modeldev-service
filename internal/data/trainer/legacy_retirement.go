package trainer

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// ObserveLegacyRetirementTraining is read-only. It shares the complete original
// training graph/exit observer, replacing only log capture and finalizer release
// with the approved fixed original UID/container exit check. It never calls the
// log API or represents unavailable bytes as a captured stream.
func (a *Adapter) ObserveLegacyRetirementTraining(ctx context.Context, execution biz.Execution, owner biz.RunAuthorityCandidate, state biz.ExecutionRuntime) (biz.TrainingRuntimeObservation, error) {
	scope, ok := biz.LookupLegacyRetirementScope(execution.ExecutionID)
	if !ok || scope.TrainingPodUID == "" || !biz.LegacyRetirementScopeMatches(execution, owner) || state.Training == nil || state.TrainingHandle == nil || state.Observation == nil || state.TrainingRejection != nil {
		return biz.TrainingRuntimeObservation{}, biz.ErrRuntimeConflict
	}
	if _, _, err := execution.CanonicalPayloads(); err != nil {
		return biz.TrainingRuntimeObservation{}, err
	}
	if state.Training.ExecutionID != execution.ExecutionID || state.Training.TenantID != execution.TenantID || !reflect.DeepEqual(state.Training.Snapshot, execution.Snapshot) {
		return biz.TrainingRuntimeObservation{}, biz.ErrRuntimeConflict
	}
	return a.observeTraining(ctx, *state.Training, *state.TrainingHandle, state.Observation.Resources, &scope)
}

func legacyOriginalTrainingExit(scope biz.LegacyRetirementScope, resources map[string]biz.RuntimeResource, objects map[string]*unstructured.Unstructured) error {
	pods := 0
	for uid, fact := range resources {
		if fact.Kind != "Pod" {
			continue
		}
		pods++
		pod := objects[uid]
		if uid != scope.TrainingPodUID || pod == nil || pod.GetNamespace() != scope.NamespaceName || pod.GetName() != fact.Name || string(pod.GetUID()) != fact.UID || !legacyOriginalJobOwner(pod, fact.OwnerUID) || !fact.APIObjectPresent || !fact.Terminal || fact.ExitCode == nil || *fact.ExitCode != 143 || fact.TrainingLog != nil || fact.InitializationAbort != nil || fact.SandboxTermination != nil || pod.GetResourceVersion() == "" {
			return biz.ErrRuntimeConflict
		}
		statuses, found, err := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
		if err != nil || !found || len(statuses) != 1 {
			return biz.ErrRuntimeConflict
		}
		status, ok := statuses[0].(map[string]any)
		if !ok || status["name"] != "node" || status["containerID"] != scope.TrainingContainerID {
			return biz.ErrRuntimeConflict
		}
		restarts, found, err := unstructured.NestedInt64(status, "restartCount")
		if err != nil || !found || restarts != 0 {
			return biz.ErrRuntimeConflict
		}
		terminal, code := podTermination(pod)
		if !terminal || code == nil || *code != 143 {
			return biz.ErrRuntimeConflict
		}
		for _, field := range []string{"containerStatuses", "initContainerStatuses", "ephemeralContainerStatuses"} {
			entries, _, err := unstructured.NestedSlice(pod.Object, "status", field)
			if err != nil {
				return biz.ErrRuntimeConflict
			}
			for _, raw := range entries {
				entry, ok := raw.(map[string]any)
				if !ok {
					return biz.ErrRuntimeConflict
				}
				text, found, err := unstructured.NestedString(entry, "state", "terminated", "finishedAt")
				at, e := time.Parse(time.RFC3339Nano, text)
				if err != nil || !found || e != nil || at.IsZero() || at.After(time.Now().UTC()) {
					return biz.ErrRuntimeConflict
				}
			}
		}
		for _, f := range pod.GetFinalizers() {
			if f != trainingExitEvidenceFinalizer {
				return biz.ErrCleanupBlocked
			}
		}
	}
	if pods != 1 {
		return biz.ErrRuntimeConflict
	}
	return nil
}

// ReleaseLegacyRetirementTrainingExit requires the committed close journal and
// original cleanup intent before stripping this service's sole finalizer. The
// exact original exit and full graph are refreshed before the UID/RV CAS patch.
func (a *Adapter) ReleaseLegacyRetirementTrainingExit(ctx context.Context, execution biz.Execution, owner biz.RunAuthorityCandidate, state biz.ExecutionRuntime) error {
	if !biz.LegacyRetirementCleanupAuthorized(execution, owner, state) {
		return biz.ErrCleanupBlocked
	}
	scope, _ := biz.LookupLegacyRetirementScope(execution.ExecutionID)
	var fact biz.RuntimeResource
	for _, r := range state.Observation.Resources {
		if r.UID == scope.TrainingPodUID {
			fact = r
		}
	}
	client := a.client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(scope.NamespaceName)
	pod, err := client.Get(ctx, fact.Name, metav1.GetOptions{})
	// A previous exact finalizer CAS may have succeeded with a lost reply. The
	// committed original exit and requested cleanup identity remain mandatory;
	// this NotFound confirms no new exit and authorizes no resource mutation.
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil || pod == nil || string(pod.GetUID()) != scope.TrainingPodUID || pod.GetResourceVersion() == "" {
		return biz.ErrRuntimeConflict
	}
	if !slices.Contains(pod.GetFinalizers(), trainingExitEvidenceFinalizer) {
		return nil
	}
	observed, err := a.ObserveLegacyRetirementTraining(ctx, execution, owner, state)
	if err != nil {
		return biz.ErrCleanupBlocked
	}
	kind, err := biz.ClassifyLegacyRetirementTrainingExit(execution, owner, state, observed)
	if err != nil || kind != state.LegacyRetirement.TrainingExitKind {
		return biz.ErrCleanupBlocked
	}
	// Refresh once more after the full writer graph and bind this exact patch to
	// the same original owner. An intervening UID/RV/owner change fails its CAS.
	pod, err = client.Get(ctx, fact.Name, metav1.GetOptions{})
	if err != nil || pod == nil || string(pod.GetUID()) != scope.TrainingPodUID || pod.GetResourceVersion() == "" {
		return biz.ErrRuntimeConflict
	}
	if err := legacyOriginalTrainingExit(scope, map[string]biz.RuntimeResource{fact.UID: fact}, map[string]*unstructured.Unstructured{fact.UID: pod}); err != nil {
		return err
	}
	finalizers := pod.GetFinalizers()
	remaining := []string{}
	if !slices.Contains(finalizers, trainingExitEvidenceFinalizer) {
		return nil
	}
	owners, _, _ := unstructured.NestedSlice(pod.Object, "metadata", "ownerReferences")
	patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": scope.TrainingPodUID}, {"op": "test", "path": "/metadata/resourceVersion", "value": pod.GetResourceVersion()}, {"op": "test", "path": "/metadata/ownerReferences", "value": owners}, {"op": "test", "path": "/metadata/finalizers", "value": finalizers}, {"op": "replace", "path": "/metadata/finalizers", "value": remaining}})
	result, err := client.Patch(ctx, fact.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
	if apierrors.IsNotFound(err) {
		return biz.ErrCleanupUncertain
	}
	if err != nil || result == nil || string(result.GetUID()) != scope.TrainingPodUID || !reflect.DeepEqual(result.GetOwnerReferences(), pod.GetOwnerReferences()) || len(result.GetFinalizers()) != 0 {
		return biz.ErrCleanupUncertain
	}
	return nil
}

func legacyOriginalJobOwner(pod *unstructured.Unstructured, uid string) bool {
	if uid == "" {
		return false
	}
	owners := pod.GetOwnerReferences()
	if len(owners) != 1 {
		return false
	}
	o := owners[0]
	return o.APIVersion == "batch/v1" && o.Kind == "Job" && o.Name != "" && string(o.UID) == uid && o.Controller != nil && *o.Controller
}
