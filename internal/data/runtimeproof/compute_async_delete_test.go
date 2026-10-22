package runtimeproof_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
)

func TestAutomaticComputeCleanupWaitsForOriginalPodAsynchronousDeletion(t *testing.T) {
	f := newComputeCleanupFixture(t, false)
	// Only Kubernetes deletion is asynchronous: HTTP 200 acknowledges the one
	// DELETE, then GET observes the original UID/deletionTimestamp twice before
	// NotFound. Admission, logs, intents and results use the real PostgreSQL.
	f.deferDelete, f.deferDeleteKind = true, "Pod"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := f.worker(t).ReconcileOnce(ctx)
	if err != nil || result.Unresolved != 0 {
		t.Fatalf("ASYNC_POD_DELETE_NOT_IMPLEMENTED: acknowledged deletion must wait for original UID absence without another attempt: %+v %v", result, err)
	}
	f.mu.Lock()
	deleted := append([]string(nil), f.deleted...)
	remaining, logReads := len(f.objects), f.logReads
	pending := make(map[string]int, len(f.pendingReads))
	for path, reads := range f.pendingReads {
		pending[path] = reads
	}
	runtimeDeleted, workflowDeleted := f.runtimeDeleted, f.workflowDeleted
	f.mu.Unlock()
	if !reflect.DeepEqual(deleted, []string{"TrainJob", "JobSet", "Job", "Pod", "PodGroup", "Workflow", "TrainingRuntime"}) || remaining != 0 || !runtimeDeleted || !workflowDeleted || logReads != 1 {
		t.Fatalf("asynchronous cleanup reissued a DELETE or left an original target: deleted=%v remaining=%d runtime=%v workflow=%v logs=%d", deleted, remaining, runtimeDeleted, workflowDeleted, logReads)
	}
	if len(pending) != 1 {
		t.Fatalf("the asynchronous Pod boundary was not exercised: %v", pending)
	}
	for path, reads := range pending {
		if reads != 0 {
			t.Fatalf("cleanup returned before the original deleting Pod became NotFound: %s remaining_reads=%d", path, reads)
		}
	}
	state, err := lifecycle.New(f.step.open()).GetRuntime(ctx, f.step.f.execution.TenantID, f.step.f.execution.ExecutionID)
	if err != nil || state.ComputeCleanup == nil || state.ComputeCleanup.Phase != "APPLIED" || state.ComputeCleanup.Attempts != 1 || len(state.ComputeCleanup.ConfirmedAbsent) != 7 || !state.ComputeCleanup.LogsReady || state.ClosedAt == nil {
		t.Fatalf("real reconnect lost single-attempt APPLIED/all-seven absence evidence: cleanup=%+v err=%v", state.ComputeCleanup, err)
	}
	if again, err := f.worker(t).ReconcileOnce(ctx); err != nil || again.Examined != 0 {
		t.Fatalf("restart renewed the completed asynchronous cleanup: %+v %v", again, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !reflect.DeepEqual(f.deleted, deleted) {
		t.Fatal("restart reissued a completed original DELETE", f.deleted)
	}
}
