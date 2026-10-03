package kfp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func TestManagedRunRequiresFrozenRunAndKFPTaskPod(t *testing.T) {
	input := fixtureCreateRequest(t, fixtureAdmission(t))
	association := biz.ManagedStepAssociation{RunID: "55555555-6666-4777-8888-999999999999", PodName: "managed-prepare"}
	// Handwritten against the fixed KFP 2.16.0 Run API. The task association
	// comes from KFP, not an annotation supplied by the callback.
	valid := `{"run_id":"55555555-6666-4777-8888-999999999999","experiment_id":"44444444-4444-4444-8444-444444444444","display_name":"md-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee","pipeline_version_reference":{"pipeline_id":"cccccccc-cccc-4ccc-8ccc-cccccccccccc","pipeline_version_id":"dddddddd-dddd-4ddd-8ddd-dddddddddddd"},"runtime_config":{"parameters":{"execution_id":"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee","spec_hash":"972dee14e65202d5d4da7da199cf5f37139b535701a0cb1b3de4d8ef8a5170b9"},"pipeline_root":"s3://fixture-kfp-artifacts/managed-root"},"service_account":"cpu-managed-step","run_details":{"task_details":[{"run_id":"55555555-6666-4777-8888-999999999999","display_name":"prepare","pod_name":"managed-prepare"}]}}`
	var body atomic.Value
	body.Store(valid)
	var calls atomic.Int32
	peer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/apis/v2beta1/runs/"+association.RunID || r.Header.Get("Authorization") != "Bearer synthetic-fixture-token" {
			t.Error("managed Run verification did not use the authenticated fixed GetRun endpoint")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	t.Cleanup(peer.Close)
	client := fixtureClient(t, peer, tokenProviderFunc(func(_ context.Context, tenant string, env cpup01.EnvironmentBindingSnapshot) (string, error) {
		if tenant != input.Plan.TenantID || env != input.Plan.Environment {
			t.Error("wrong frozen identity")
		}
		return "synthetic-fixture-token", nil
	}))
	if err := client.VerifyManagedRun(context.Background(), input.Plan, association, "prepare"); err != nil {
		t.Fatalf("valid Run and KFP task association rejected: %v", err)
	}
	for _, invalid := range []string{
		strings.Replace(valid, `"pod_name":"managed-prepare"`, `"pod_name":"unrelated-pod"`, 1),
		strings.Replace(valid, `"display_name":"prepare"`, `"display_name":"train-wait"`, 1),
		strings.Replace(valid, input.Plan.SpecHash, strings.Repeat("f", 64), 1),
		strings.Replace(valid, `"service_account":"cpu-managed-step"`, `"service_account":"ordinary-trainer"`, 1),
		strings.TrimSuffix(valid, "}") + `,"run_details":{"task_details":[]}}`,
	} {
		body.Store(invalid)
		if err := client.VerifyManagedRun(context.Background(), input.Plan, association, "prepare"); err == nil {
			t.Error("unproven Run/task association accepted")
		}
	}
	if calls.Load() != 6 {
		t.Errorf("expected one GET for each verification, got %d", calls.Load())
	}
}
