//go:build cpu_mainflow

package submittest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestMainFlowDeadlineInitializationAbortPersistsOwnerProofAcrossRecovery(t *testing.T) {
	f := newCompleteFixture(t)
	deadline := time.Now().UTC().Truncate(time.Microsecond).Add(3 * time.Second)
	f.request.Admission.Snapshot.DeadlineAt = deadline
	var err error
	f.request.Admission.SpecHash, err = f.request.Admission.Snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../runtimeproof/testdata/kubelet-1.35.8-pod-initialization-abort.json")
	if err != nil {
		t.Fatal(err)
	}
	var captured map[string]any
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatal(err)
	}
	var visible atomic.Bool
	// Only KFP/Kubernetes are boundary fixtures. The captured Pod has an actual
	// init exit and unstarted suffix/regular containers, never fake main exits.
	f.kfp = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !visible.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		f.kfpRequest(w, r)
		if r.Method == http.MethodPost && r.URL.Path == "/apis/v2beta1/runs/"+completeRunID+":terminate" {
			f.mu.Lock()
			defer f.mu.Unlock()
			pod := f.objects["/api/v1/namespaces/"+f.workspace.NamespaceName+"/pods/main-prepare"]
			metadata := pod["metadata"].(map[string]any)
			for key, value := range captured["metadata"].(map[string]any) {
				metadata[key] = value
			}
			pod["spec"], pod["status"] = captured["spec"], captured["status"]
			pod["spec"].(map[string]any)["serviceAccountName"] = f.request.Admission.Snapshot.Environment.Identities.KFPStepServiceAccount
			// Initially a live sandbox deliberately lacks stopped-writer proof.
			pod["status"].(map[string]any)["conditions"].([]any)[1].(map[string]any)["status"] = "True"
		}
	}))
	t.Cleanup(f.kfp.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	open := postgres.Prepare(t)
	pool := open()
	_, _, _, kube, store := bootstrapFixtureWithPool(t, f, pool)
	if _, err := submission.New(pool).GetRunAuthority(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID); !errors.Is(err, biz.ErrExecutionNotFound) {
		t.Fatal("preflight bound a normal step authority", err)
	}
	select {
	case <-time.After(time.Until(deadline) + 10*time.Millisecond):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	worker, _ := assembleRecoveryOwner(t, f, pool)
	batch, err := worker.ReconcileOnce(ctx)
	unknown, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || readErr != nil || batch.Unresolved != 1 || unknown.CloseReason != "DEADLINE" || unknown.CloseReviewReason != "KFP_CREATE_UNRESOLVED" || unknown.ClosedAt != nil {
		t.Fatalf("unknown original Run lost its fence: %+v %+v %v %v", batch, unknown, err, readErr)
	}
	visible.Store(true)
	pool.Close()
	pool = open()
	worker, _ = assembleRecoveryOwner(t, f, pool)
	batch, err = worker.ReconcileOnce(ctx)
	closing, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || readErr != nil || batch.Unresolved != 1 || closing.CloseAuthority == nil || closing.CloseAuthority.RunID != completeRunID || closing.CloseGeneration != unknown.CloseGeneration || closing.CloseReviewReason != unknown.CloseReviewReason || closing.ClosedAt != nil || closing.CloseEvidence != nil {
		t.Fatalf("live sandbox or recovered owner bypassed writer proof: %+v %+v %v %v", batch, closing, err, readErr)
	}
	f.mu.Lock()
	pod := f.objects["/api/v1/namespaces/"+f.workspace.NamespaceName+"/pods/main-prepare"]
	pod["status"].(map[string]any)["conditions"].([]any)[1].(map[string]any)["status"] = "False"
	f.mu.Unlock()
	pool.Close()
	pool = open()
	worker, _ = assembleRecoveryOwner(t, f, pool)
	batch, err = worker.ReconcileOnce(ctx)
	closed, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || readErr != nil || batch.Closed != 1 || closed.ClosedAt == nil || closed.CloseGeneration != unknown.CloseGeneration || closed.CloseReason != "DEADLINE" || closed.CloseReviewReason != "" || closed.CloseEvidence == nil || closed.CloseEvidence.OwnerTermination == nil || closed.Workspace != nil || closed.Training != nil || closed.Publication != nil {
		t.Fatalf("INITIALIZATION_ABORT_NOT_IMPLEMENTED: completed original owner and stopped sandbox did not durably close: %+v %+v %v %v", batch, closed, err, readErr)
	}
	var proof *biz.PodInitializationAbort
	for _, resource := range closed.CloseEvidence.Resources {
		if resource.UID == completeStepUID("prepare") {
			proof = resource.InitializationAbort
			if resource.ExitCode != nil || !resource.CreationDisabled {
				t.Fatal("persisted abort invented main termination", resource)
			}
		}
	}
	if proof == nil || !proof.ValidAt(closed.CloseEvidence.ObservedAt) || len(proof.InitContainerExits) != 1 || proof.InitContainerExits[0].ExitCode != 0 || !reflect.DeepEqual(proof.UnstartedInitContainers, []string{"kfp-launcher"}) || !reflect.DeepEqual(proof.UnstartedContainers, []string{"wait", "main"}) {
		t.Fatalf("database lost actual init-abort facts: %+v", proof)
	}
	pool.Close()
	pool = open()
	recovered, err := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || !reflect.DeepEqual(closed.CloseEvidence, recovered.CloseEvidence) {
		t.Fatal("reconnect lost persisted abort proof", err)
	}
	if _, err := submission.New(pool).GetRunAuthority(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID); !errors.Is(err, biz.ErrExecutionNotFound) {
		t.Fatal("abort recovery invented ordinary step authority", err)
	}
	_, runs := assembleRecoveryOwner(t, f, pool)
	client := newFixtureRuntimeClient(t, f, pool, kube, store, runs)
	if _, err := client.EnsureTraining(bootstrapCall(ctx, f, "synthetic-bound-train-wait"), &modeldevv1.EnsureTrainingRequest{Context: f.stepContext("train-wait")}); err == nil {
		t.Fatal("late Ensure granted a training permit")
	}
	f.mu.Lock()
	runCreates, creates, stops := f.runCreates, f.creates, f.runStops
	f.mu.Unlock()
	if runCreates != 1 || creates != 0 || stops != 1 {
		t.Fatalf("recovery changed original create permission: runs=%d training=%d stops=%d", runCreates, creates, stops)
	}
	t.Log("actual PG recovered DEADLINE, retained initialization-abort facts with nil main exit, persisted CLOSED, and denied late Ensure without recreating work")
}
