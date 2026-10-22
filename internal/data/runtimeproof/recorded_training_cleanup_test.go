package runtimeproof_test

import (
	"context"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
)

func TestComputeCleanupReusesCommittedFinalLogsAfterOriginalPodGC(t *testing.T) {
	f := newComputeCleanupClosingFixture(t, false, "", func(f *computeCleanupFixture) { f.captureBeforeClose = true })
	result, err := f.worker(t).ReconcileOnce(context.Background())
	state, readErr := lifecycle.New(f.step.open()).GetRuntime(context.Background(), f.step.f.execution.TenantID, f.step.f.execution.ExecutionID)
	if err != nil || readErr != nil || result.Unresolved != 0 || state.ComputeCleanup == nil || state.ComputeCleanup.Phase != "APPLIED" || !state.ComputeCleanup.LogsReady || len(state.ComputeCleanup.Logs) != 1 || f.logReads != 1 || len(f.deleted) != 6 {
		t.Fatalf("original final log was lost or reread after GC: result=%+v error=%v read=%v reads=%d deletes=%v", result, err, readErr, f.logReads, f.deleted)
	}
	if state.Observation == nil || len(state.Observation.RecordedLogs) != 1 {
		t.Fatal("cleanup erased the original transaction's final tail")
	}
	for _, pod := range state.Observation.Resources {
		if pod.Kind == "Pod" && (pod.APIObjectPresent || !pod.Terminal || pod.ExitCode == nil || *pod.ExitCode != 0) {
			t.Fatal("GC changed the durable original process exit", pod)
		}
	}
	if again, err := f.worker(t).ReconcileOnce(context.Background()); err != nil || again.Examined != 0 || f.logReads != 1 {
		t.Fatal("restart repeated a final log read or cleanup", again, err)
	}
}
