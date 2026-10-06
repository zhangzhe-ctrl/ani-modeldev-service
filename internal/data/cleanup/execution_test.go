package cleanup_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/cleanup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func TestCleanupDeleteUsesExactUIDVersionAndOrphansEvidence(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "original", true: "replacement"}[changed], func(t *testing.T) {
			closed := time.Date(2026, 10, 5, 2, 0, 0, 0, time.UTC)
			target := biz.RuntimeResource{APIVersion: "batch/v1", Kind: "Job", Namespace: "test-ns", Name: "training-node", UID: "original-job", OwnerUID: "original-set", Terminal: true, APIObjectPresent: true}
			parents := []biz.RuntimeResource{
				{APIVersion: "trainer.kubeflow.org/v1alpha1", Kind: "TrainJob", Namespace: "test-ns", Name: "training", UID: "original-train", Terminal: true, APIObjectPresent: true},
				{APIVersion: "jobset.x-k8s.io/v1alpha2", Kind: "JobSet", Namespace: "test-ns", Name: "training", UID: "original-set", OwnerUID: "original-train", Terminal: true, APIObjectPresent: true},
			}
			record := biz.OperationRecord{QueryRecord: biz.QueryRecord{Execution: biz.Execution{Admission: biz.Admission{TenantID: "11111111-1111-4111-8111-111111111111", ExecutionID: "22222222-2222-4222-8222-222222222222", OperationID: "33333333-3333-4333-8333-333333333333", SpecHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Snapshot: cpup01.Snapshot{Environment: cpup01.EnvironmentBindingSnapshot{NamespaceName: "test-ns", NamespaceUID: "namespace-1"}}}, OwnerRevision: 9, States: biz.ExecutionStates{Close: biz.CloseStateClosed, Delivery: biz.DeliveryStatePublished}}, Runtime: biz.ExecutionRuntime{OwnerRevision: 9, ClosedAt: &closed, CloseGeneration: 1, CloseEvidence: &biz.ManagedCloseEvidence{ObservedAt: closed}, Training: &biz.TrainingPlan{}, TrainingHandle: &biz.TrainingHandle{NamespaceUID: "namespace-1", TrainJobUID: "original-train"}, Publication: &biz.RuntimePublication{ID: "published-1"}, Observation: &biz.TrainingRuntimeObservation{WritersAbsent: true, Resources: append(parents, target)}}}}
			deleted, calls := false, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet && (r.URL.Path == "/apis/jobset.x-k8s.io/v1alpha2/namespaces/test-ns/jobsets/training" || r.URL.Path == "/apis/trainer.kubeflow.org/v1alpha1/namespaces/test-ns/trainjobs/training") {
					w.WriteHeader(404)
					_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "NotFound", "code": 404})
					return
				}
				if r.URL.Path != "/apis/batch/v1/namespaces/test-ns/jobs/training-node" {
					t.Error("delete escaped exact execution target")
					w.WriteHeader(403)
					return
				}
				switch r.Method {
				case http.MethodGet:
					if deleted {
						w.WriteHeader(404)
						_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "NotFound", "code": 404})
						return
					}
					uid := "original-job"
					if changed {
						uid = "replacement-job"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]any{"namespace": "test-ns", "name": "training-node", "uid": uid, "resourceVersion": "9", "ownerReferences": []any{map[string]any{"apiVersion": "jobset.x-k8s.io/v1alpha2", "kind": "JobSet", "name": "training", "uid": "original-set", "controller": true}}}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Complete", "status": "True"}}}})
				case http.MethodDelete:
					calls++
					var options metav1.DeleteOptions
					if json.NewDecoder(r.Body).Decode(&options) != nil || options.Preconditions == nil || options.Preconditions.UID == nil || string(*options.Preconditions.UID) != "original-job" || options.Preconditions.ResourceVersion == nil || *options.Preconditions.ResourceVersion != "9" || options.PropagationPolicy == nil || *options.PropagationPolicy != metav1.DeletePropagationOrphan {
						t.Error("delete lacks literal original UID/version/Orphan preconditions")
					}
					deleted = true
					_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "Status", "status": "Success", "code": 200})
				default:
					t.Error("unexpected cleanup mutation")
					w.WriteHeader(405)
				}
			}))
			defer server.Close()
			kube, err := dynamic.NewForConfig(&rest.Config{Host: server.URL, Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			err = cleanup.New(kube, nil).DeleteExecutionResource(context.Background(), record, target)
			if changed {
				if !errors.Is(err, biz.ErrCleanupConflict) || calls != 0 {
					t.Fatal("replacement UID was deleted", err)
				}
			} else if err != nil || calls != 1 || !deleted {
				t.Fatalf("CLEANUP_DELETE_NOT_IMPLEMENTED: exact target not disposed with retained evidence: calls=%d %v", calls, err)
			}
		})
	}
}
