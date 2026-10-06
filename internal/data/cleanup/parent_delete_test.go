package cleanup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/cleanup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"
)

func TestCleanupDisposesOriginalOrphanChildOnlyAfterItsParentsAreGone(t *testing.T) {
	f := newCleanupResolutionFixture(t)
	plan, err := biz.CleanupPlanFor(f.record)
	if err != nil {
		t.Fatal(err)
	}
	var target biz.RuntimeResource
	for _, candidate := range plan.Targets {
		if candidate.Kind == "JobSet" {
			target = candidate
		}
	}
	object := cleanupControllerObject(target, "9", false)
	resource := controllerResource(target)
	if _, err := f.kube.Resource(resource).Namespace(f.namespace()).Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	f.kube.ClearActions()
	err = cleanup.New(f.kube, nil).DeleteExecutionResource(context.Background(), f.record, target)
	if err != nil {
		t.Fatalf("original orphan child was not disposed after its exact parent was absent: %v", err)
	}
	if _, err := f.kube.Resource(resource).Namespace(f.namespace()).Get(context.Background(), target.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("original child still present after successful cleanup: %v", err)
	}
	deletes := 0
	for _, action := range f.kube.Actions() {
		if action.GetVerb() != "delete" {
			continue
		}
		deletes++
		options := action.(k8stesting.DeleteAction).GetDeleteOptions()
		if options.Preconditions == nil || options.Preconditions.UID == nil || *options.Preconditions.UID != types.UID(target.UID) || options.Preconditions.ResourceVersion == nil || *options.Preconditions.ResourceVersion != "9" || options.PropagationPolicy == nil || *options.PropagationPolicy != metav1.DeletePropagationOrphan {
			t.Fatal("original child delete lost exact UID/current RV/Orphan guards")
		}
	}
	if deletes != 1 {
		t.Fatalf("cleanup sent %d deletes instead of one", deletes)
	}
}

func TestCleanupWaitsForGoneOriginalOrphanChildAfterConfirmedDelete(t *testing.T) {
	f := newCleanupResolutionFixture(t)
	plan, err := biz.CleanupPlanFor(f.record)
	if err != nil {
		t.Fatal(err)
	}
	var target biz.RuntimeResource
	for _, candidate := range plan.Targets {
		if candidate.Kind == "JobSet" {
			target = candidate
		}
	}
	object := cleanupControllerObject(target, "9", false)
	deleted, pendingRead := false, false
	f.kube.PrependReactor("delete", "jobsets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if deleted {
			t.Fatal("original child DELETE was resent while waiting for Orphan GC")
		}
		deleted = true
		return true, nil, nil
	})
	f.kube.PrependReactor("get", "jobsets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if !deleted {
			return true, object.DeepCopy(), nil
		}
		if !pendingRead {
			pendingRead = true
			present := object.DeepCopy()
			at := metav1.Now()
			present.SetDeletionTimestamp(&at)
			return true, present, nil
		}
		return true, nil, apierrors.NewNotFound(controllerResource(target).GroupResource(), target.Name)
	})
	if err := cleanup.New(f.kube, nil).DeleteExecutionResource(context.Background(), f.record, target); err != nil || !deleted || !pendingRead {
		t.Fatalf("confirmed Orphan DELETE could not wait for the original ownerless UID to disappear: %v", err)
	}
}

