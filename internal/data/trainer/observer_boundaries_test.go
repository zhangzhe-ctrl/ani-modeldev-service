package trainer_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/trainer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func TestObserveRejectsInvalidBindingBeforeRequest(t *testing.T) {
	cases := []struct {
		name   string
		change func(*biz.TrainJobBinding)
	}{
		{"invalid tenant", func(b *biz.TrainJobBinding) { b.TenantID = "not-a-tenant" }},
		{"nil tenant", func(b *biz.TrainJobBinding) { b.TenantID = "00000000-0000-0000-0000-000000000000" }},
		{"invalid execution", func(b *biz.TrainJobBinding) { b.ExecutionID = "not-an-execution" }},
		{"short spec digest", func(b *biz.TrainJobBinding) { b.SpecSHA256 = "aabb" }},
		{"nonhex spec digest", func(b *biz.TrainJobBinding) { b.SpecSHA256 = "z" + b.SpecSHA256[1:] }},
		{"invalid namespace", func(b *biz.TrainJobBinding) { b.Namespace = "../other" }},
		{"invalid resource name", func(b *biz.TrainJobBinding) { b.Name = "other/resource" }},
		{"missing namespace UID", func(b *biz.TrainJobBinding) { b.NamespaceUID = "" }},
		{"missing TrainJob UID", func(b *biz.TrainJobBinding) { b.UID = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newObservationFixture()
			adapter, requests := observationAPI(t, fixture)
			binding := observationBinding()
			tc.change(&binding)
			got, err := adapter.ObserveTrainJob(context.Background(), binding)
			if !errors.Is(err, trainer.ErrInvalidBinding) || got != (biz.TrainJobObservation{}) {
				t.Fatalf("invalid binding must return no usable observation: got=%+v err=%v", got, err)
			}
			if requests.namespace.Load() != 0 || requests.job.Load() != 0 {
				t.Fatalf("invalid binding reached Kubernetes: namespace=%d job=%d", requests.namespace.Load(), requests.job.Load())
			}
		})
	}
}

func TestObserveRejectsForeignOrRecreatedResources(t *testing.T) {
	cases := []struct {
		name        string
		change      func(*observationFixture)
		jobRequests int32
	}{
		{"recreated namespace", func(f *observationFixture) { f.namespace["metadata"].(map[string]any)["uid"] = "another-namespace-uid" }, 0},
		{"wrong namespace name", func(f *observationFixture) { f.namespace["metadata"].(map[string]any)["name"] = "another-namespace" }, 0},
		{"wrong namespace kind", func(f *observationFixture) { f.namespace["kind"] = "Pod" }, 0},
		{"recreated TrainJob", func(f *observationFixture) { f.job["metadata"].(map[string]any)["uid"] = "another-job-uid" }, 1},
		{"foreign TrainJob namespace", func(f *observationFixture) { f.job["metadata"].(map[string]any)["namespace"] = "another-namespace" }, 1},
		{"foreign TrainJob name", func(f *observationFixture) { f.job["metadata"].(map[string]any)["name"] = "another-job" }, 1},
		{"wrong TrainJob kind", func(f *observationFixture) { f.job["kind"] = "JobSet" }, 1},
		{"wrong TrainJob API", func(f *observationFixture) { f.job["apiVersion"] = "trainer.kubeflow.org/v9" }, 1},
		{"foreign tenant", func(f *observationFixture) {
			f.annotations()["modeldev.ani.io/tenant-id"] = "33333333-3333-4333-8333-333333333333"
		}, 1},
		{"foreign execution", func(f *observationFixture) {
			f.annotations()["modeldev.ani.io/execution-id"] = "33333333-3333-4333-8333-333333333333"
		}, 1},
		{"different frozen spec", func(f *observationFixture) {
			f.annotations()["modeldev.ani.io/execution-spec-sha256"] = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}, 1},
		{"missing ownership", func(f *observationFixture) { delete(f.job["metadata"].(map[string]any), "annotations") }, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newObservationFixture()
			tc.change(fixture)
			adapter, requests := observationAPI(t, fixture)
			got, err := adapter.ObserveTrainJob(context.Background(), observationBinding())
			if !errors.Is(err, trainer.ErrBindingMismatch) || got != (biz.TrainJobObservation{}) {
				t.Fatalf("unbound resource must not report its Complete=True: got=%+v err=%v", got, err)
			}
			if requests.namespace.Load() != 1 || requests.job.Load() != tc.jobRequests {
				t.Fatalf("unexpected read boundary: namespace=%d job=%d, want 1/%d", requests.namespace.Load(), requests.job.Load(), tc.jobRequests)
			}
		})
	}
}

