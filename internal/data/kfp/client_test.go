package kfp_test

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/kfp"
)

func TestCreateRunResponseLostPreservesUncertaintyWithoutResending(t *testing.T) {
	// This TLS server substitutes only the KFP HTTP boundary. It is not evidence
	// of a real Run, durable SUBMITTING, tenant authorization, or ENV readiness.
	admission := fixtureAdmission(t)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/apis/v2beta1/runs" || r.URL.RawQuery != "" {
			t.Errorf("unexpected KFP operation: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-fixture-token" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("request lacks regenerated fixture identity or JSON content type")
		}
		for _, key := range []string{"Idempotency-Key", "X-Idempotency-Key", "Cookie", "Kubeflow-Userid"} {
			if len(r.Header.Values(key)) != 0 {
				t.Errorf("unexpected forwarded or retry-enabling header %s", key)
			}
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read fixture request: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		// Handwritten against KFP 2.16.0 run.proto / runtime_config.proto.
		// No source implementation or encoder is used to form the expectation.
		var want map[string]any
		if err := json.Unmarshal([]byte(`{
  "experiment_id":"44444444-4444-4444-8444-444444444444",
  "display_name":"md-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
  "pipeline_version_reference":{
    "pipeline_id":"cccccccc-cccc-4ccc-8ccc-cccccccccccc",
    "pipeline_version_id":"dddddddd-dddd-4ddd-8ddd-dddddddddddd"
  },
  "runtime_config":{
    "parameters":{
      "execution_id":"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
      "spec_hash":"972dee14e65202d5d4da7da199cf5f37139b535701a0cb1b3de4d8ef8a5170b9"
    },
    "pipeline_root":"s3://fixture-kfp-artifacts/managed-root"
  },
  "service_account":"cpu-managed-step"
}`), &want); err != nil {
			t.Errorf("invalid handwritten fixture: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Error("request does not match the fixed owner-controlled CreateRun contract")
		}
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("disconnect response fixture: %v", err)
			return
		}
		_ = connection.Close()
	}))
	t.Cleanup(server.Close)
	certificates := x509.NewCertPool()
	certificates.AddCert(server.Certificate())
	var tokenCalls atomic.Int32
	provider := tokenProviderFunc(func(_ context.Context, tenant string, binding cpup01.EnvironmentBindingSnapshot) (string, error) {
		tokenCalls.Add(1)
		if tenant != admission.TenantID || binding != admission.Snapshot.Environment {
			t.Error("identity provider did not receive the frozen trusted tenant binding")
		}
		return "synthetic-fixture-token", nil
	})
	client, err := kfp.New(kfp.Config{
		ConnectionRef: admission.Snapshot.Environment.KFPConnectionRef,
		Endpoint: server.URL,
		PipelineRoot: "s3://fixture-kfp-artifacts/managed-root",
		RootCAs: certificates,
		Timeout: 3*time.Second,
	}, provider)
	if err != nil {
		t.Fatalf("construct candidate client: %v", err)
	}
	observation, err := client.CreateRun(context.Background(), admission)
	if err == nil || observation.State != biz.PipelineSubmissionUncertain || observation.RunID != "" {
		t.Errorf("lost creation response must remain uncertain with no confirmed Run: %+v, %v", observation, err)
	}
	if calls.Load() != 1 || tokenCalls.Load() != 1 {
		t.Errorf("want exactly one credential generation and POST; got %d credentials and %d requests", tokenCalls.Load(), calls.Load())
	}
}

type tokenProviderFunc func(context.Context, string, cpup01.EnvironmentBindingSnapshot) (string, error)

func (provider tokenProviderFunc) BearerToken(ctx context.Context, tenant string, binding cpup01.EnvironmentBindingSnapshot) (string, error) {
	return provider(ctx, tenant, binding)
}

func fixtureAdmission(t *testing.T) biz.Admission {
	t.Helper()
	snapshot := conformance.SnapshotV1()
	intent := cpup01.Intent{Name:"kfp-boundary-fixture", Kind:"GENERAL_TRAINING", PresetID:snapshot.Release.PresetID, DatasetVersionID:snapshot.Input.InputVersionID}
	_, intentHash, err := cpup01.CanonicalIntent(intent)
	if err != nil {
		t.Fatalf("fixture intent: %v", err)
	}
	return biz.Admission{
		TenantID:"11111111-2222-4333-8444-555555555555",
		Actor:"governance:synthetic-fixture",
		OperationID:"bbbbbbbb-cccc-4ddd-8eee-ffffffffffff",
		ExecutionID:"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		Intent:intent, IntentHash:intentHash,
		Snapshot:snapshot, SpecHash:conformance.SnapshotSHA256V1,
		AcceptedAt:snapshot.DeadlineAt.Add(-time.Hour).UTC(),
	}
}
