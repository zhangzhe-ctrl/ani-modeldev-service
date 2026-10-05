//go:build cpu_mainflow

package submittest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/component"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Real admission/authority/PG/mTLS/prepare/owner code; only external Kubernetes,
// KFP and storage are the existing explicit TLS boundary substitutes.
func TestMainFlowTrainingRejectionPersistsTerminalFailureAndClosesWithoutRecreate(t *testing.T) {
	for _, test := range []struct {
		name                         string
		code                         int
		stepClose, conflict, unknown bool
	}{
		{name: "Forbidden owner close", code: 403},
		{name: "Bad request owner close", code: 400},
		{name: "Forbidden step close", code: 403, stepClose: true},
		{name: "Unknown unavailable stays unresolved", code: 503, unknown: true},
		{name: "Rejected creation with conflicting current object", code: 403, conflict: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			code := test.code
			f := newCompleteFixture(t)
			var conflict atomic.Bool
			trainPath := "/apis/trainer.kubeflow.org/v1alpha1/namespaces/" + f.workspace.NamespaceName + "/trainjobs"
			f.kube = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == trainPath {
					f.mu.Lock()
					f.creates++
					f.mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(code)
					reason := "Forbidden"
					if code == 400 {
						reason = "BadRequest"
					}
					if code == 503 {
						reason = "ServiceUnavailable"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": reason, "code": code, "message": "untrusted-peer-secret-must-not-persist"})
					return
				}
				if r.Method == http.MethodGet && r.URL.Path == trainPath+"/md-"+f.request.Admission.ExecutionID && conflict.Load() {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "trainer.kubeflow.org/v1alpha1", "kind": "TrainJob", "metadata": map[string]any{"name": "md-" + f.request.Admission.ExecutionID, "namespace": f.workspace.NamespaceName, "uid": "conflicting-current-object"}})
					return
				}
				f.kubeRequest(w, r)
			}))
			t.Cleanup(f.kube.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			open := postgres.Prepare(t)
			pool := open()
			_, client, facts, kube, store := bootstrapFixtureWithPool(t, f, pool)
			if _, err := client.BeginExecution(bootstrapCall(ctx, f, "synthetic-bound-prepare"), &modeldevv1.BeginExecutionRequest{Context: f.stepContext("prepare")}); err != nil {
				t.Fatal(err)
			}
			token := filepath.Join(t.TempDir(), "projected-token")
			if err := os.WriteFile(token, []byte("synthetic-bound-prepare"), 0600); err != nil {
				t.Fatal(err)
			}
			prepare, err := component.New(component.Config{TenantID: f.request.Admission.TenantID, Context: f.stepContext("prepare"), TokenFile: token, WorkspaceDirectory: f.root, PVCName: f.workspace.PVCName}, client, kube, store)
			if err != nil {
				t.Fatal(err)
			}
			if err := prepare.Run(ctx, "prepare"); err != nil {
				t.Fatal(err)
			}
			f.finishStep("prepare")
			_, ensureErr := client.EnsureTraining(bootstrapCall(ctx, f, "synthetic-bound-train-wait"), &modeldevv1.EnsureTrainingRequest{Context: f.stepContext("train-wait")})
			if test.unknown {
				if status.Code(ensureErr) != codes.Unavailable {
					t.Fatal("unknown outcome became a definite refusal", ensureErr)
				}
				before, err := facts.GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
				if err != nil || before.Training == nil || before.TrainingRejection != nil {
					t.Fatal("unknown outcome lost consumed intent or fabricated refusal", err)
				}
				for range 2 {
					if _, err := client.EnsureTraining(bootstrapCall(ctx, f, "synthetic-bound-train-wait"), &modeldevv1.EnsureTrainingRequest{Context: f.stepContext("train-wait")}); status.Code(err) != codes.FailedPrecondition {
						t.Fatal("unknown retry bypassed deterministic lookup", err)
					}
				}
				applyOwnerStop(t, ctx, f, pool)
				worker, _ := assembleRecoveryOwner(t, f, pool)
				batch, err := worker.ReconcileOnce(ctx)
				state, readErr := facts.GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
				if err != nil || readErr != nil || batch.Unresolved != 1 || state.ClosedAt != nil || state.CloseReviewReason != "TRAINJOB_CREATE_UNRESOLVED" || state.TrainingRejection != nil || f.creates != 1 {
					t.Fatal("unknown 503/404 closed or reissued original POST", batch, err, readErr)
				}
				return
			}
			if status.Code(ensureErr) != codes.Aborted {
				t.Fatalf("TRAINING_REJECTION_NOT_IMPLEMENTED: complete %d must be terminal Aborted, got %v", code, status.Code(ensureErr))
			}
			failure := status.Convert(ensureErr).Details()
			if len(failure) != 1 {
				t.Fatal("terminal refusal lost its public error detail")
			}
			detail, ok := failure[0].(*modeldevv1.ErrorDetail)
			if !ok || detail.Reason != modeldevv1.ErrorReason_ERROR_REASON_TRAINING_FAILED || strings.Contains(detail.SafeMessage, "untrusted-peer-secret") {
				t.Fatal("terminal refusal lost TRAINING_FAILED or exposed peer message")
			}
			original, err := facts.GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
			if err != nil || original.Training == nil || original.TrainingHandle != nil {
				t.Fatal("rejection lost immutable consumed intent", err)
			}
			raw, _ := json.Marshal(original)
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			rejection, ok := fields["TrainingRejection"].(map[string]any)
			if !ok || rejection["RequestSHA256"] != original.Training.RequestSHA256 || rejection["NamespaceUID"] != f.workspace.NamespaceUID || rejection["StatusCode"] != float64(code) || rejection["ObservedAt"] == nil || strings.Contains(string(raw), "untrusted-peer-secret") {
				t.Fatalf("TRAINING_REJECTION_NOT_IMPLEMENTED: rejection receipt missing exact request/namespace/status/time or leaks peer body")
			}
			pool.Close()
			pool = open()
			worker, runs := assembleRecoveryOwner(t, f, pool)
			client = newFixtureRuntimeClient(t, f, pool, kube, store, runs)
			for _, call := range []func() error{
				func() error {
					_, err := client.EnsureTraining(bootstrapCall(ctx, f, "synthetic-bound-train-wait"), &modeldevv1.EnsureTrainingRequest{Context: f.stepContext("train-wait")})
					return err
				},
				func() error {
					_, err := client.GetTrainingStatus(bootstrapCall(ctx, f, "synthetic-bound-train-wait"), &modeldevv1.GetTrainingStatusRequest{Context: f.stepContext("train-wait")})
					return err
				},
			} {
				if status.Code(call()) != codes.Aborted {
					t.Fatal("restarted owner retried a rejected intent")
				}
			}
			if err := os.WriteFile(token, []byte("synthetic-bound-train-wait"), 0600); err != nil {
				t.Fatal(err)
			}
			waiter, err := component.New(component.Config{TenantID: f.request.Admission.TenantID, Context: f.stepContext("train-wait"), TokenFile: token, PollInterval: time.Millisecond}, client, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			waitCtx, cancelWait := context.WithTimeout(ctx, 2*time.Second)
			waitErr := waiter.Run(waitCtx, "train-wait")
			cancelWait()
			if status.Code(waitErr) != codes.Aborted {
				t.Fatal("real component continued polling a terminal refusal", waitErr)
			}
			readback, err := execution.New(pool).Get(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
			if err != nil || readback.States.Compute != biz.ComputeStateFailed {
				t.Fatal("durable rejection not projected as FAILED", err)
			}
			if test.stepClose {
				f.mu.Lock()
				f.objects["/api/v1/namespaces/"+f.workspace.NamespaceName+"/pods/main-train-wait"]["status"] = map[string]any{"phase": "Failed", "containerStatuses": []any{map[string]any{"name": "main", "restartCount": 0, "state": map[string]any{"terminated": map[string]any{"exitCode": 1, "finishedAt": time.Now().UTC().Format(time.RFC3339)}}}}}
				f.skipped = map[string]bool{"collect": true, "publish": true}
				for step := range f.skipped {
					delete(f.objects, "/api/v1/namespaces/"+f.workspace.NamespaceName+"/pods/main-"+step)
				}
				f.mu.Unlock()
				closeRequest := &modeldevv1.RequestExecutionCloseRequest{Context: f.stepContext("close"), Reason: modeldevv1.CloseReason_CLOSE_REASON_STEP_FAILED}
				for range 2 {
					receipt, err := client.RequestExecutionClose(bootstrapCall(ctx, f, "synthetic-bound-close"), closeRequest)
					if err != nil || receipt.GetCloseState() != modeldevv1.CloseState_CLOSE_STATE_CLOSED {
						t.Fatal("authenticated step close remained unresolved", err)
					}
				}
				closed, err := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
				if err != nil || closed.ClosedAt == nil || closed.CloseReason != "STEP_FAILED" || closed.CloseEvidence == nil || len(closed.CloseEvidence.SkippedTasks) != 2 || closed.TrainingRejection == nil || f.creates != 1 {
					t.Fatal("step close lost original rejection or skipped full writer proof", err)
				}
				return
			}
			conflict.Store(test.conflict)
			generation := applyOwnerStop(t, ctx, f, pool)
			batch, err := worker.ReconcileOnce(ctx)
			closed, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
			if test.conflict {
				if err != nil || readErr != nil || batch.Unresolved != 1 || closed.ClosedAt != nil || closed.CloseReviewReason != "TRAINJOB_CREATE_UNRESOLVED" || closed.TrainingRejection == nil || closed.TrainingHandle != nil || f.creates != 1 {
					t.Fatal("a refusal hid a conflicting current writer", batch, err, readErr)
				}
				return
			}
			if err != nil || readErr != nil || batch.Closed != 1 || closed.ClosedAt == nil || closed.CloseGeneration != generation || closed.Training == nil || closed.Training.RequestSHA256 != original.Training.RequestSHA256 || closed.TrainingHandle != nil || closed.CloseEvidence == nil || closed.CloseReviewReason != "" {
				t.Fatalf("TRAINING_REJECTION_NOT_IMPLEMENTED: trusted rejection did not close under real fence and full writer proof: %+v %v %v", batch, err, readErr)
			}
			f.mu.Lock()
			creates, runCreates, runStops := f.creates, f.runCreates, f.runStops
			f.mu.Unlock()
			if creates != 1 || runCreates != 1 || runStops != 1 {
				t.Fatalf("recreated or missed original KFP stop: %d/%d/%d", creates, runCreates, runStops)
			}
			t.Log("TRAINING_REJECTION: complete API rejection survives reconnect, terminal FAILED and original KFP fenced/writer-free CLOSED; no TrainJob recreate")
		})
	}
}
