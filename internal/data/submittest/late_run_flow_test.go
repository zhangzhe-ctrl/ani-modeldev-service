//go:build cpu_mainflow

package submittest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestMainFlowLateRunRecoveryFindsOriginalAndClosesWithoutRecreating(t *testing.T) {
	for _, name := range []string{"before-managed-begin", "already-bound-before-response", "ambiguous-discovery-stays-fenced"} {
		t.Run(name, func(t *testing.T) {
			bound, multiple := name == "already-bound-before-response", name == "ambiguous-discovery-stays-fenced"
			const secondRunID = "66666666-7777-4888-9999-aaaaaaaaaaaa"
			f := newCompleteFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			started, release := make(chan struct{}), make(chan struct{})
			var visible atomic.Bool
			var duplicate atomic.Bool
			// Only KFP's external API is faulted: the original accepted creation is
			// hidden, its HTTP response is lost, and reads initially cannot see it.
			f.kfp = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/apis/v2beta1/runs" {
					f.kfpRequest(httptest.NewRecorder(), r)
					close(started)
					select {
					case <-release:
					case <-r.Context().Done():
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"error":"response_lost"}`))
					return
				}
				if r.Method == http.MethodGet && r.URL.Path == "/apis/v2beta1/runs" {
					w.Header().Set("Content-Type", "application/json")
					if !visible.Load() {
						_, _ = w.Write([]byte(`{"runs":[],"total_size":0}`))
						return
					}
					get := r.Clone(r.Context())
					copyURL := *r.URL
					get.URL = &copyURL
					get.URL.Path = "/apis/v2beta1/runs/" + completeRunID
					recorder := httptest.NewRecorder()
					f.kfpRequest(recorder, get)
					var run json.RawMessage = recorder.Body.Bytes()
					items := []json.RawMessage{run}
					if duplicate.Load() { items = append(items, json.RawMessage(strings.ReplaceAll(string(run), completeRunID, secondRunID))) }
					_ = json.NewEncoder(w).Encode(map[string]any{"runs": items, "total_size": len(items)})
					return
				}
				if r.Method == http.MethodGet && r.URL.Path == "/apis/v2beta1/runs/"+secondRunID {
					get := r.Clone(r.Context()); copyURL := *r.URL; get.URL = &copyURL; get.URL.Path = "/apis/v2beta1/runs/"+completeRunID
					recorder := httptest.NewRecorder(); f.kfpRequest(recorder, get)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(strings.ReplaceAll(recorder.Body.String(), completeRunID, secondRunID)))
					return
				}
				if r.Method == http.MethodGet && !visible.Load() {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":"not_found"}`))
					return
				}
				f.kfpRequest(w, r)
			}))
			t.Cleanup(f.kfp.Close)
			open := postgres.Prepare(t)
			pool := open()
			acceptThroughCommandRPC(t, ctx, execution.New(pool), f.request.Admission)
			_, runs := assembleRecoveryOwner(t, f, pool)
			submitter, err := biz.NewPipelineSubmitter(submission.New(pool), runs, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			submitted := make(chan error, 1)
			go func() { _, err := submitter.Submit(ctx, f.request); submitted <- err }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("original CreateRun never reached external API")
			}
			if bound {
				visible.Store(true)
				_, client, _, _, _ := bootstrapFixtureWithPool(t, f, pool)
				if _, err := client.BeginExecution(bootstrapCall(ctx, f, "synthetic-bound-prepare"), &modeldevv1.BeginExecutionRequest{Context: f.stepContext("prepare")}); err != nil {
					t.Fatalf("real Begin could not bind the in-flight Run: %v", err)
				}
				visible.Store(false)
			}
			generation := applyOwnerStop(t, ctx, f, pool)
			close(release)
			if err := <-submitted; !errors.Is(err, biz.ErrPipelineSubmissionUncertain) {
				t.Fatalf("lost create response was not uncertain: %v", err)
			}
			original, err := submission.New(pool).Get(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
			if err != nil || original.State != biz.PipelineDispatchUncertain {
				t.Fatalf("original uncertain attempt was not durable: %+v %v", original, err)
			}
			pool.Close()
			pool = open()
			worker, _ := assembleRecoveryOwner(t, f, pool)
			batch, err := worker.ReconcileOnce(ctx)
			unresolved, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
			if err != nil || readErr != nil || batch.Unresolved != 1 || unresolved.ClosedAt != nil {
				t.Fatalf("unknown late create incorrectly closed: %+v %+v %v %v", batch, unresolved, err, readErr)
			}
			// Public runtime facts must keep a durable review reason across restart.
			raw, err := json.Marshal(unresolved)
			if err != nil {
				t.Fatal(err)
			}
			var facts map[string]any
			if err := json.Unmarshal(raw, &facts); err != nil {
				t.Fatal(err)
			}
			if facts["CloseReviewReason"] != "KFP_CREATE_UNRESOLVED" {
				t.Fatalf("LATE_RUN_CLOSE_NOT_IMPLEMENTED: unknown create lacks durable NEEDS_REVIEW reason: %s", raw)
			}
			visible.Store(true)
			duplicate.Store(multiple)
			pool.Close()
			pool = open()
			worker, runs = assembleRecoveryOwner(t, f, pool)
			batch, err = worker.ReconcileOnce(ctx)
			closed, readErr := lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
			if multiple {
				if err != nil || readErr != nil || batch.Unresolved != 1 || closed.ClosedAt != nil || closed.CloseReviewReason != "MULTIPLE_RUNS" { t.Fatalf("ambiguous external Runs were not fenced: %+v %+v %v %v", batch, closed, err, readErr) }
				duplicate.Store(false)
				pool.Close(); pool = open(); worker, _ = assembleRecoveryOwner(t, f, pool)
				batch, err = worker.ReconcileOnce(ctx)
				closed, readErr = lifecycle.New(pool).GetRuntime(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
				recovered, recordErr := submission.New(pool).Get(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
				if err != nil || readErr != nil || recordErr != nil || batch.Unresolved != 1 || closed.ClosedAt != nil || closed.CloseReviewReason != "MULTIPLE_RUNS" || recovered.AttemptID != original.AttemptID || len(recovered.ConfirmedRuns) != 2 || recovered.ConfirmedRuns[0].RunID != completeRunID || recovered.ConfirmedRuns[1].RunID != secondRunID { t.Fatalf("shorter list erased original ambiguity/Run identities: %+v %+v %v %v %v", batch, recovered, err, readErr, recordErr) }
				t.Log("LATE_RUN_CLOSE: both independently observed Run IDs retained; restart and a shorter later list cannot select one or report CLOSED")
				return
			}
			if err != nil || readErr != nil || batch.Closed != 1 || closed.ClosedAt == nil || closed.CloseGeneration != generation || closed.CloseEvidence == nil || closed.CloseEvidence.RunID != completeRunID {
				t.Fatalf("LATE_RUN_CLOSE_NOT_IMPLEMENTED: original late run not found/stopped/closed: %+v %+v %v %v", batch, closed, err, readErr)
			}
			gotAuthority, authorityErr := submission.New(pool).GetRunAuthority(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
			if (!bound && !errors.Is(authorityErr, biz.ErrExecutionNotFound)) || (bound && (authorityErr != nil || gotAuthority.RunID != completeRunID || gotAuthority.AttemptID != original.AttemptID)) {
				t.Fatalf("owner close recovery changed managed-step authority: %+v %v", gotAuthority, authorityErr)
			}
			recovered, err := submission.New(pool).Get(ctx, f.request.Admission.TenantID, f.request.Admission.ExecutionID)
			if err != nil || recovered.AttemptID != original.AttemptID || recovered.PlanHash != original.PlanHash || len(recovered.ConfirmedRuns) != 1 || recovered.ConfirmedRuns[0].RunID != completeRunID {
				t.Fatalf("late recovery replaced the original attempt: %+v %v", recovered, err)
			}
			submitter, err = biz.NewPipelineSubmitter(submission.New(pool), runs, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := submitter.Submit(ctx, f.request); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			creates, runCreates, runStops := f.creates, f.runCreates, f.runStops
			f.mu.Unlock()
			if creates != 0 || runCreates != 1 || runStops != 1 {
				t.Fatalf("recovery recreated or failed to stop: train=%d run=%d stops=%d", creates, runCreates, runStops)
			}
			t.Log("LATE_RUN_CLOSE: close during CreateRun, lost response, durable NEEDS_REVIEW, restart, strict original Run recovery and CLOSED without another creation")
		})
	}
}
