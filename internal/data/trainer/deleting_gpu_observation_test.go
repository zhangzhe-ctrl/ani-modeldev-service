package trainer_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/trainer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// The real observer must first offer the original process exit for durable
// storage. Observation alone cannot remove the retention finalizer: a suspended
// controller may already be deleting this Pod before its log tail is durable.
func TestGPUObservationRetainsDeletingSignedOriginalExitUntilRecorded(t *testing.T) {
	plan, handle, pod, client, keyFile, _ := ownerSigningFixture(t)
	signer, err := trainer.NewOwnerAssertionSigner(client, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := signer.SignPod(ctx, plan, handle, pod); err != nil {
		t.Fatal(err)
	}
	objects := newTrainingFixture(t)
	objects.plan = plan
	installFixtureRuntime(t, objects)
	train := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "trainer.kubeflow.org/v1alpha1", "kind": "TrainJob",
		"metadata": map[string]any{"name": plan.Name, "namespace": pod.GetNamespace(), "uid": handle.TrainJobUID,
			"annotations": map[string]any{"modeldev.ani.io/tenant-id": plan.TenantID,
				"modeldev.ani.io/execution-id": plan.ExecutionID, "modeldev.ani.io/execution-spec-sha256": plan.SpecHash,
				"modeldev.ani.io/training-request-sha256": plan.RequestSHA256, "modeldev.ani.io/workspace-uid": handle.PVCUID}},
		"spec": map[string]any{"suspend": true},
	}}
	for _, object := range []*unstructured.Unstructured{train, {Object: objects.namespace}, {Object: objects.podGroup}} {
		if err := client.Tracker().Add(object); err != nil {
			t.Fatal(err)
		}
	}
	for _, resource := range []schema.GroupVersionResource{
		{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"},
		{Group: "batch", Version: "v1", Resource: "jobs"},
	} {
		name := plan.Name
		if resource.Resource == "jobs" {
			name += "-trainer-0"
		}
		object, err := client.Resource(resource).Namespace(pod.GetNamespace()).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		_ = unstructured.SetNestedField(object.Object, true, "spec", "suspend")
		if _, err := client.Resource(resource).Namespace(pod.GetNamespace()).Update(ctx, object, metav1.UpdateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	resource := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(pod.GetNamespace())
	original, err := resource.Get(ctx, pod.GetName(), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertion := original.GetAnnotations()["accelerator.ani.io/owner-signature"]
	var runtimeSpec map[string]any
	if err := json.Unmarshal(plan.Snapshot.RuntimePlan.ExpectedSpec, &runtimeSpec); err != nil {
		t.Fatal(err)
	}
	jobs, _, _ := unstructured.NestedSlice(runtimeSpec, "template", "spec", "replicatedJobs")
	required, _, _ := unstructured.NestedStringMap(jobs[0].(map[string]any), "template", "spec", "template", "metadata", "annotations")
	annotations := original.GetAnnotations()
	for key, value := range required {
		annotations[key] = value
	}
	original.SetAnnotations(annotations)
	original.SetFinalizers([]string{exitEvidenceFinalizer})
	deleting := metav1.NewTime(time.Now().UTC())
	original.SetDeletionTimestamp(&deleting)
	original.Object["status"] = map[string]any{"phase": "Failed", "containerStatuses": []any{
		map[string]any{"name": "node", "state": map[string]any{"terminated": map[string]any{"exitCode": int64(143), "finishedAt": time.Now().UTC().Format(time.RFC3339)}}},
	}}
	if _, err := resource.Update(ctx, original, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	client.ClearActions()
	observer := trainer.New(client).WithOwnerSigner(signer)
	observed, err := observer.ObserveTraining(ctx, plan, handle, nil)
	if err != nil {
		t.Fatalf("DELETING_SIGNED_GPU_EXIT_NOT_OBSERVABLE: %v", err)
	}
	if observed.WritersAbsent || observed.SpecificationMismatch {
		t.Fatalf("new exit escaped durable-recording barrier or original vector: %+v", observed)
	}
	var exit biz.RuntimeResource
	for _, item := range observed.Resources {
		if item.UID == string(pod.GetUID()) {
			exit = item
		}
	}
	if !exit.Terminal || exit.ExitCode == nil || *exit.ExitCode != 143 || !exit.APIObjectPresent {
		t.Fatalf("original exit was not offered for persistence: %+v", exit)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() != "get" && action.GetVerb() != "list" {
			t.Fatalf("first exit observation changed Kubernetes before persistence: %s", action.GetVerb())
		}
	}
	confirmed, err := observer.ObserveTraining(ctx, plan, handle, observed.Resources)
	if err != nil || !confirmed.WritersAbsent {
		t.Fatalf("recorded original exit did not unblock close: %+v %v", confirmed, err)
	}
	after, err := resource.Get(ctx, pod.GetName(), metav1.GetOptions{})
	if err != nil || !reflect.DeepEqual(after.GetFinalizers(), []string{exitEvidenceFinalizer}) || after.GetAnnotations()["accelerator.ani.io/owner-signature"] != assertion {
		t.Fatalf("EXIT_FINALIZER_RELEASED_BEFORE_LOG_CAPTURE: observation changed the original retention or signature: %v", err)
	}
	if !reflect.DeepEqual(after.GetOwnerReferences(), original.GetOwnerReferences()) {
		t.Fatal("original controller association changed")
	}
	if err := observer.ReleaseRecordedTrainingExits(ctx, plan, handle, confirmed); !errors.Is(err, biz.ErrTrainingLogsUnavailable) {
		t.Fatalf("missing final tail allowed release: %v", err)
	}
	durable := terminalLogEvidence(t, plan, handle, confirmed)
	for _, attack := range []string{"unsigned", "replacement-uid", "running", "foreign-controller"} {
		t.Run("release rejects "+attack, func(t *testing.T) {
			changed := after.DeepCopy()
			switch attack {
			case "unsigned":
				annotations := changed.GetAnnotations()
				delete(annotations, "accelerator.ani.io/owner-signature")
				changed.SetAnnotations(annotations)
			case "replacement-uid":
				changed.SetUID(types.UID("replacement-pod-uid"))
			case "running":
				changed.Object["status"] = pod.Object["status"]
			case "foreign-controller":
				refs := changed.GetOwnerReferences()
				refs[0].UID = types.UID("foreign-job-uid")
				changed.SetOwnerReferences(refs)
			}
			if _, err := resource.Update(ctx, changed, metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			client.ClearActions()
			if err := observer.ReleaseRecordedTrainingExits(ctx, plan, handle, durable); err == nil {
				t.Fatal("untrusted original exit accepted for release")
			}
			for _, action := range client.Actions() {
				if action.GetVerb() != "get" && action.GetVerb() != "list" {
					t.Fatalf("rejected release changed API: %s", action.GetVerb())
				}
			}
			if _, err := resource.Update(ctx, after.DeepCopy(), metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := observer.ReleaseRecordedTrainingExits(ctx, plan, handle, durable); err != nil {
		t.Fatal("durable original log/exit did not release", err)
	}
	released, err := resource.Get(ctx, pod.GetName(), metav1.GetOptions{})
	if err != nil || len(released.GetFinalizers()) != 0 || released.GetAnnotations()["accelerator.ani.io/owner-signature"] != assertion {
		t.Fatal("release changed identity or left protection", err)
	}
}

func TestDeletingGPUOwnerAssertionNeverCreatesOrRepairsIdentity(t *testing.T) {
	for _, attack := range []string{"unsigned", "wrong-signature", "replacement-uid", "running", "foreign-controller"} {
		t.Run(attack, func(t *testing.T) {
			plan, handle, pod, client, keyFile, _ := ownerSigningFixture(t)
			signer, err := trainer.NewOwnerAssertionSigner(client, keyFile)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := signer.SignPod(ctx, plan, handle, pod); err != nil {
				t.Fatal(err)
			}
			resource := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(pod.GetNamespace())
			actual, _ := resource.Get(ctx, pod.GetName(), metav1.GetOptions{})
			deleting := metav1.NewTime(time.Now().UTC())
			actual.SetDeletionTimestamp(&deleting)
			actual.Object["status"] = map[string]any{"phase": "Failed", "containerStatuses": []any{
				map[string]any{"name": "node", "state": map[string]any{"terminated": map[string]any{"exitCode": int64(143), "finishedAt": time.Now().UTC().Format(time.RFC3339)}}},
			}}
			switch attack {
			case "unsigned", "wrong-signature":
				annotations := actual.GetAnnotations()
				delete(annotations, "accelerator.ani.io/owner-signature")
				if attack == "wrong-signature" {
					annotations["accelerator.ani.io/owner-signature"] = "not-the-verified-original-signature"
				}
				actual.SetAnnotations(annotations)
			case "replacement-uid":
				actual.SetUID(types.UID("replacement-pod-uid"))
			case "running":
				actual.Object["status"] = pod.Object["status"]
			case "foreign-controller":
				refs := actual.GetOwnerReferences()
				refs[0].UID = types.UID("foreign-job-uid")
				actual.SetOwnerReferences(refs)
			}
			if _, err := resource.Update(ctx, actual, metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			client.ClearActions()
			if err := signer.SignPod(ctx, plan, handle, pod); !errors.Is(err, biz.ErrRuntimeConflict) {
				t.Fatalf("untrusted deleting Pod accepted: %v", err)
			}
			for _, action := range client.Actions() {
				if action.GetVerb() != "get" {
					t.Fatalf("untrusted deleting Pod reached a mutation: %s", action.GetVerb())
				}
			}
		})
	}
}