func TestObserveKeepsUnavailableConditionsUnknown(t *testing.T) {
	cases := []struct {
		name       string
		conditions []any
	}{
		{"missing status", nil},
		{"empty conditions", []any{}},
		{"explicit unknown", []any{observationCondition("Complete", "Unknown", 7)}},
		{"stale completion", []any{observationCondition("Complete", "True", 6)}},
		{"future completion", []any{observationCondition("Complete", "True", 8)}},
		{"unversioned completion", []any{map[string]any{"type": "Complete", "status": "True", "reason": "Unversioned", "lastTransitionTime": "2026-09-30T09:00:00Z"}}},
		{"unrelated true condition", []any{observationCondition("Created", "True", 7)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newObservationFixture()
			if tc.conditions == nil {
				delete(fixture.job, "status")
			} else {
				fixture.job["status"] = map[string]any{"conditions": tc.conditions}
			}
			adapter, _ := observationAPI(t, fixture)
			got, err := adapter.ObserveTrainJob(context.Background(), observationBinding())
			if err != nil {
				t.Fatalf("read bound resource with unavailable conditions: %v", err)
			}
			if got.Suspended != biz.TrainingConditionUnknown || got.Complete != biz.TrainingConditionUnknown || got.Failed != biz.TrainingConditionUnknown {
				t.Fatalf("unavailable current condition became a confirmed state: %+v", got)
			}
			binding := observationBinding()
			if got.TrainJobUID != binding.UID || got.NamespaceUID != binding.NamespaceUID || got.Generation != 7 || got.ObservedAt.IsZero() {
				t.Fatalf("unknown conditions lost observed resource identity: %+v", got)
			}
		})
	}
}

func TestObserveRejectsAmbiguousConditions(t *testing.T) {
	cases := []struct {
		name   string
		change func(*observationFixture)
	}{
		{"missing generation", func(f *observationFixture) { delete(f.job["metadata"].(map[string]any), "generation") }},
		{"invalid status value", func(f *observationFixture) { f.conditions([]any{observationCondition("Complete", "Succeeded", 7)}) }},
		{"malformed status type", func(f *observationFixture) {
			c := observationCondition("Complete", "True", 7)
			c["status"] = 1
			f.conditions([]any{c})
		}},
		{"conditions not list", func(f *observationFixture) { f.job["status"] = map[string]any{"conditions": "Complete"} }},
		{"condition not object", func(f *observationFixture) { f.conditions([]any{"Complete"}) }},
		{"duplicate completion", func(f *observationFixture) {
			f.conditions([]any{observationCondition("Complete", "True", 7), observationCondition("Complete", "False", 7)})
		}},
		{"conflicting terminal states", func(f *observationFixture) {
			f.conditions([]any{observationCondition("Complete", "True", 7), observationCondition("Failed", "True", 7)})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newObservationFixture()
			tc.change(fixture)
			adapter, _ := observationAPI(t, fixture)
			got, err := adapter.ObserveTrainJob(context.Background(), observationBinding())
			if !errors.Is(err, trainer.ErrInvalidObservation) || got != (biz.TrainJobObservation{}) {
				t.Fatalf("ambiguous controller state must not yield success: got=%+v err=%v", got, err)
			}
		})
	}
}

