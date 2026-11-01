package lifecycle_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	codec "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/admissionfixture"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	compute "github.com/zhangzhe-ctrl/ani-resource-service/api/compute/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// KFP/Kubernetes observations enter at their typed external boundary. Admission,
// owner association, close fence, receipts, revision, release anchor and ACK use
// the original production repositories and a real restricted PostgreSQL role.
func legacyRetirementFixture(t *testing.T, id string, ownerOnly bool) (func() *pgxpool.Pool, biz.Admission, biz.LegacyRetirementRequest) {
	t.Helper()
	ctx := context.Background()
	open := postgres.Prepare(t)
	scope, ok := biz.LookupLegacyRetirementScope(id)
	if !ok {
		t.Fatal("fixture scope")
	}
	a := gpuReleaseAdmission(t)
	a.TenantID, a.ExecutionID = scope.TenantID, scope.ExecutionID
	if id != "eca4d47b-5911-4f0f-8b45-89c1be775925" {
		a.Snapshot.ComputeOrigin = conformance.SnapshotV1().ComputeOrigin
		a.Snapshot.GPU = nil
	} else {
		a.OperationID = "1bc185ad-0977-4b3f-bb1c-404bc37e94fa"
	}
	a.Snapshot.Environment.ClusterID, a.Snapshot.Environment.NamespaceName, a.Snapshot.Environment.NamespaceUID = scope.ClusterID, scope.NamespaceName, scope.NamespaceUID
	var selection compute.ComputeSelection
	if protojson.Unmarshal([]byte(a.Snapshot.ComputeOrigin.SelectionJSON), &selection) != nil {
		t.Fatal("fixture selection")
	}
	if selection.Gpu != nil {
		selection.Gpu.ClusterId = scope.ClusterID
	}
	var err error
	selection.SelectionDigest, err = compute.SelectionDigest(&selection)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := compute.CanonicalSelection(&selection)
	if err != nil {
		t.Fatal(err)
	}
	a.Snapshot.ComputeOrigin, err = codec.DecodeComputeSelection(raw, selection.SelectionDigest)
	if err != nil {
		t.Fatal(err)
	}
	a = admissionfixture.Bind(t, a)
	pool := open()
	if _, err = execution.New(pool).Accept(ctx, a); err != nil {
		t.Fatal(err)
	}
	submissions := submission.New(pool)
	reserved, err := submissions.Reserve(ctx, biz.PipelineDispatchRequest{Admission: a, Owner: biz.PipelineOwnerConfiguration{Reference: "legacy-retirement-fixture", RevisionSHA256: strings.Repeat("a", 64), PipelineRoot: "s3://fixture-kfp-artifacts/retirement"}})
	if err != nil || reserved.SendPermit == nil {
		t.Fatal("fixture dispatch", err)
	}
	owner := biz.RunAuthorityCandidate{TenantID: a.TenantID, ExecutionID: a.ExecutionID, OperationID: a.OperationID, SpecHash: a.SpecHash, AttemptID: reserved.Dispatch.AttemptID, PlanHash: reserved.Dispatch.PlanHash, RunID: scope.RunID, NamespaceName: scope.NamespaceName, NamespaceUID: scope.NamespaceUID, WorkflowName: scope.WorkflowName, WorkflowUID: scope.WorkflowUID}
	if _, err = submissions.RecordSubmissionConfirmed(ctx, *reserved.SendPermit, biz.PipelineSubmissionObservation{State: biz.PipelineSubmissionConfirmed, RunID: scope.RunID}, time.Now().UTC().Truncate(time.Microsecond)); err != nil {
		t.Fatal("fixture confirmation", err)
	}
	r := lifecycle.New(pool)
	if !ownerOnly {
		if _, err = submissions.BindRunAuthority(ctx, owner); err != nil {
			t.Fatal(err)
		}
		w := a.Snapshot.Workspace
		workspace := biz.WorkspaceBinding{Mode: w.Mode, NamespaceName: scope.NamespaceName, NamespaceUID: scope.NamespaceUID, PVCName: biz.ExpectedWorkspacePVCName(a.ExecutionID), PVCUID: uuid.NewString(), InputSubpath: w.InputSubpath, TrainingSubpath: w.TrainingSubpath, ReportsSubpath: w.ReportsSubpath, PublicationSubpath: w.PublicationSubpath, WorkSubpath: w.WorkSubpath, PreparedManifestSHA256: strings.Repeat("1", 64), PreparedManifestBytes: 256}
		startManagedRoleV21(t, r, owner, "prepare")
		if _, _, err = r.RecordPrepared(ctx, owner, workspace); err != nil {
			t.Fatal(err)
		}
		training, e := reserveTrainingV21(t, r, ctx, owner)
		if e != nil || !training.SendPermit {
			t.Fatal("fixture training", e)
		}
		handle := biz.TrainingHandle{NamespaceUID: scope.NamespaceUID, PVCUID: workspace.PVCUID, TrainJobUID: "retirement-trainjob-uid"}
		if _, err = r.RecordTrainingHandle(ctx, owner, handle); err != nil {
			t.Fatal(err)
		}
		if _, _, err = r.RequestRuntimeClose(ctx, owner, "STEP_FAILED"); err != nil {
			t.Fatal(err)
		}
		if _, err = r.MarkOwnerCloseReview(ctx, a.TenantID, a.ExecutionID, "TRAINING_LOGS_UNAVAILABLE"); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err = execution.New(pool).ApplyCloseIntent(ctx, biz.CloseIntent{TenantID: a.TenantID, ExecutionID: a.ExecutionID, OperationID: a.OperationID, SpecHash: a.SpecHash, SourceGeneration: 1, Reason: biz.CloseReasonUserStop, RequestedAt: time.Now().UTC().Truncate(time.Microsecond), RequestedActor: a.Actor}); err != nil {
			t.Fatal(err)
		}
		if _, err = r.RecordClosingRun(ctx, owner); err != nil {
			t.Fatal(err)
		}
		if _, err = r.MarkOwnerCloseReview(ctx, a.TenantID, a.ExecutionID, "KFP_CREATE_UNRESOLVED"); err != nil {
			t.Fatal(err)
		}
	}
	state, err := r.GetRuntime(ctx, a.TenantID, a.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	evidence := biz.ManagedCloseEvidence{RunID: owner.RunID, WorkflowUID: owner.WorkflowUID, ObservedAt: now, OwnerTermination: &biz.ManagedOwnerTermination{RunState: "FAILED", RunFinishedAt: now.Add(-time.Second), WorkflowPhase: "Failed", WorkflowFinishedAt: now.Add(-time.Second), WorkflowResourceVersion: "original-archive-rv", WorkflowNodesSHA256: strings.Repeat("c", 64)}}
	zero := int32(0)
	for _, role := range []string{"prepare", "train-wait", "collect", "publish"} {
		evidence.Resources = append(evidence.Resources, biz.RuntimeResource{APIVersion: "v1", Kind: "Pod", Namespace: scope.NamespaceName, Name: "retirement-" + role, UID: "retirement-" + role + "-uid", OwnerUID: scope.WorkflowUID, APIObjectPresent: true, Terminal: true, ExitCode: &zero})
	}
	request := biz.LegacyRetirementRequest{Authority: owner, ExpectedOwnerRevision: state.OwnerRevision, CloseGeneration: state.CloseGeneration, CommandFence: state.CloseGeneration, ApprovalSHA256: biz.LegacyRetirementApprovalSHA256, Actor: "maintenance:human-approved-retirement", Archive: biz.LegacyRetirementArchiveReceipt{Source: biz.LegacyRetirementArchiveSource, ExecutionID: scope.ExecutionID, NamespaceName: scope.NamespaceName, NamespaceUID: scope.NamespaceUID, RunID: scope.RunID, WorkflowName: scope.WorkflowName, WorkflowUID: scope.WorkflowUID, SHA256: scope.ArchiveSHA256, ArchivedResourceVersion: evidence.OwnerTermination.WorkflowResourceVersion, ObservedAt: now}, Evidence: evidence}
	if !ownerOnly {
		exit := int32(143)
		request.TrainingExitKind = biz.LegacyRetirementOriginalFailure
		request.Observation = biz.TrainingRuntimeObservation{Handle: *state.TrainingHandle, Outcome: "FAILED", WritersAbsent: true, ObservedAt: now, Resources: []biz.RuntimeResource{
			{APIVersion: "trainer.kubeflow.org/v1alpha1", Kind: "TrainJob", Namespace: scope.NamespaceName, Name: state.Training.Name, UID: state.TrainingHandle.TrainJobUID, APIObjectPresent: true, Terminal: true},
			{APIVersion: "jobset.x-k8s.io/v1alpha2", Kind: "JobSet", Namespace: scope.NamespaceName, Name: "retirement-set", UID: "retirement-jobset-uid", OwnerUID: state.TrainingHandle.TrainJobUID, APIObjectPresent: true, Terminal: true},
			{APIVersion: "batch/v1", Kind: "Job", Namespace: scope.NamespaceName, Name: "retirement-job", UID: "retirement-job-uid", OwnerUID: "retirement-jobset-uid", APIObjectPresent: true, Terminal: true},
			{APIVersion: "v1", Kind: "Pod", Namespace: scope.NamespaceName, Name: "retirement-worker", UID: scope.TrainingPodUID, OwnerUID: "retirement-job-uid", APIObjectPresent: true, Terminal: true, ExitCode: &exit},
		}}
		request.UnrecoverableLogs = []biz.LegacyTrainingLogDisposition{{Status: biz.LegacyTrainingLogsUnrecoverable, PodUID: scope.TrainingPodUID, ContainerID: scope.TrainingContainerID, KubeletErrorBytes: 115, KubeletErrorSHA256: scope.TrainingLogErrorSHA256, ObservedAt: now}}
	}
	return open, a, request
}

func TestLegacyRetirementPostgresKeepsUnknownSuspendedOriginalExit(t *testing.T) {
	for _, id := range []string{"1cce950b-d6e9-453e-aa11-fffd8846224b", "4a1a92f7-4d94-439d-a8d7-1833f38c7dbc"} {
		t.Run(id, func(t *testing.T) {
			open, a, request := legacyRetirementFixture(t, id, false)
			request.TrainingExitKind = biz.LegacyRetirementOriginalSuspended143
			request.Observation.Outcome = "UNKNOWN"
			for i := range request.Observation.Resources {
				r := &request.Observation.Resources[i]
				if r.Kind == "Pod" {
					continue
				}
				r.Terminal, r.CreationDisabled = false, true
				r.Suspension = &biz.TrainingControllerSuspension{Generation: 2, AcknowledgedAt: request.Observation.ObservedAt, WorkerExitRetention: r.Kind == "Job"}
			}
			repository := lifecycle.NewLegacyRetirement(open())
			ctx := context.Background()
			for name, change := range map[string]func(*biz.LegacyRetirementRequest){
				"invented-controller-failure-kind": func(r *biz.LegacyRetirementRequest) { r.TrainingExitKind = biz.LegacyRetirementOriginalFailure },
				"unclassified":                     func(r *biz.LegacyRetirementRequest) { r.TrainingExitKind = "" },
				"running":                          func(r *biz.LegacyRetirementRequest) { r.Observation.Outcome = "RUNNING" },
				"success":                          func(r *biz.LegacyRetirementRequest) { r.Observation.Outcome = "SUCCEEDED" },
				"writer-present":                   func(r *biz.LegacyRetirementRequest) { r.Observation.WritersAbsent = false },
				"missing-ack":                      func(r *biz.LegacyRetirementRequest) { r.Observation.Resources[0].Suspension = nil },
				"old-ack": func(r *biz.LegacyRetirementRequest) {
					r.Observation.Resources[0].Suspension.AcknowledgedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
				},
				"changed-generation":          func(r *biz.LegacyRetirementRequest) { r.Observation.Resources[1].Suspension.Generation = 3 },
				"changed-observed-generation": func(r *biz.LegacyRetirementRequest) { r.Observation.Resources[1].Suspension.ObservedGeneration = 1 },
				"unretained-worker": func(r *biz.LegacyRetirementRequest) {
					r.Observation.Resources[2].Suspension.WorkerExitRetention = false
				},
				"controller-can-create":       func(r *biz.LegacyRetirementRequest) { r.Observation.Resources[0].CreationDisabled = false },
				"missing-original-controller": func(r *biz.LegacyRetirementRequest) { r.Observation.Resources = r.Observation.Resources[1:] },
				"changed-original-pod":        func(r *biz.LegacyRetirementRequest) { r.Observation.Resources[3].UID = uuid.NewString() },
				"invented-exit":               func(r *biz.LegacyRetirementRequest) { code := int32(0); r.Observation.Resources[3].ExitCode = &code },
				"missing-exit":                func(r *biz.LegacyRetirementRequest) { r.Observation.Resources[3].ExitCode = nil },
				"not-terminal":                func(r *biz.LegacyRetirementRequest) { r.Observation.Resources[3].Terminal = false },
				"wrong-parent": func(r *biz.LegacyRetirementRequest) {
					r.Observation.Resources[3].OwnerUID = r.Observation.Resources[0].UID
				},
				"extra-worker": func(r *biz.LegacyRetirementRequest) {
					extra := r.Observation.Resources[3]
					extra.UID = uuid.NewString()
					r.Observation.Resources = append(r.Observation.Resources, extra)
				},
			} {
				t.Run(name, func(t *testing.T) {
					bad := copyLegacyRequest(t, request)
					change(&bad)
					if err := repository.ValidateLegacyRetirement(ctx, bad); err == nil {
						t.Fatal("changed original proof passed plan")
					}
					if _, err := repository.ConfirmLegacyRetirement(ctx, bad); err == nil {
						t.Fatal("changed original proof committed")
					}
				})
			}
			if err := repository.ValidateLegacyRetirement(ctx, request); err != nil {
				t.Fatal("truthful suspended exit rejected", err)
			}
			closed, err := repository.ConfirmLegacyRetirement(ctx, request)
			if err != nil || closed.ClosedAt == nil || closed.Observation.Outcome != "UNKNOWN" || closed.LegacyRetirement.TrainingExitKind != biz.LegacyRetirementOriginalSuspended143 {
				t.Fatal("truthful outcome was rewritten", err)
			}
			step, err := repository.BeginStepExitCleanup(ctx, request.Authority, closed.CloseGeneration, request.Evidence)
			if err != nil {
				t.Fatal(err)
			}
			removed := append([]biz.RuntimeResource{}, step.StepExitCleanup.Evidence.Resources...)
			for i := range removed {
				removed[i].APIObjectPresent = false
			}
			if _, err = repository.CompleteStepExitCleanup(ctx, request.Authority, closed.CloseGeneration, removed); err != nil {
				t.Fatal(err)
			}
			state, err := repository.BeginComputeCleanup(ctx, request.Authority, closed.CloseGeneration)
			if err != nil {
				t.Fatal(err)
			}
			state, err = repository.StartComputeCleanupAttempt(ctx, request.Authority, closed.CloseGeneration)
			if err != nil {
				t.Fatal(err)
			}
			removed = append([]biz.RuntimeResource{}, state.ComputeCleanup.Targets...)
			for i := range removed {
				removed[i].APIObjectPresent = false
			}
			state, err = repository.RecordComputeCleanupResult(ctx, request.Authority, closed.CloseGeneration, 1, removed, "")
			if err != nil || !biz.LegacyRetirementComputeCleanupApplied(biz.Execution{Admission: a}, request.Authority, state) || state.Observation.Outcome != "UNKNOWN" || state.ComputeCleanup.LogsReady || len(state.ComputeCleanup.Logs) != 0 {
				t.Fatal("suspended original cleanup fabricated outcome/logs", err)
			}
		})
	}
}

func copyLegacyRequest(t *testing.T, r biz.LegacyRetirementRequest) biz.LegacyRetirementRequest {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var out biz.LegacyRetirementRequest
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("copy request")
	}
	return out
}

