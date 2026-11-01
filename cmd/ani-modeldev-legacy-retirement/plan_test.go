package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func TestLegacyRetirementSafeReasonRetainsOnlyFixedQualificationLocations(t *testing.T) {
	for _, tc := range []struct{ detail, code string }{
		{"owner workflow completion", "ARCHIVE_GRAPH_COMPLETION"},
		{"unprepared workspace request", "UNPREPARED_WORKSPACE_REQUEST"},
		{"owner pod exits 1/9", "HISTORICAL_POD_EXITS"},
		{"owner remaining managed tasks", "REMAINING_MANAGED_TASKS"},
	} {
		err := fmt.Errorf("CURRENT_OWNER_WRITER_PROOF: %s: private-external-value: %w", tc.detail, biz.ErrRuntimeNotReady)
		want := "CURRENT_OWNER_WRITER_PROOF:" + tc.code + ":" + biz.ErrRuntimeNotReady.Error()
		if got := safeReason(err); got != want || strings.Contains(got, "private-external-value") {
			t.Fatalf("unsafe or incomplete qualification reason: %q", got)
		}
	}
}

type countingTransport struct{ calls int }

func (t *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
}
func TestPlanTransportCannotMutateOriginalCluster(t *testing.T) {
	boundary := &countingTransport{}
	transport := readOnlyTransport{boundary}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		request, _ := http.NewRequestWithContext(context.Background(), method, "https://cluster.invalid/object", nil)
		if _, err := transport.RoundTrip(request); err == nil {
			t.Fatal("plan permitted", method)
		}
	}
	if boundary.calls != 0 {
		t.Fatal("plan sent a mutation")
	}
	request, _ := http.NewRequest("GET", "https://cluster.invalid/object", nil)
	if _, err := transport.RoundTrip(request); err != nil || boundary.calls != 1 {
		t.Fatal("plan cannot read original object")
	}
}
func TestReviewedProofFreezesOriginalGraphAndCAS(t *testing.T) {
	code := int32(143)
	r := biz.LegacyRetirementRequest{Authority: biz.RunAuthorityCandidate{WorkflowUID: "original-workflow"}, ExpectedOwnerRevision: 16, CloseGeneration: 2, CommandFence: 2, ApprovalSHA256: biz.LegacyRetirementApprovalSHA256, Archive: biz.LegacyRetirementArchiveReceipt{SHA256: "original-archive", ObservedAt: time.Now()}, Evidence: biz.ManagedCloseEvidence{Resources: []biz.RuntimeResource{{Kind: "Pod", UID: "original-step", OwnerUID: "original-workflow", ExitCode: &code}}, ObservedAt: time.Now()}, Observation: biz.TrainingRuntimeObservation{Resources: []biz.RuntimeResource{{Kind: "Pod", UID: "original-worker", ExitCode: &code}}, WritersAbsent: true, ObservedAt: time.Now()}}
	initial := proofDigest(r)
	copy := r
	copy.Archive.ObservedAt = time.Now().Add(time.Second)
	copy.Evidence.ObservedAt = time.Now().Add(time.Second)
	copy.Observation.ObservedAt = time.Now().Add(time.Second)
	if proofDigest(copy) != initial {
		t.Fatal("fresh read time changed fixed proof")
	}
	for _, mutate := range []func(*biz.LegacyRetirementRequest){func(x *biz.LegacyRetirementRequest) { x.ExpectedOwnerRevision++ }, func(x *biz.LegacyRetirementRequest) { x.CommandFence++ }, func(x *biz.LegacyRetirementRequest) { x.CloseGeneration++ }, func(x *biz.LegacyRetirementRequest) { x.Authority.WorkflowUID = "replacement" }, func(x *biz.LegacyRetirementRequest) { x.Archive.SHA256 = "replacement" }, func(x *biz.LegacyRetirementRequest) { x.Observation.WritersAbsent = false }} {
		copy = r
		mutate(&copy)
		if proofDigest(copy) == initial {
			t.Fatal("reviewed original proof accepted drift")
		}
	}
	// The digest helper must not mutate the actual evidence receipt passed to CAS.
	if r.Archive.ObservedAt.IsZero() || r.Evidence.ObservedAt.IsZero() || r.Observation.ObservedAt.IsZero() {
		t.Fatal("proof hashing rewrote real observation")
	}
}
func TestApplyRequiresExactFreshStopAdmissionReceipt(t *testing.T) {
	valid := admissionGate{Schema: "ani.modeldev.legacy-retirement.stopped-admission.v1", SourceEvidenceSHA256: strings.Repeat("a", 64), ReviewedPlanSHA256: strings.Repeat("b", 64), ObservedAt: time.Now().UTC(), ClusterID: "f39bf6e2-f3ed-4191-ada3-caeebf86c220", KubeSystemUID: "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24", GovernanceDeploymentUID: "4ae87828-5f2c-4b27-a8b8-d348aeb86ee5", GovernanceImage: "registry.invalid/gov@sha256:fixed", CreatePaused: true, CreateHTTPStatus: 503, CreateReason: "MODELDEV_CREATE_MAINTENANCE", ActiveCreateLeases: 0, RootApplyAuthorized: true}
	path := filepath.Join(t.TempDir(), "gate.json")
	check := func(g admissionGate, want bool) {
		raw, _ := json.Marshal(g)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		err := checkAdmissionGate(path, digest(raw), strings.Repeat("b", 64))
		if (err == nil) != want {
			t.Fatal("stop gate", want, err)
		}
	}
	check(valid, true)
	for _, mutate := range []func(*admissionGate){func(g *admissionGate) { g.CreatePaused = false }, func(g *admissionGate) { g.ActiveCreateLeases = 1 }, func(g *admissionGate) { g.CreateHTTPStatus = 200 }, func(g *admissionGate) { g.CreateReason = "HTTP_503" }, func(g *admissionGate) { g.RootApplyAuthorized = false }, func(g *admissionGate) { g.KubeSystemUID = "other-cluster" }, func(g *admissionGate) { g.ObservedAt = time.Now().Add(-6 * time.Minute) }} {
		copy := valid
		mutate(&copy)
		check(copy, false)
	}
	check(valid, true)
	if checkAdmissionGate(path, "wrong-hash", strings.Repeat("b", 64)) == nil {
		t.Fatal("changed gate bytes accepted")
	}
}