func TestObservePreservesAPIReadErrors(t *testing.T) {
	cases := []struct {
		name            string
		namespaceStatus int
		jobStatus       int
		isError         func(error) bool
		jobRequests     int32
	}{
		{"namespace absent", http.StatusNotFound, 0, apierrors.IsNotFound, 0},
		{"TrainJob absent", 0, http.StatusNotFound, apierrors.IsNotFound, 1},
		{"TrainJob forbidden", 0, http.StatusForbidden, apierrors.IsForbidden, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newObservationFixture()
			fixture.namespaceStatus = tc.namespaceStatus
			fixture.jobStatus = tc.jobStatus
			adapter, requests := observationAPI(t, fixture)
			got, err := adapter.ObserveTrainJob(context.Background(), observationBinding())
			if !tc.isError(err) || got != (biz.TrainJobObservation{}) {
				t.Fatalf("API error became an observation or lost its error classification: got=%+v err=%v", got, err)
			}
			if requests.namespace.Load() != 1 || requests.job.Load() != tc.jobRequests {
				t.Fatalf("unexpected reads following API error: namespace=%d job=%d", requests.namespace.Load(), requests.job.Load())
			}
		})
	}
}

type observationFixture struct {
	namespace       map[string]any
	job             map[string]any
	namespaceStatus int
	jobStatus       int
}

type observationRequests struct {
	namespace atomic.Int32
	job       atomic.Int32
}

func observationBinding() biz.TrainJobBinding {
	return biz.TrainJobBinding{
		TenantID:     "11111111-1111-4111-8111-111111111111",
		ExecutionID:  "22222222-2222-4222-8222-222222222222",
		SpecSHA256:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Namespace:    "cpu-execution",
		NamespaceUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		Name:         "md-22222222-2222-4222-8222-222222222222",
		UID:          "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
	}
}

func observationCondition(kind, status string, generation int) map[string]any {
	return map[string]any{"type": kind, "status": status, "observedGeneration": generation, "reason": "FixtureCondition", "lastTransitionTime": "2026-09-30T09:00:00Z"}
}

func newObservationFixture() *observationFixture {
	binding := observationBinding()
	f := &observationFixture{
		namespace: map[string]any{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": map[string]any{"name": binding.Namespace, "uid": binding.NamespaceUID},
		},
		job: map[string]any{
			"apiVersion": "trainer.kubeflow.org/v1alpha1", "kind": "TrainJob",
			"metadata": map[string]any{
				"name": binding.Name, "namespace": binding.Namespace, "uid": binding.UID, "generation": 7,
				"annotations": map[string]any{
					"modeldev.ani.io/tenant-id":             binding.TenantID,
					"modeldev.ani.io/execution-id":          binding.ExecutionID,
					"modeldev.ani.io/execution-spec-sha256": binding.SpecSHA256,
				},
			},
		},
	}
	f.conditions([]any{observationCondition("Suspended", "False", 7), observationCondition("Complete", "True", 7), observationCondition("Failed", "False", 7)})
	return f
}

func (f *observationFixture) annotations() map[string]any {
	return f.job["metadata"].(map[string]any)["annotations"].(map[string]any)
}

func (f *observationFixture) conditions(conditions []any) {
	f.job["status"] = map[string]any{"conditions": conditions}
}

func observationAPI(t *testing.T, fixture *observationFixture) (*trainer.Adapter, *observationRequests) {
	t.Helper()
	requests := &observationRequests{}
	// HTTP is the external API seam. No Kubernetes fake client or private
	// adapter helper replaces identity checks or condition interpretation.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			t.Errorf("read-only observer issued %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var object map[string]any
		var status int
		switch r.URL.Path {
		case "/api/v1/namespaces/cpu-execution":
			requests.namespace.Add(1)
			object, status = fixture.namespace, fixture.namespaceStatus
		case "/apis/trainer.kubeflow.org/v1alpha1/namespaces/cpu-execution/trainjobs/md-22222222-2222-4222-8222-222222222222":
			requests.job.Add(1)
			object, status = fixture.job, fixture.jobStatus
		default:
			t.Errorf("unexpected API path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if status != 0 {
			reason := "NotFound"
			if status == http.StatusForbidden {
				reason = "Forbidden"
			}
			object = map[string]any{"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": reason, "code": status}
			w.WriteHeader(status)
		}
		if err := json.NewEncoder(w).Encode(object); err != nil {
			t.Errorf("encode API fixture: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("construct official API client: %v", err)
	}
	return trainer.New(client), requests
}