func TestLegacyRetirementPostgresRejectsChangedScopeCASArchiveAndInventedLogs(t *testing.T) {
	open, a, request := legacyRetirementFixture(t, "1cce950b-d6e9-453e-aa11-fffd8846224b", false)
	ctx := context.Background()
	repository := lifecycle.NewLegacyRetirement(open())
	for name, change := range map[string]func(*biz.LegacyRetirementRequest){
		"tenant":              func(r *biz.LegacyRetirementRequest) { r.Authority.TenantID = uuid.NewString() },
		"execution":           func(r *biz.LegacyRetirementRequest) { r.Authority.ExecutionID = uuid.NewString() },
		"operation":           func(r *biz.LegacyRetirementRequest) { r.Authority.OperationID = uuid.NewString() },
		"spec-hash":           func(r *biz.LegacyRetirementRequest) { r.Authority.SpecHash = strings.Repeat("0", 64) },
		"attempt":             func(r *biz.LegacyRetirementRequest) { r.Authority.AttemptID = uuid.NewString() },
		"plan-hash":           func(r *biz.LegacyRetirementRequest) { r.Authority.PlanHash = strings.Repeat("0", 64) },
		"run-id":              func(r *biz.LegacyRetirementRequest) { r.Authority.RunID = uuid.NewString() },
		"generation":          func(r *biz.LegacyRetirementRequest) { r.CloseGeneration++ },
		"revision":            func(r *biz.LegacyRetirementRequest) { r.ExpectedOwnerRevision++ },
		"fence":               func(r *biz.LegacyRetirementRequest) { r.CommandFence++ },
		"namespace-uid":       func(r *biz.LegacyRetirementRequest) { r.Authority.NamespaceUID = uuid.NewString() },
		"namespace-name":      func(r *biz.LegacyRetirementRequest) { r.Authority.NamespaceName = "another-namespace" },
		"workflow-uid":        func(r *biz.LegacyRetirementRequest) { r.Authority.WorkflowUID = uuid.NewString() },
		"workflow-name":       func(r *biz.LegacyRetirementRequest) { r.Authority.WorkflowName = "another-workflow" },
		"archive-sha":         func(r *biz.LegacyRetirementRequest) { r.Archive.SHA256 = strings.Repeat("0", 64) },
		"archive-live-source": func(r *biz.LegacyRetirementRequest) { r.Archive.Source = "LIVE_GET" },
		"not-exited": func(r *biz.LegacyRetirementRequest) {
			r.Observation.Resources[3].Terminal = false
			r.Observation.Resources[3].ExitCode = nil
		},
		"missing-job-parent": func(r *biz.LegacyRetirementRequest) {
			r.Observation.Resources[3].OwnerUID = r.Observation.Resources[1].UID
			r.Observation.Resources = append(r.Observation.Resources[:2], r.Observation.Resources[3])
		},
		"controller-still-creating": func(r *biz.LegacyRetirementRequest) {
			r.Observation.Resources[0].Terminal = false
			r.Observation.Resources[0].CreationDisabled = false
		},
		"wrong-pod": func(r *biz.LegacyRetirementRequest) { r.Observation.Resources[3].UID = uuid.NewString() },
		"captured": func(r *biz.LegacyRetirementRequest) {
			r.Observation.Resources[3].TrainingLog = &biz.RecordedTrainingLog{}
		},
		"missing-disposition":  func(r *biz.LegacyRetirementRequest) { r.UnrecoverableLogs = nil },
		"captured-disposition": func(r *biz.LegacyRetirementRequest) { r.UnrecoverableLogs[0].Status = "CAPTURED" },
		"wrong-error-hash": func(r *biz.LegacyRetirementRequest) {
			r.UnrecoverableLogs[0].KubeletErrorSHA256 = strings.Repeat("0", 64)
		},
		"short-container":   func(r *biz.LegacyRetirementRequest) { r.UnrecoverableLogs[0].ContainerID = "4cddb476" },
		"missing-step-exit": func(r *biz.LegacyRetirementRequest) { r.Evidence.Resources[0].ExitCode = nil },
	} {
		t.Run(name, func(t *testing.T) {
			bad := copyLegacyRequest(t, request)
			change(&bad)
			if err := repository.ValidateLegacyRetirement(ctx, bad); err == nil {
				t.Fatal("invalid qualification plan passed")
			}
			if _, err := repository.ConfirmLegacyRetirement(ctx, bad); err == nil {
				t.Fatal("invalid qualification committed")
			}
			state, err := lifecycle.New(open()).GetRuntime(ctx, a.TenantID, a.ExecutionID)
			if err != nil || state.ClosedAt != nil || state.LegacyRetirement != nil || state.OwnerRevision != request.ExpectedOwnerRevision {
				t.Fatal("failed CAS changed original aggregate", err)
			}
		})
	}
	if err := repository.ValidateLegacyRetirement(ctx, request); err != nil {
		t.Fatal("read-only full qualification", err)
	}
	unmodified, err := lifecycle.New(open()).GetRuntime(ctx, a.TenantID, a.ExecutionID)
	if err != nil || unmodified.ClosedAt != nil || unmodified.LegacyRetirement != nil || unmodified.OwnerRevision != request.ExpectedOwnerRevision {
		t.Fatal("read-only plan wrote execution", err)
	}
	closed, err := repository.ConfirmLegacyRetirement(ctx, request)
	if err != nil || closed.ClosedAt == nil || closed.LegacyRetirement == nil {
		t.Fatal("verified qualification did not close", err)
	}
	if _, err := lifecycle.New(open()).GetRuntime(ctx, a.TenantID, a.ExecutionID); !errors.Is(err, biz.ErrPersistence) {
		t.Fatal("ordinary runtime accepted maintenance journal", err)
	}
	if _, err := lifecycle.New(open()).BeginComputeCleanup(ctx, request.Authority, closed.CloseGeneration); err == nil {
		t.Fatal("ordinary cleanup accepted unrecoverable logs")
	}
	replay, err := lifecycle.NewLegacyRetirement(open()).ConfirmLegacyRetirement(ctx, request)
	if err != nil || replay.OwnerRevision != closed.OwnerRevision || !replay.ClosedAt.Equal(*closed.ClosedAt) {
		t.Fatal("replay renewed original closure", err)
	}
	t.Log("LEGACY_RETIREMENT_CAS PASS: fixed scope, original identity/revision/generation/fence, exact archive and true unavailable logs; default runtime remains strict")
}