func TestCleanupRefusesSuspendedOriginalTrainParent(t *testing.T) {
	f := newCleanupResolutionFixture(t)
	plan, err := biz.CleanupPlanFor(f.record)
	if err != nil {
		t.Fatal(err)
	}
	var target biz.RuntimeResource
	for _, candidate := range plan.Targets {
		object := cleanupControllerObject(candidate, "9", candidate.OwnerUID != "")
		object.SetGeneration(1)
		object.Object["spec"] = map[string]any{"suspend": false}
		if candidate.Kind == "TrainJob" {
			target = candidate
			object.Object["spec"] = map[string]any{"suspend": true}
		}
		if _, err := f.kube.Resource(controllerResource(candidate)).Namespace(f.namespace()).Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	f.kube.ClearActions()
	err = cleanup.New(f.kube, nil).DeleteExecutionResource(context.Background(), f.record, target)
	if !errors.Is(err, biz.ErrCleanupBlocked) {
		t.Fatalf("suspended original parent could run controller owner reattachment during Orphan disposal: %v", err)
	}
	for _, action := range f.kube.Actions() {
		if action.GetVerb() == "delete" {
			t.Fatal("suspended original TrainJob must use reviewed maintenance, not product DELETE")
		}
	}
}

func TestCleanupStartsOnlyFromUnmodifiedUnsuspendedOriginalParents(t *testing.T) {
	for _, name := range []string{"original", "changed-train-generation", "suspended-set", "changed-set-generation", "replacement-set", "changed-set-owner", "deleting-set", "nonterminal-set", "unreadable-set", "empty-set-body"} {
		t.Run(name, func(t *testing.T) {
			f := newCleanupResolutionFixture(t)
			plan, err := biz.CleanupPlanFor(f.record)
			if err != nil {
				t.Fatal(err)
			}
			var trainTarget biz.RuntimeResource
			want := biz.ErrCleanupBlocked
			for _, candidate := range plan.Targets {
				object := cleanupControllerObject(candidate, "9", candidate.OwnerUID != "")
				object.SetGeneration(1)
				object.Object["spec"] = map[string]any{"suspend": false}
				if candidate.Kind == "TrainJob" {
					trainTarget = candidate
					if name == "changed-train-generation" {
						object.SetGeneration(2)
					}
				}
				if candidate.Kind == "JobSet" {
					switch name {
					case "suspended-set":
						object.Object["spec"] = map[string]any{"suspend": true}
					case "changed-set-generation":
						object.SetGeneration(2)
					case "replacement-set":
						want = biz.ErrCleanupConflict
						object.SetUID("replacement-set")
					case "changed-set-owner":
						want = biz.ErrCleanupConflict
						owners := object.GetOwnerReferences()
						owners[0].UID = "other-parent"
						object.SetOwnerReferences(owners)
					case "deleting-set":
						want = biz.ErrCleanupConflict
						at := metav1.Now()
						object.SetDeletionTimestamp(&at)
					case "nonterminal-set":
						want = biz.ErrCleanupConflict
						object.Object["status"] = map[string]any{}
					}
				}
				if _, err := f.kube.Resource(controllerResource(candidate)).Namespace(f.namespace()).Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			if name == "unreadable-set" || name == "empty-set-body" {
				want = biz.ErrCleanupUncertain
				f.kube.PrependReactor("get", "jobsets", func(action k8stesting.Action) (bool, runtime.Object, error) {
					if name == "empty-set-body" {
						return true, nil, nil
					}
					return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "jobset.x-k8s.io", Resource: "jobsets"}, "original-set", errors.New("synthetic K8s boundary refusal"))
				})
			}
			f.kube.ClearActions()
			err = cleanup.New(f.kube, nil).DeleteExecutionResource(context.Background(), f.record, trainTarget)
			deletes := 0
			for _, action := range f.kube.Actions() {
				if action.GetVerb() == "delete" {
					deletes++
				}
			}
			if name == "original" {
				if err != nil || deletes != 1 {
					t.Fatalf("original generation1/unsuspended/terminal chain could not begin bounded parent-first cleanup: %v, deletes=%d", err, deletes)
				}
			} else if !errors.Is(err, want) || deletes != 0 {
				t.Fatalf("modified or unreadable original parent entered product disposal: %v, deletes=%d (want %v)", err, deletes, want)
			}
		})
	}
}

