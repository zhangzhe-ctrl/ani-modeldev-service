//go:build cpu_mainflow

package submittest_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestMainFlowEarlyDeadlineRetainsReviewUntilEveryOriginalWriterExitIsDurable(t *testing.T) {
	f := newCompleteFixture(t)
	deadline := time.Now().UTC().Truncate(time.Microsecond).Add(3 * time.Second)
	f.request.Admission.Snapshot.DeadlineAt = deadline
	var err error
	f.request.Admission.SpecHash, err = f.request.Admission.Snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	var visible atomic.Bool
	// Only external KFP/Kubernetes records are substituted. At the deadline,
	// the confirmed Run is temporarily unreadable and one actual sidecar keeps
	// running after termination is acknowledged. No Begin has bound authority.
	f.kfp = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !visible.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
			return
		}
		f.kfpRequest(w, r)
		if r.Method == http.MethodPost && r.URL.Path == "/apis/v2beta1/runs/"+completeRunID+":terminate" {
			f.mu.Lock()
			defer f.mu.Unlock()
			pod := f.objects["/api/v1/namespaces/"+f.workspace.NamespaceName+"/pods/main-prepare"]
			pod["metadata"].(map[string]any)["deletionTimestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
			pod["metadata"].(map[string]any)["finalizers"] = []any{"modeldev.ani.io/step-exit-evidence"}
			pod["spec"].(map[string]any)["containers"] = []any{map[string]any{"name": "main"}, map[string]any{"name": "sidecar"}}
			status := pod["status"].(map[string]any)
			status["containerStatuses"] = append(status["containerStatuses"].([]any), map[string]any{"name": "sidecar", "state": map[string]any{"running": map[string]any{"startedAt": time.Now().UTC().Format(time.RFC3339Nano)}}})
		}
	}))
	t.Cleanup(f.kfp.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	open := postgres.Prepare(t)
	pool := open()
	_, client, facts, _, _ := bootstrapFixtureWithPool(t, f, pool)
	if _, err := submission.New(pool).GetRunAuthority(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID); !errors.Is(err, biz.ErrExecutionNotFound) {
		t.Fatal("early-deadline preflight already has normal step authority", err)
	}
	select {
	case <-time.After(time.Until(deadline) + 10*time.Millisecond):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	worker, _ := assembleRecoveryOwner(t, f, pool)
	batch, err := worker.ReconcileOnce(ctx)
	unknown, readErr := facts.GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || readErr != nil || batch.Unresolved != 1 || unknown.CloseGeneration != 1 || unknown.CloseReason != "DEADLINE" || unknown.CloseReviewReason != "KFP_CREATE_UNRESOLVED" || unknown.CloseAuthority != nil || unknown.ClosedAt != nil {
		t.Fatalf("original unknown Run must retain its deadline fence and review: %+v %+v %v %v", batch, unknown, err, readErr)
	}
	visible.Store(true)
	pool.Close()
	pool = open()
	worker, _ = assembleRecoveryOwner(t, f, pool)
	batch, err = worker.ReconcileOnce(ctx)
	closing, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || readErr != nil || batch.Unresolved != 1 || closing.CloseGeneration != unknown.CloseGeneration || closing.CloseAuthority == nil || closing.CloseAuthority.RunID != completeRunID || closing.CloseReviewReason != unknown.CloseReviewReason || closing.ClosedAt != nil || closing.CloseEvidence != nil {
		t.Fatalf("EARLY_DEADLINE_REVIEW_LOST: recovering the original owner cleared review before the retained sidecar exited: %+v %+v %v %v", batch, closing, err, readErr)
	}
	// Exact recovery replay must retain its review and revision; it cannot
	// replace the original send permission or advance the aggregate repeatedly.
	if replay, err := lifecycle.New(pool).RecordClosingRun(ctx, *closing.CloseAuthority); err != nil || replay.OwnerRevision != closing.OwnerRevision || replay.CloseReviewReason != closing.CloseReviewReason {
		t.Fatalf("owner recovery replay changed the unresolved execution: %+v %v", replay, err)
	}
	f.mu.Lock()
	pod := f.objects["/api/v1/namespaces/"+f.workspace.NamespaceName+"/pods/main-prepare"]
	statuses := pod["status"].(map[string]any)["containerStatuses"].([]any)
	statuses[1].(map[string]any)["state"] = map[string]any{"terminated": map[string]any{"exitCode": 143, "finishedAt": time.Now().UTC().Format(time.RFC3339Nano)}}
	f.mu.Unlock()
	batch, err = worker.ReconcileOnce(ctx)
	closed, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || readErr != nil || batch.Closed != 1 || closed.ClosedAt == nil || closed.CloseGeneration != unknown.CloseGeneration || closed.CloseReason != "DEADLINE" || closed.CloseReviewReason != "" || closed.CloseEvidence == nil || len(closed.CloseEvidence.Resources) != 5 || closed.Workspace != nil || closed.Training != nil || closed.Publication != nil {
		t.Fatalf("retained actual writer exits did not durably close the original unbound execution: %+v %+v %v %v", batch, closed, err, readErr)
	}
	if _, err := submission.New(pool).GetRunAuthority(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID); !errors.Is(err, biz.ErrExecutionNotFound) {
		t.Fatal("owner recovery fabricated a normal step authority", err)
	}
	if _, err := client.EnsureTraining(bootstrapCall(ctx, f, "synthetic-bound-train-wait"), &modeldevv1.EnsureTrainingRequest{Context: f.stepContext("train-wait")}); err == nil {
		t.Fatal("early deadline granted a late training permit")
	}
	f.mu.Lock()
	runCreates, trainingCreates, stops := f.runCreates, f.creates, f.runStops
	f.mu.Unlock()
	if runCreates != 1 || trainingCreates != 0 || stops != 1 {
		t.Fatalf("early deadline recreated or lost the original work: runs=%d training=%d stops=%d", runCreates, trainingCreates, stops)
	}
}