func TestLegacyRetirementPostgresReusesOriginalCleanupLimitReleaseAnchorAndACK(t *testing.T) {
	for _, id := range []string{"1cce950b-d6e9-453e-aa11-fffd8846224b", "4a1a92f7-4d94-439d-a8d7-1833f38c7dbc", "eca4d47b-5911-4f0f-8b45-89c1be775925"} {
		t.Run(id, func(t *testing.T) {
			open, a, request := legacyRetirementFixture(t, id, id == "eca4d47b-5911-4f0f-8b45-89c1be775925")
			ctx := context.Background()
			r := lifecycle.NewLegacyRetirement(open())
			closed, err := r.ConfirmLegacyRetirement(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if a.ExecutionID == "eca4d47b-5911-4f0f-8b45-89c1be775925" {
				if _, err := r.RecordLegacyRetirementGPURelease(ctx, request.Authority, closed.OwnerRevision, legacyPriorGPUReceipt()); err == nil {
					t.Fatal("prior generic Gov ACK bypassed original step/compute cleanup")
				}
			}
			if _, err = r.BeginComputeCleanup(ctx, request.Authority, closed.CloseGeneration); err == nil {
				t.Fatal("compute cleanup before original steps")
			}
			if id != "eca4d47b-5911-4f0f-8b45-89c1be775925" {
				if _, err = r.SealGPURelease(ctx, a.TenantID, a.ExecutionID); err == nil {
					t.Fatal("GPU release before original cleanup")
				}
			}
			step, err := r.BeginStepExitCleanup(ctx, request.Authority, closed.CloseGeneration, request.Evidence)
			if err != nil {
				t.Fatal(err)
			}
			removed := append([]biz.RuntimeResource{}, step.StepExitCleanup.Evidence.Resources...)
			for i := range removed {
				removed[i].APIObjectPresent = false
			}
			if _, err = r.CompleteStepExitCleanup(ctx, request.Authority, closed.CloseGeneration, removed[:len(removed)-1]); err == nil {
				t.Fatal("partial step UID list accepted")
			}
			if _, err = r.CompleteStepExitCleanup(ctx, request.Authority, closed.CloseGeneration, removed); err != nil {
				t.Fatal(err)
			}
			r = lifecycle.NewLegacyRetirement(open())
			state, err := r.BeginComputeCleanup(ctx, request.Authority, closed.CloseGeneration)
			if err != nil {
				t.Fatal(err)
			}
			if state.ComputeCleanup.LogsReady || len(state.ComputeCleanup.Logs) != 0 {
				t.Fatal("unavailable log relabeled captured")
			}
			if _, err = r.RecordComputeCleanupLogs(ctx, request.Authority, closed.CloseGeneration, []biz.RecordedTrainingLog{{}}); err == nil {
				t.Fatal("fake captured log accepted")
			}
			for attempt := uint32(1); attempt <= 3; attempt++ {
				state, err = r.StartComputeCleanupAttempt(ctx, request.Authority, closed.CloseGeneration)
				if err != nil || state.ComputeCleanup.Attempts != attempt {
					t.Fatal("original attempt budget", err)
				}
				if !biz.LegacyRetirementCleanupAuthorized(biz.Execution{Admission: a}, request.Authority, state) || state.ComputeCleanup.LogsReady {
					t.Fatal("separate maintenance intent missing")
				}
				if _, err = r.RecordComputeCleanupLogs(ctx, request.Authority, closed.CloseGeneration, nil); err == nil {
					t.Fatal("unavailable logs relabeled as empty captured logs")
				}
				if attempt < 3 {
					if _, err = r.RecordComputeCleanupResult(ctx, request.Authority, closed.CloseGeneration, attempt, nil, "BOUNDARY_UNCERTAIN"); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err = r.StartComputeCleanupAttempt(ctx, request.Authority, closed.CloseGeneration); !errors.Is(err, biz.ErrCleanupUncertain) {
				t.Fatal("fourth cleanup attempt granted", err)
			}
			state, err = r.GetRuntime(ctx, a.TenantID, a.ExecutionID)
			if err != nil || state.ComputeCleanup.Attempts != 3 {
				t.Fatal(err)
			}
			if _, err = r.AdmitComputeCleanupRetry(ctx, request.Authority, closed.CloseGeneration, "maintenance:retirement", nil); err == nil {
				t.Fatal("maintenance exceeded three attempts through operator retry")
			}
			removed = append([]biz.RuntimeResource{}, state.ComputeCleanup.Targets...)
			for i := range removed {
				removed[i].APIObjectPresent = false
			}
			forged := append([]biz.RuntimeResource{}, removed...)
			forged[0].UID = uuid.NewString()
			if _, err = r.RecordComputeCleanupResult(ctx, request.Authority, closed.CloseGeneration, 3, forged, ""); err == nil {
				t.Fatal("foreign UID cleanup accepted")
			}
			state, err = r.RecordComputeCleanupResult(ctx, request.Authority, closed.CloseGeneration, 3, removed, "")
			if err != nil || !biz.LegacyRetirementComputeCleanupApplied(biz.Execution{Admission: a}, request.Authority, state) || state.ComputeCleanup.LogsReady {
				t.Fatal("original cleanup receipt", err)
			}
			if biz.ComputeCleanupApplied(biz.OperationRecord{QueryRecord: biz.QueryRecord{Execution: biz.Execution{Admission: a}, Runtime: state}, Authority: &request.Authority}) {
				t.Fatal("ordinary captured-log cleanup validator widened")
			}
			if _, err := lifecycle.NewLegacyRetirement(open()).SealGPURelease(ctx, a.TenantID, a.ExecutionID); err == nil {
				t.Fatal("maintenance fabricated a new MD scope/outbox/event")
			}
			if a.ExecutionID == "eca4d47b-5911-4f0f-8b45-89c1be775925" {
				receipt := legacyPriorGPUReceipt()
				for name, change := range map[string]func(*biz.LegacyRetirementPriorGPURelease){"new-event": func(r *biz.LegacyRetirementPriorGPURelease) { r.Event.ReleaseEventID = uuid.NewString() }, "new-reason": func(r *biz.LegacyRetirementPriorGPURelease) { r.Event.Reason = "ABORTED_CLEANED" }, "foreign-ack": func(r *biz.LegacyRetirementPriorGPURelease) { r.ACK.ResourceID = uuid.NewString() }, "applied-not-actual-already": func(r *biz.LegacyRetirementPriorGPURelease) { r.ACK.Status = "APPLIED" }, "unreleased-charge": func(r *biz.LegacyRetirementPriorGPURelease) { r.ReleasedUnits-- }, "account-imbalance": func(r *biz.LegacyRetirementPriorGPURelease) { r.AfterAccounts[0].OccupiedUnits++ }, "changed-account-delta": func(r *biz.LegacyRetirementPriorGPURelease) { r.AfterEvidenceSHA256 = strings.Repeat("b", 64) }, "different-old-receipt": func(r *biz.LegacyRetirementPriorGPURelease) { r.ReceiptID = uuid.NewString() }} {
					raw, _ := json.Marshal(receipt)
					var bad biz.LegacyRetirementPriorGPURelease
					json.Unmarshal(raw, &bad)
					change(&bad)
					if _, err := r.RecordLegacyRetirementGPURelease(ctx, request.Authority, state.OwnerRevision, bad); err == nil {
						t.Fatal("original Gov receipt invariant widened", name)
					}
				}
				if _, err := r.RecordLegacyRetirementGPURelease(ctx, request.Authority, state.OwnerRevision+1, receipt); err == nil {
					t.Fatal("original prior-GPU journal CAS widened")
				}
				state, err = r.RecordLegacyRetirementGPURelease(ctx, request.Authority, state.OwnerRevision, receipt)
				if err != nil || state.GPURelease != nil || state.LegacyRetirement.PriorGPURelease == nil {
					t.Fatal("original generic Gov event journal", err)
				}
			} else if _, err := r.RecordLegacyRetirementGPURelease(ctx, request.Authority, state.OwnerRevision, legacyPriorGPUReceipt()); err == nil {
				t.Fatal("GPU prior event widened to approved CPU executions")
			}
			reloaded, err := lifecycle.NewLegacyRetirement(open()).GetRuntime(ctx, a.TenantID, a.ExecutionID)
			if err != nil || !reflect.DeepEqual(reloaded.LegacyRetirement, state.LegacyRetirement) || reloaded.GPURelease != nil || reloaded.ComputeCleanup.LogsReady || len(reloaded.ComputeCleanup.Logs) != 0 {
				t.Fatal("reconnect lost explicit retirement journal", err)
			}
			readExecution, readState, readDispatch, readOwner, fence, err := lifecycle.NewLegacyRetirement(open()).GetLegacyRetirementRecord(ctx, a.TenantID, a.ExecutionID)
			if err != nil || readExecution.ExecutionID != a.ExecutionID || readState.OwnerRevision != reloaded.OwnerRevision || len(readDispatch.ConfirmedRuns) != 1 || readOwner != request.Authority || fence != request.CommandFence {
				t.Fatal("read-only restart lost one coherent original identity", err)
			}
			t.Log("LEGACY_RETIREMENT_FLOW PASS: archive/unrecoverable journal -> original CLOSED -> full step UID cleanup -> original compute attempts <=3 -> original generic Gov event/ALREADY_APPLIED independent journal; MD scope/outbox remain NULL; captured remains false")
		})
	}
}

func TestLegacyRetirementPostgresRejectsMultipleOriginalAttemptRuns(t *testing.T) {
	open, a, request := legacyRetirementFixture(t, "1cce950b-d6e9-453e-aa11-fffd8846224b", false)
	ctx := context.Background()
	dispatch, err := submission.New(open()).Get(ctx, a.TenantID, a.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lifecycle.New(open()).RecordClosingRunAmbiguity(ctx, dispatch, []string{request.Authority.RunID, uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	state, err := lifecycle.New(open()).GetRuntime(ctx, a.TenantID, a.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedOwnerRevision = state.OwnerRevision
	r := lifecycle.NewLegacyRetirement(open())
	if err = r.ValidateLegacyRetirement(ctx, request); err == nil {
		t.Fatal("multiple Runs passed read-only qualification")
	}
	if _, err = r.ConfirmLegacyRetirement(ctx, request); err == nil {
		t.Fatal("multiple Runs passed original locked qualification")
	}
	state, err = lifecycle.New(open()).GetRuntime(ctx, a.TenantID, a.ExecutionID)
	if err != nil || state.ClosedAt != nil || state.LegacyRetirement != nil || state.CloseReviewReason != "MULTIPLE_RUNS" {
		t.Fatal("ambiguity erased", err)
	}
}

func legacyPriorGPUReceipt() biz.LegacyRetirementPriorGPURelease {
	event := biz.LegacyRetirementOriginalGPUEvent()
	accounts := []biz.LegacyRetirementAccountInvariant{{AccountID: 2, QuotaCode: "gpu.physical.count"}, {AccountID: 6, QuotaCode: "gpu.shared_memory_mib"}}
	return biz.LegacyRetirementPriorGPURelease{ReceiptID: "23600748-d80d-5ceb-be35-664a995bb7db", PayloadSHA256: "3e63b8ddc427e1228169804c6a6d28c34899318224257119b0c788b522ca7cad", Event: event, ACK: biz.GPUReleaseACK{ReleaseEventID: event.ReleaseEventID, CreateOperationID: event.CreateOperationID, ResourceID: event.ResourceID, Status: "ALREADY_APPLIED"}, ChargeID: "1045b409-962a-4211-aee7-30319bffe459", QuotaCode: "gpu.shared_memory_mib", OriginalUnits: 24570, ReleasedUnits: 24570, BeforeAccounts: accounts, AfterAccounts: append([]biz.LegacyRetirementAccountInvariant{}, accounts...), BeforeEvidenceSHA256: strings.Repeat("a", 64), AfterEvidenceSHA256: strings.Repeat("a", 64), ObservedAt: time.Now().UTC().Truncate(time.Microsecond)}
}