func TestCleanupPlanReadBlocksSuspendedOriginalParentsBeforeAnyMutation(t *testing.T) {
	f := newCleanupResolutionFixture(t)
	plan, err := biz.CleanupPlanFor(f.record)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range plan.Targets {
		object := cleanupControllerObject(candidate, "9", candidate.OwnerUID != "")
		object.SetGeneration(1)
		object.Object["spec"] = map[string]any{"suspend": candidate.Kind == "TrainJob"}
		if _, err := f.kube.Resource(controllerResource(candidate)).Namespace(f.namespace()).Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	f.kube.ClearActions()
	if err := cleanup.New(f.kube, f.proof).VerifyCleanupWritersAbsent(context.Background(), f.record); !errors.Is(err, biz.ErrCleanupBlocked) {
		t.Fatalf("planning did not reject the unsupported suspended profile before reserving cleanup: %v", err)
	}
	f.assertReadOnly(t)
}

func TestCleanupCannotDeleteChildrenWhileOriginalParentAbsenceIsUnproven(t *testing.T) {
	for _, name := range []string{"parent-present", "parent-replaced", "parent-forbidden", "parent-nil-body", "ancestor-present", "missing-parent-plan", "handle-changed", "namespace-handle-changed", "child-replaced", "child-changed-owner", "child-extra-owner", "child-deleting", "child-running"} {
		t.Run(name, func(t *testing.T) {
			f := newCleanupResolutionFixture(t)
			plan, err := biz.CleanupPlanFor(f.record)
			if err != nil {
				t.Fatal(err)
			}
			byKind := map[string]biz.RuntimeResource{}
			for _, target := range plan.Targets {
				byKind[target.Kind] = target
			}
			target := byKind["JobSet"]
			if name == "ancestor-present" {
				target = byKind["Job"]
			}
			child := cleanupControllerObject(target, "9", name == "parent-present")
			want := biz.ErrCleanupConflict
			switch name {
			case "parent-present", "parent-replaced", "ancestor-present":
				parent := cleanupControllerObject(byKind["TrainJob"], "7", false)
				if name == "parent-replaced" {
					parent.SetUID("replacement-parent")
				}
				if _, err := f.kube.Resource(controllerResource(byKind["TrainJob"])).Namespace(f.namespace()).Create(context.Background(), parent, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			case "parent-forbidden", "parent-nil-body":
				want = biz.ErrCleanupUncertain
				f.kube.PrependReactor("get", "trainjobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					if name == "parent-nil-body" {
						return true, nil, nil
					}
					return true, nil, apierrors.NewForbidden(controllerResource(byKind["TrainJob"]).GroupResource(), byKind["TrainJob"].Name, errors.New("synthetic K8s boundary refusal"))
				})
			case "missing-parent-plan":
				for i := range f.record.Runtime.Observation.Resources {
					if f.record.Runtime.Observation.Resources[i].Kind == "TrainJob" {
						f.record.Runtime.Observation.Resources[i].UID = "outside-plan-parent"
					}
				}
			case "handle-changed":
				f.record.Runtime.TrainingHandle.TrainJobUID = "other-train"
			case "namespace-handle-changed":
				f.record.Runtime.TrainingHandle.NamespaceUID = "other-namespace"
			case "child-replaced":
				child.SetUID("replacement-child")
			case "child-changed-owner":
				controller := true
				child.SetOwnerReferences([]metav1.OwnerReference{{UID: "other-parent", Controller: &controller}})
			case "child-extra-owner":
				child.SetOwnerReferences([]metav1.OwnerReference{{UID: "extra-reference"}})
			case "child-deleting":
				at := metav1.Now()
				child.SetDeletionTimestamp(&at)
			case "child-running":
				child.Object["status"] = map[string]any{}
			}
			if _, err := f.kube.Resource(controllerResource(target)).Namespace(f.namespace()).Create(context.Background(), child, metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
			f.kube.ClearActions()
			err = cleanup.New(f.kube, nil).DeleteExecutionResource(context.Background(), f.record, target)
			if !errors.Is(err, want) {
				t.Fatalf("unproven original parent/child was accepted: %v (want %v)", err, want)
			}
			for _, action := range f.kube.Actions() {
				if action.GetVerb() == "delete" {
					t.Fatalf("unproven original chain sent DELETE in %s", name)
				}
			}
		})
	}
}

func cleanupControllerObject(target biz.RuntimeResource, version string, owned bool) *unstructured.Unstructured {
	condition := "Complete"
	if target.Kind == "JobSet" {
		condition = "Completed"
	}
	object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": target.APIVersion, "kind": target.Kind,
		"metadata": map[string]any{"name": target.Name, "namespace": target.Namespace, "uid": target.UID, "resourceVersion": version},
		"status":   map[string]any{"conditions": []any{map[string]any{"type": condition, "status": "True"}}}}}
	if owned {
		controller := true
		object.SetOwnerReferences([]metav1.OwnerReference{{UID: types.UID(target.OwnerUID), Controller: &controller}})
	}
	return object
}

func controllerResource(target biz.RuntimeResource) schema.GroupVersionResource {
	switch target.Kind {
	case "TrainJob":
		return schema.GroupVersionResource{Group: "trainer.kubeflow.org", Version: "v1alpha1", Resource: "trainjobs"}
	case "JobSet":
		return schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}
	default:
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	}
}
