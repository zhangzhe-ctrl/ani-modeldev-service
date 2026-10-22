package biz_test

import (
	"strings"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func TestComputeCleanupResolutionCannotSealBeforeIndependentReconciliation(t *testing.T) {
	start := time.Date(2026, 10, 22, 13, 29, 0, 0, time.UTC)
	resolved := start.Add(10 * time.Minute)
	target := biz.RuntimeResource{APIVersion: "v1", Kind: "Pod", Name: "original", Namespace: "tenant", UID: "original-pod", APIObjectPresent: true}
	absent := target
	absent.APIObjectPresent = false
	for _, mode := range []string{"complete", "only original failure", "missing one UID", "replacement UID", "fake apply time", "before original start", "no independent audit"} {
		t.Run(mode, func(t *testing.T) {
			r := &biz.ComputeCleanupReceipt{CleanupReceipt: biz.CleanupReceipt{Phase: "RECONCILED", StartedAt: start, ReconciledAt: resolved, ReconciledFromPhase: "NEEDS_REVIEW", Requested: []biz.RuntimeResource{target}, ResolvedAbsent: []biz.RuntimeResource{absent}}, Targets: []biz.RuntimeResource{target}, Attempts: 4, LastError: "BOUNDARY_UNCERTAIN", LogsReady: true, OperatorRetry: &biz.ComputeCleanupOperatorRetry{}, MaintenancePlanHash: strings.Repeat("a", 64)}
			switch mode {
			case "only original failure":
				r.Phase = "NEEDS_REVIEW"
			case "missing one UID":
				r.ResolvedAbsent = nil
			case "replacement UID":
				r.ResolvedAbsent[0].UID = "replacement"
			case "fake apply time":
				r.CompletedAt = resolved
			case "before original start":
				r.ReconciledAt = start.Add(-time.Second)
			case "no independent audit":
				r.MaintenancePlanHash = ""
			}
			at := biz.ComputeCleanupSettledAt(r)
			if mode == "complete" {
				if !at.Equal(resolved) || !r.CompletedAt.IsZero() || r.Attempts != 4 || r.LastError != "BOUNDARY_UNCERTAIN" {
					t.Fatal("resolution erased the automatic failure")
				}
			} else if !at.IsZero() {
				t.Fatal("unproven or forged resolution authorized sealing", at)
			}
		})
	}
}
