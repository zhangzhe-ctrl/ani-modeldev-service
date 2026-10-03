//go:build cpu_mainflow

package submittest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestMainFlowDeadlineWithoutDispatchNeverCreates(t *testing.T) {
	open := postgres.Prepare(t)
	f := newCompleteFixture(t)
	deadline := time.Now().UTC().Truncate(time.Microsecond).Add(3*time.Second)
	f.request.Admission.Snapshot.DeadlineAt = deadline
	var err error
	f.request.Admission.SpecHash, err = f.request.Admission.Snapshot.Digest()
	if err != nil { t.Fatal(err) }
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	pool := open()
	acceptThroughCommandRPC(t, ctx, execution.New(pool), f.request.Admission)
	worker, _ := assembleRecoveryOwner(t, f, pool)
	if batch, err := worker.ReconcileOnce(ctx); err != nil || batch.Examined != 0 { t.Fatalf("premature no-dispatch deadline: %+v %v", batch, err) }
	pool.Close()
	pool = open()
	worker, runs := assembleRecoveryOwner(t, f, pool)
	workerContext, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(workerContext); close(done) }()
	defer func() { stop(); <-done }()
	ticker := time.NewTicker(50*time.Millisecond)
	defer ticker.Stop()
	for {
		closed, err := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
		if err != nil { t.Fatal(err) }
		if closed.ClosedAt != nil {
			if closed.CloseGeneration != 1 || closed.CloseReason != "DEADLINE" || closed.CloseRequestedAt.Before(deadline) || closed.CloseEvidence == nil || !closed.CloseEvidence.NoDispatch { t.Fatalf("DEADLINE_NOT_IMPLEMENTED: never-dispatched execution did not close at its original deadline: %+v", closed) }
			break
		}
		select {
		case err := <-done: t.Fatalf("deadline worker exited: %v", err)
		case <-ctx.Done(): t.Fatal("DEADLINE_NOT_IMPLEMENTED: no-dispatch deadline never closed")
		case <-ticker.C:
		}
	}
	stop()
	if err := <-done; err != nil { t.Fatal(err) }
	acceptThroughCommandRPC(t, ctx, execution.New(pool), f.request.Admission)
	submitter, err := biz.NewPipelineSubmitter(submission.New(pool), runs, time.Second)
	if err != nil { t.Fatal(err) }
	if _, err := submitter.Submit(ctx, f.request); !errors.Is(err, biz.ErrPipelineDispatchBlocked) { t.Fatalf("expired execution got a new dispatch: %v", err) }
	f.mu.Lock()
	runCreates, trainingCreates := f.runCreates, f.creates
	f.mu.Unlock()
	if runCreates != 0 || trainingCreates != 0 { t.Fatal("expired no-dispatch execution created compute") }
	t.Log("DEADLINE_NO_DISPATCH: original deadline closed after restart without creating Run or training; late submission remained fenced")
}

func TestMainFlowDeadlineClosesOriginalRunAfterOwnerRestart(t *testing.T) {
	open := postgres.Prepare(t)
	f := newCompleteFixture(t)
	deadline := time.Now().UTC().Truncate(time.Microsecond).Add(6 * time.Second)
	f.request.Admission.Snapshot.DeadlineAt = deadline
	var err error
	f.request.Admission.SpecHash, err = f.request.Admission.Snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := open()
	_, client, facts, kube, store := bootstrapFixtureWithPool(t, f, pool)
	if _, err := client.BeginExecution(bootstrapCall(ctx, f, "synthetic-bound-prepare"), &modeldevv1.BeginExecutionRequest{Context: f.stepContext("prepare")}); err != nil {
		t.Fatalf("DEADLINE_PREFLIGHT: actual authenticated Begin failed: %v", err)
	}
	worker, _ := assembleRecoveryOwner(t, f, pool)
	if batch, err := worker.ReconcileOnce(ctx); err != nil || batch.Examined != 0 {
		t.Fatalf("deadline closed before the original frozen time: %+v %v", batch, err)
	}
	before, err := facts.GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || before.CloseGeneration != 0 || before.ClosedAt != nil {
		t.Fatalf("premature deadline fence: %+v %v", before, err)
	}
	pool.Close()
	pool = open()
	// Replaying admission after reconnect must not extend the deadline.
	acceptThroughCommandRPC(t, ctx, execution.New(pool), f.request.Admission)
	worker, _ = assembleRecoveryOwner(t, f, pool)
	workerContext, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(workerContext); close(done) }()
	defer func() { stop(); <-done }()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		closed, err := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
		if err != nil {
			t.Fatal(err)
		}
		if closed.ClosedAt != nil {
			if closed.CloseGeneration != 1 || closed.CloseReason != "DEADLINE" || closed.CloseRequestedAt.Before(deadline) || closed.CloseEvidence == nil || closed.Training != nil || closed.Publication != nil {
				t.Fatalf("DEADLINE_NOT_IMPLEMENTED: original deadline lacks durable writer-free closure: %+v", closed)
			}
			break
		}
		select {
		case err := <-done:
			t.Fatalf("DEADLINE_NOT_IMPLEMENTED: recovery worker exited: %v", err)
		case <-ctx.Done():
			t.Fatal("DEADLINE_NOT_IMPLEMENTED: original deadline did not close the Run")
		case <-ticker.C:
		}
	}
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	pool.Close()
	pool = open()
	admitted, err := execution.New(pool).Get(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
	if err != nil || !admitted.Snapshot.DeadlineAt.Equal(deadline) || string(admitted.States.Close) != "CLOSED" {
		t.Fatalf("deadline/reconnect changed the frozen execution: %+v %v", admitted, err)
	}
	_, runsClient := assembleRecoveryOwner(t, f, pool)
	client = newFixtureRuntimeClient(t, f, pool, kube, store, runsClient)
	if _, err := client.EnsureTraining(bootstrapCall(ctx, f, "synthetic-bound-train-wait"), &modeldevv1.EnsureTrainingRequest{Context: f.stepContext("train-wait")}); err == nil {
		t.Fatal("expired closed execution granted a new training permit")
	}
	f.mu.Lock()
	runs, training, stops := f.runCreates, f.creates, f.runStops
	f.mu.Unlock()
	if runs != 1 || training != 0 || stops < 1 {
		t.Fatalf("deadline repeated or failed to stop original work: runs=%d training=%d stops=%d", runs, training, stops)
	}
	t.Log("DEADLINE_CLOSED: original frozen deadline survived admission replay and owner restart; original KFP stopped, writers absent, no late training creation")
}
