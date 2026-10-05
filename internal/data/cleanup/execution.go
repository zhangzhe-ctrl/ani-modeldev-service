// Package cleanup implements bounded exact-UID disposal of closed CPU-P01
// controller objects. It never deletes Pods, work volumes, or publications.
package cleanup

import (
	"context"
	"reflect"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/trainer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type Adapter struct {
	kube    dynamic.Interface
	writers biz.OwnerWriterVerifier
}

func New(kube dynamic.Interface, writers biz.OwnerWriterVerifier) *Adapter {
	return &Adapter{kube: kube, writers: writers}
}

func (adapter *Adapter) VerifyCleanupWritersAbsent(ctx context.Context, record biz.OperationRecord) error {
	if ctx == nil || adapter == nil || adapter.kube == nil {
		return biz.ErrRuntimeNotReady
	}
	plan, err := biz.CleanupPlanFor(record)
	if err != nil {
		return err
	}
	namespace := record.Execution.Snapshot.Environment.NamespaceName
	ns, err := adapter.kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(ctx, namespace, metav1.GetOptions{})
	if err != nil || ns.GetAPIVersion() != "v1" || ns.GetKind() != "Namespace" || ns.GetName() != namespace || string(ns.GetUID()) != plan.NamespaceUID || ns.GetDeletionTimestamp() != nil {
		return biz.ErrCleanupBlocked
	}
	if record.Runtime.CloseEvidence.NoDispatch {
		if record.Dispatch != nil || record.Runtime.Training != nil || record.Runtime.Workspace != nil || len(plan.Targets) != 0 {
			return biz.ErrCleanupBlocked
		}
		return nil
	}
	if record.Authority == nil || adapter.writers == nil {
		return biz.ErrCleanupBlocked
	}
	if _, err := adapter.writers.VerifyOwnerWritersAbsent(ctx, record.Execution, *record.Authority, record.Runtime.Workspace); err != nil {
		return err
	}
	if record.Runtime.Training == nil {
		return nil
	}
	observation, err := trainer.New(adapter.kube).ObserveTraining(ctx, *record.Runtime.Training, *record.Runtime.TrainingHandle, record.Runtime.Observation.Resources)
	if err != nil {
		return err
	}
	if !observation.WritersAbsent {
		return biz.ErrCleanupBlocked
	}
	fresh := record
	fresh.Runtime.Observation = &observation
	actual, err := biz.CleanupPlanFor(fresh)
	if err != nil || len(actual.Targets) != len(plan.Targets) {
		return biz.ErrCleanupConflict
	}
	for i, resource := range actual.Targets {
		expected := plan.Targets[i]
		if resource.APIVersion != expected.APIVersion || resource.Kind != expected.Kind || resource.Namespace != expected.Namespace || resource.Name != expected.Name || resource.UID != expected.UID || resource.OwnerUID != expected.OwnerUID {
			return biz.ErrCleanupConflict
		}
	}
	return nil
}

func (adapter *Adapter) DeleteExecutionResource(ctx context.Context, record biz.OperationRecord, target biz.RuntimeResource) error {
	plan, err := biz.CleanupPlanFor(record)
	if err != nil || adapter == nil || adapter.kube == nil || ctx == nil {
		return biz.ErrCleanupBlocked
	}
	known := false
	for _, expected := range plan.Targets {
		known = known || reflect.DeepEqual(expected, target)
	}
	resource, valid := targetResource(target)
	if !known || !valid {
		return biz.ErrCleanupConflict
	}
	client := adapter.kube.Resource(resource).Namespace(target.Namespace)
	object, err := client.Get(ctx, target.Name, metav1.GetOptions{})
	// A missing object before our first exact delete is unresolved rather than
	// an inferred success. STARTED audit prevents retrying an unknown result.
	if err != nil || !exactObject(object, target) || object.GetDeletionTimestamp() != nil || object.GetResourceVersion() == "" || !creationStopped(object) {
		return biz.ErrCleanupConflict
	}
	uid, version := types.UID(target.UID), object.GetResourceVersion()
	orphan := metav1.DeletePropagationOrphan
	if err := client.Delete(ctx, target.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version}, PropagationPolicy: &orphan}); err != nil {
		return biz.ErrCleanupUncertain
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		current, err := client.Get(wait, target.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		// A UID replacement or unreadable read cannot confirm removal of the
		// original target. DeletionTimestamp still needs a confirmed absence.
		if err != nil || !exactObject(current, target) {
			return biz.ErrCleanupUncertain
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-wait.Done():
			timer.Stop()
			return biz.ErrCleanupUncertain
		case <-timer.C:
		}
	}
}

func targetResource(target biz.RuntimeResource) (schema.GroupVersionResource, bool) {
	switch {
	case target.APIVersion == "trainer.kubeflow.org/v1alpha1" && target.Kind == "TrainJob":
		return schema.GroupVersionResource{Group: "trainer.kubeflow.org", Version: "v1alpha1", Resource: "trainjobs"}, true
	case target.APIVersion == "jobset.x-k8s.io/v1alpha2" && target.Kind == "JobSet":
		return schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}, true
	case target.APIVersion == "batch/v1" && target.Kind == "Job":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, true
	default:
		return schema.GroupVersionResource{}, false
	}
}

func exactObject(object *unstructured.Unstructured, target biz.RuntimeResource) bool {
	if object == nil || object.GetAPIVersion() != target.APIVersion || object.GetKind() != target.Kind || object.GetNamespace() != target.Namespace || object.GetName() != target.Name || string(object.GetUID()) != target.UID {
		return false
	}
	owner := ""
	for _, reference := range object.GetOwnerReferences() {
		if reference.Controller != nil && *reference.Controller {
			if owner != "" {
				return false
			}
			owner = string(reference.UID)
		}
	}
	return owner == target.OwnerUID
}

func creationStopped(object *unstructured.Unstructured) bool {
	suspend, _, err := unstructured.NestedBool(object.Object, "spec", "suspend")
	if err != nil {
		return false
	}
	if suspend {
		return true
	}
	conditions, _, err := unstructured.NestedSlice(object.Object, "status", "conditions")
	if err != nil {
		return false
	}
	complete := "Complete"
	if object.GetKind() == "JobSet" {
		complete = "Completed"
	}
	seen, completeTrue, failedTrue := map[string]bool{}, false, false
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		kind, ok := condition["type"].(string)
		if !ok {
			return false
		}
		if kind != complete && kind != "Failed" {
			continue
		}
		if seen[kind] {
			return false
		}
		seen[kind] = true
		if kind == complete {
			completeTrue = condition["status"] == "True"
		} else {
			failedTrue = condition["status"] == "True"
		}
	}
	return completeTrue != failedTrue
}
