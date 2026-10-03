//go:build cpu_mainflow

package submittest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/component"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestMainFlowLateTrainingResponseRecoversOriginalWriterAndCloses(t *testing.T) {
	f := newCompleteFixture(t)
	f.request.Admission.Snapshot.Program.ResolvedArgs = append(f.request.Admission.Snapshot.Program.ResolvedArgs, "--recipe", "slow-stop")
	var err error
	f.request.Admission.SpecHash, err = f.request.Admission.Snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	var visible atomic.Bool
	trainPath := "/apis/trainer.kubeflow.org/v1alpha1/namespaces/" + f.workspace.NamespaceName + "/trainjobs"
	f.kube = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == trainPath {
			f.kubeRequest(httptest.NewRecorder(), r)
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"ServiceUnavailable","code":503}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == trainPath+"/md-"+f.request.Admission.ExecutionID && !visible.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"NotFound","code":404}`))
			return
		}
		f.kubeRequest(w, r)
	}))
	t.Cleanup(f.kube.Close)
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
	if err := os.MkdirAll(filepath.Join(f.root, f.workspace.TrainingSubpath), 0700); err != nil {
		t.Fatal(err)
	}
	ensured := make(chan error, 1)
	go func() {
		_, err := client.EnsureTraining(bootstrapCall(ctx, f, "synthetic-bound-train-wait"), &modeldevv1.EnsureTrainingRequest{Context: f.stepContext("train-wait")})
		ensured <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("TrainJob create never reached external boundary")
	}
	generation := applyOwnerStop(t, ctx, f, pool)
	close(release)
	if err := <-ensured; err == nil {
		t.Fatal("lost TrainJob response returned success")
	}
	original, err := facts.GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || original.Training == nil || original.TrainingHandle != nil {
		t.Fatalf("lost creation intent was not durable: %+v %v", original, err)
	}
	awaitOwnerCloseOptimizerStep(t, ctx, f)
	pool.Close()
	pool = open()
	worker, _ := assembleRecoveryOwner(t, f, pool)
	batch, err := worker.ReconcileOnce(ctx)
	unresolved, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || readErr != nil || batch.Unresolved != 1 || unresolved.ClosedAt != nil || unresolved.TrainingHandle != nil {
		t.Fatalf("initial TrainJob NotFound implied closure: %+v %+v %v %v", batch, unresolved, err, readErr)
	}
	raw, _ := json.Marshal(unresolved)
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["CloseReviewReason"] != "TRAINJOB_CREATE_UNRESOLVED" {
		t.Fatalf("LATE_TRAIN_CLOSE_NOT_IMPLEMENTED: missing durable review reason: %s", raw)
	}
	assertQueryCloseState(t, ctx, pool, f.request.Admission, modeldevv1.CloseState_CLOSE_STATE_NEEDS_REVIEW)
	visible.Store(true)
	pool.Close()
	pool = open()
	worker, _ = assembleRecoveryOwner(t, f, pool)
	batch, err = worker.ReconcileOnce(ctx)
	closed, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || readErr != nil || batch.Closed != 1 || closed.ClosedAt == nil || closed.CloseGeneration != generation || closed.Training == nil || closed.Training.RequestSHA256 != original.Training.RequestSHA256 || closed.TrainingHandle == nil || closed.TrainingHandle.TrainJobUID != completeTrainUID || closed.Observation == nil || !closed.Observation.WritersAbsent {
		t.Fatalf("LATE_TRAIN_CLOSE_NOT_IMPLEMENTED: original writer did not close after recovery: %+v %+v %v %v", batch, closed, err, readErr)
	}
	assertQueryCloseState(t, ctx, pool, f.request.Admission, modeldevv1.CloseState_CLOSE_STATE_CLOSED)
	select {
	case <-f.trainingDone:
	default:
		t.Fatal("CLOSED while actual training remained active")
	}
	f.mu.Lock()
	creates, runCreates, trainStops, trainingErr := f.creates, f.runCreates, f.trainStops, f.trainingErr
	f.mu.Unlock()
	if creates != 1 || runCreates != 1 || trainStops != 1 || trainingErr == nil {
		t.Fatalf("recovery lost original training or recreated: train=%d run=%d stops=%d err=%v", creates, runCreates, trainStops, trainingErr)
	}
	t.Log("LATE_TRAIN_CLOSE: stop during CreateTrainJob, actual optimizer step, hidden response and first NotFound remain unresolved; original spec/UID recovered and actual writer stopped without recreation")
}
