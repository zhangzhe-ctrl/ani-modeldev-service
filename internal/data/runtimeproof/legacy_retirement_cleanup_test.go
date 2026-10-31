package runtimeproof

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

// The public archive constructor is intentionally not given a synthetic hash.
// Only its private shared cleanup core receives authored external-boundary
// records, exactly as the qualified one-time wrapper does after its public gate.
func TestLegacyRetirementStepCleanupSharesOriginalUIDAndVersionGuards(t *testing.T) {
	for _, attack := range []string{"none", "foreign-finalizer", "uid-replaced", "uid-at-patch", "version-at-patch", "workflow-reappears", "workflow-reappears-after-cleanup", "extra-owned-writer"} {
		t.Run(attack, func(t *testing.T) {
			f := newLegacyOwnerFixture(t)
			for i := range f.pods {
				f.pods[i].SetResourceVersion("1")
				f.pods[i].SetFinalizers([]string{stepExitEvidenceFinalizer})
			}
			objects := []runtime.Object{f.namespace}
			for i := range f.pods {
				objects = append(objects, &f.pods[i])
			}
			kube := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Version: "v1", Resource: "pods"}: "PodList"}, objects...)
			verifier := New(kube, f.runs, nil, legacyOwnerPlans{f.dispatch})
			evidence, err := verifier.verifyOwnerWritersAbsent(context.Background(), f.execution, f.owner, nil, &legacyRetirementWorkflow{workflow: f.workflow})
			if err != nil {
				t.Fatal("initial real original exits", err)
			}
			receipt := biz.StepExitCleanupReceipt{CloseGeneration: 1, Evidence: evidence, StartedAt: time.Now().UTC()}
			kube.ClearActions()
			if _, err := verifier.ReconcileStepExitCleanup(context.Background(), f.execution, f.owner, nil, receipt); err == nil {
				t.Fatal("ordinary step cleanup accepted an archived owner 404")
			}
			for _, a := range kube.Actions() {
				if a.GetVerb() == "patch" || a.GetVerb() == "delete" {
					t.Fatal("ordinary missing-owner cleanup mutated Pods")
				}
			}
			kube.ClearActions()
			target := evidence.Resources[0]
			gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
			object, err := kube.Tracker().Get(gvr, f.owner.NamespaceName, target.Name)
			if err != nil {
				t.Fatal(err)
			}
			pod := object.(*unstructured.Unstructured)
			switch attack {
			case "foreign-finalizer":
				pod.SetFinalizers([]string{stepExitEvidenceFinalizer, "foreign.example/retain"})
				_ = kube.Tracker().Update(gvr, pod, f.owner.NamespaceName)
			case "uid-replaced":
				pod.SetUID("replacement-uid")
				_ = kube.Tracker().Update(gvr, pod, f.owner.NamespaceName)
			case "workflow-reappears":
				_ = kube.Tracker().Add(f.workflow)
			case "extra-owned-writer":
				extra := pod.DeepCopy()
				extra.SetName("extra-current-owner-writer")
				extra.SetUID("extra-current-owner-uid")
				_ = kube.Tracker().Add(extra)
			}
			gets, patches, deletes := 0, 0, 0
			kube.PrependReactor("get", "workflows", func(ktesting.Action) (bool, runtime.Object, error) {
				gets++
				if attack == "workflow-reappears-after-cleanup" && gets > 1 {
					return true, f.workflow.DeepCopy(), nil
				}
				return false, nil, nil
			})
			kube.PrependReactor("patch", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
				patches++
				patchAction := action.(ktesting.PatchAction)
				current, err := kube.Tracker().Get(gvr, action.GetNamespace(), patchAction.GetName())
				if err != nil {
					return true, nil, err
				}
				actual := current.(*unstructured.Unstructured)
				if patches == 1 && attack == "uid-at-patch" {
					actual.SetUID("replaced-during-patch")
				}
				if patches == 1 && attack == "version-at-patch" {
					actual.SetResourceVersion("raced-version")
				}
				var patch []struct {
					Op, Path string
					Value    any
				}
				if json.Unmarshal(patchAction.GetPatch(), &patch) != nil || len(patch) != 5 {
					t.Error("not bounded original JSON CAS")
					return true, nil, fmt.Errorf("bad patch")
				}
				owners, _, _ := unstructured.NestedSlice(actual.Object, "metadata", "ownerReferences")
				expected := []any{string(actual.GetUID()), actual.GetResourceVersion(), owners, actual.GetFinalizers()}
				paths := []string{"/metadata/uid", "/metadata/resourceVersion", "/metadata/ownerReferences", "/metadata/finalizers"}
				for i, value := range expected {
					raw, _ := json.Marshal(value)
					var normalized any
					_ = json.Unmarshal(raw, &normalized)
					if patch[i].Op != "test" || patch[i].Path != paths[i] || !reflect.DeepEqual(patch[i].Value, normalized) {
						return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "pods"}, actual.GetName(), fmt.Errorf("identity CAS changed"))
					}
				}
				var remaining []string
				for _, f := range actual.GetFinalizers() {
					if f != stepExitEvidenceFinalizer {
						remaining = append(remaining, f)
					}
				}
				if remaining == nil {
					remaining = []string{}
				}
				raw, _ := json.Marshal(remaining)
				var expectedRemaining any
				_ = json.Unmarshal(raw, &expectedRemaining)
				if patch[4].Op != "replace" || patch[4].Path != "/metadata/finalizers" || !reflect.DeepEqual(patch[4].Value, expectedRemaining) {
					t.Error("foreign finalizer removal")
					return true, nil, fmt.Errorf("foreign finalizer")
				}
				actual.SetFinalizers(remaining)
				actual.SetResourceVersion("2")
				if err := kube.Tracker().Update(gvr, actual, action.GetNamespace()); err != nil {
					return true, nil, err
				}
				return true, actual, nil
			})
			kube.PrependReactor("delete", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
				deletes++
				a := action.(ktesting.DeleteAction)
				current, err := kube.Tracker().Get(gvr, action.GetNamespace(), a.GetName())
				if err != nil {
					return true, nil, err
				}
				actual := current.(*unstructured.Unstructured)
				options := a.GetDeleteOptions()
				if options.Preconditions == nil || options.Preconditions.UID == nil || options.Preconditions.ResourceVersion == nil || string(*options.Preconditions.UID) != string(actual.GetUID()) || *options.Preconditions.ResourceVersion != actual.GetResourceVersion() || options.PropagationPolicy == nil || *options.PropagationPolicy != metav1.DeletePropagationOrphan {
					t.Error("original UID/RV/orphan delete guard missing")
					return true, nil, fmt.Errorf("unguarded delete")
				}
				return false, nil, nil
			})
			archivedVerify := func(ctx context.Context, e biz.Execution, o biz.RunAuthorityCandidate, w *biz.WorkspaceBinding, fact biz.ManagedCloseEvidence) error {
				return verifier.verifyRecordedOwnerWritersAbsentWithArchive(ctx, e, o, w, fact, false, true)
			}
			removed, err := verifier.reconcileStepExitCleanup(context.Background(), f.execution, f.owner, nil, receipt, archivedVerify)
			if attack == "none" {
				if err != nil || len(removed) != len(evidence.Resources) || patches != len(evidence.Resources) || deletes != len(evidence.Resources) {
					t.Fatalf("archive-qualified shared cleanup failed: removed=%d patches=%d deletes=%d err=%v", len(removed), patches, deletes, err)
				}
				for _, r := range removed {
					if r.APIObjectPresent || r.UID == "" || r.ExitCode == nil {
						t.Fatal("cleanup lost original exit", r)
					}
				}
			} else if err == nil {
				t.Fatal("uncertain or foreign identity accepted", attack)
			}
			if attack == "foreign-finalizer" {
				current, err := kube.Tracker().Get(gvr, f.owner.NamespaceName, target.Name)
				if err != nil || !reflect.DeepEqual(current.(*unstructured.Unstructured).GetFinalizers(), []string{"foreign.example/retain"}) || deletes != 0 {
					t.Fatal("foreign finalizer or object lost", err, deletes)
				}
			}
			if attack == "uid-replaced" || attack == "workflow-reappears" || attack == "extra-owned-writer" {
				if patches != 0 || deletes != 0 {
					t.Fatal("failed initial writer proof allowed mutation", attack, patches, deletes)
				}
			}
			if attack == "uid-at-patch" || attack == "version-at-patch" {
				if deletes != 0 {
					t.Fatal("failed identity CAS allowed delete", attack, deletes)
				}
			}
		})
	}
}

func TestLegacyRetirementCleanupConstructorRejectsUnauthenticatedArchive(t *testing.T) {
	f := newLegacyOwnerFixture(t)
	for _, raw := range [][]byte{nil, []byte(`{"kind":"Workflow"}`)} {
		if boundary, err := NewLegacyRetirementBoundary(nil, f.execution, f.owner, biz.ExecutionRuntime{}, raw); err == nil || boundary != nil {
			t.Fatal("unqualified archive constructor accepted", err)
		}
	}
}
