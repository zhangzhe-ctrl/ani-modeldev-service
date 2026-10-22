package lifecycle_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
)

func trainingLogEvidenceFixture(t *testing.T) (func() *pgxpool.Pool, biz.RunAuthorityCandidate, biz.TrainingRuntimeObservation, biz.TrainingLogSource) {
	t.Helper()
	open, admission, authority, workspace := runtimeFixture(t)
	ctx := context.Background()
	repository := lifecycle.New(open())
	if _, _, err := repository.RecordPrepared(ctx, authority, workspace); err != nil {
		t.Fatal(err)
	}
	reserved, err := reserveTrainingV21(t, repository, ctx, authority)
	if err != nil || !reserved.SendPermit {
		t.Fatalf("original training intent: %v", err)
	}
	handle := biz.TrainingHandle{NamespaceUID: workspace.NamespaceUID, TrainJobUID: "log-training-original", PVCUID: workspace.PVCUID}
	if _, err := repository.RecordTrainingHandle(ctx, authority, handle); err != nil {
		t.Fatal(err)
	}
	zero := int32(0)
	observation := biz.TrainingRuntimeObservation{Handle: handle, Outcome: "SUCCEEDED", WritersAbsent: true, ObservedAt: time.Now().UTC().Truncate(time.Microsecond), Resources: []biz.RuntimeResource{
		{APIVersion: "trainer.kubeflow.org/v1alpha1", Kind: "TrainJob", Namespace: workspace.NamespaceName, Name: reserved.State.Training.Name, UID: handle.TrainJobUID, APIObjectPresent: true, Terminal: true},
		{APIVersion: "jobset.x-k8s.io/v1alpha2", Kind: "JobSet", Namespace: workspace.NamespaceName, Name: "log-set-original", UID: "log-set-original", OwnerUID: handle.TrainJobUID, APIObjectPresent: true, Terminal: true},
		{APIVersion: "batch/v1", Kind: "Job", Namespace: workspace.NamespaceName, Name: "log-job-original", UID: "log-job-original", OwnerUID: "log-set-original", APIObjectPresent: true, Terminal: true},
		{APIVersion: "v1", Kind: "Pod", Namespace: workspace.NamespaceName, Name: "log-pod-original", UID: "log-pod-original", OwnerUID: "log-job-original", APIObjectPresent: true, Terminal: true, ExitCode: &zero},
	}}
	state, err := repository.GetRuntime(ctx, authority.TenantID, authority.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	state.Observation = &observation
	source, err := biz.TrainingLogSourceFor(biz.QueryRecord{Execution: biz.Execution{Admission: admission}, Runtime: state}, "")
	if err != nil {
		t.Fatal("original complete training resource chain", err)
	}
	return open, authority, observation, source
}

func originalTrainingLog(source biz.TrainingLogSource, observed time.Time) biz.RecordedTrainingLog {
	return biz.RecordedTrainingLog{Source: source, Result: biz.TrainingLogResult{ObservedAt: observed, Lines: []biz.TrainingLogLine{{Timestamp: observed.Add(-time.Second), Text: "cuda original finite update"}}}}
}

func TestTrainingObservationPersistsOriginalTerminalLogsAcrossReconnect(t *testing.T) {
	open, authority, observation, source := trainingLogEvidenceFixture(t)
	ctx := context.Background()
	repository := lifecycle.New(open())
	// A reader failure must not prevent the independent original exit fact.
	if _, err := repository.RecordTrainingObservation(ctx, authority, observation); err != nil {
		t.Fatal("terminal exit without available logs", err)
	}
	visible, err := lifecycle.New(open()).GetRuntime(ctx, authority.TenantID, authority.ExecutionID)
	if err != nil || visible.Observation == nil || visible.Observation.Outcome != "SUCCEEDED" || !visible.Observation.WritersAbsent || len(visible.Observation.RecordedLogs) != 0 {
		t.Fatalf("exit without a log read was not durable: %+v; %v", visible.Observation, err)
	}
	originalExit := false
	for _, resource := range visible.Observation.Resources {
		if resource.UID == source.PodUID && resource.Terminal && resource.ExitCode != nil && *resource.ExitCode == 0 {
			originalExit = true
		}
	}
	if !originalExit {
		t.Fatal("original Pod UID/terminal exit lost across reconnect")
	}
	observation.ObservedAt = observation.ObservedAt.Add(time.Microsecond)
	original := originalTrainingLog(source, observation.ObservedAt)
	observation.RecordedLogs = []biz.RecordedTrainingLog{original}
	if _, err := repository.RecordTrainingObservation(ctx, authority, observation); err != nil {
		t.Fatal("later successful original bounded log capture", err)
	}
	visible, err = lifecycle.New(open()).GetRuntime(ctx, authority.TenantID, authority.ExecutionID)
	if err != nil || visible.Observation == nil || !reflect.DeepEqual(visible.Observation.RecordedLogs, []biz.RecordedTrainingLog{original}) {
		t.Fatalf("ORIGINAL_TRAINING_LOG_NOT_PERSISTED: %+v; %v", visible.Observation, err)
	}
	observation.ObservedAt = observation.ObservedAt.Add(time.Microsecond)
	observation.RecordedLogs = nil
	if _, err := repository.RecordTrainingObservation(ctx, authority, observation); err != nil {
		t.Fatal("repeat terminal observation without a new log read", err)
	}
	visible, err = lifecycle.New(open()).GetRuntime(ctx, authority.TenantID, authority.ExecutionID)
	if err != nil || visible.Observation == nil || !reflect.DeepEqual(visible.Observation.RecordedLogs, []biz.RecordedTrainingLog{original}) {
		t.Fatalf("REPEATED_OBSERVATION_DROPPED_ORIGINAL_LOG: %+v; %v", visible.Observation, err)
	}
	observation.ObservedAt = observation.ObservedAt.Add(time.Microsecond)
	changed := originalTrainingLog(source, observation.ObservedAt)
	changed.Result.Lines[0].Text = "replacement content must not overwrite the original"
	observation.RecordedLogs = []biz.RecordedTrainingLog{changed}
	_, _ = repository.RecordTrainingObservation(ctx, authority, observation)
	visible, err = lifecycle.New(open()).GetRuntime(ctx, authority.TenantID, authority.ExecutionID)
	if err != nil || visible.Observation == nil || !reflect.DeepEqual(visible.Observation.RecordedLogs, []biz.RecordedTrainingLog{original}) {
		t.Fatalf("REPEATED_CAPTURE_REPLACED_ORIGINAL_LOG: %+v; %v", visible.Observation, err)
	}
}

func TestTrainingObservationRejectsInvalidLogEvidenceAtomically(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*biz.TrainingRuntimeObservation)
	}{
		{"replacement Pod UID", func(o *biz.TrainingRuntimeObservation) { o.RecordedLogs[0].Source.PodUID = "same-name-replacement-uid" }},
		{"foreign source Job UID", func(o *biz.TrainingRuntimeObservation) { o.RecordedLogs[0].Source.OwnerUID = "foreign-job-uid" }},
		{"foreign namespace UID", func(o *biz.TrainingRuntimeObservation) {
			o.RecordedLogs[0].Source.NamespaceUID = "foreign-namespace-uid"
		}},
		{"foreign log ID", func(o *biz.TrainingRuntimeObservation) {
			o.RecordedLogs[0].Source.LogID = "5f3f767d-590a-49bd-850d-5c3595781173"
		}},
		{"over 1000 lines", func(o *biz.TrainingRuntimeObservation) {
			line := o.RecordedLogs[0].Result.Lines[0]
			o.RecordedLogs[0].Result.Lines = make([]biz.TrainingLogLine, 1001)
			for i := range o.RecordedLogs[0].Result.Lines {
				o.RecordedLogs[0].Result.Lines[i] = line
			}
		}},
		{"over 65536 wire bytes", func(o *biz.TrainingRuntimeObservation) {
			o.RecordedLogs[0].Result.Lines[0].Text = strings.Repeat("x", 65536)
		}},
		{"nonterminal original Pod", func(o *biz.TrainingRuntimeObservation) {
			o.Outcome, o.WritersAbsent = "RUNNING", false
			for i := range o.Resources {
				if o.Resources[i].Kind == "Pod" {
					o.Resources[i].Terminal, o.Resources[i].ExitCode = false, nil
				}
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			open, authority, observation, source := trainingLogEvidenceFixture(t)
			ctx := context.Background()
			repository := lifecycle.New(open())
			running := observation
			running.Resources = append([]biz.RuntimeResource(nil), observation.Resources...)
			running.Outcome, running.WritersAbsent = "RUNNING", false
			for i := range running.Resources {
				running.Resources[i].Terminal, running.Resources[i].ExitCode = false, nil
			}
			if _, err := repository.RecordTrainingObservation(ctx, authority, running); err != nil {
				t.Fatal("original RUNNING observation", err)
			}
			before, err := lifecycle.New(open()).GetRuntime(ctx, authority.TenantID, authority.ExecutionID)
			if err != nil {
				t.Fatal(err)
			}
			observation.ObservedAt = observation.ObservedAt.Add(time.Microsecond)
			observation.RecordedLogs = []biz.RecordedTrainingLog{originalTrainingLog(source, observation.ObservedAt)}
			test.change(&observation)
			_, writeErr := repository.RecordTrainingObservation(ctx, authority, observation)
			after, readErr := lifecycle.New(open()).GetRuntime(ctx, authority.TenantID, authority.ExecutionID)
			if writeErr == nil {
				t.Error("INVALID_TRAINING_LOG_EVIDENCE_ACCEPTED")
			}
			if readErr != nil || after.OwnerRevision != before.OwnerRevision || !reflect.DeepEqual(after.Observation, before.Observation) {
				t.Fatalf("INVALID_LOG_PARTIALLY_COMMITTED_TERMINAL_OR_LOG_FACTS: beforeRevision=%d afterRevision=%d read=%v", before.OwnerRevision, after.OwnerRevision, readErr)
			}
		})
	}
}

type trainingLogStatusBoundary struct {
	*artifactsOnlyExternalBoundary
	open        func() *pgxpool.Pool
	observation biz.TrainingRuntimeObservation
	original    biz.RecordedTrainingLog
	readErr     error
	reads       int
	releases    int
}

func (b *trainingLogStatusBoundary) ObserveTraining(_ context.Context, plan biz.TrainingPlan, handle biz.TrainingHandle, _ []biz.RuntimeResource) (biz.TrainingRuntimeObservation, error) {
	if plan.ExecutionID != b.authority.ExecutionID || handle != b.observation.Handle {
		b.t.Fatal("status observation changed the original training identity")
	}
	return b.observation, nil
}

func (b *trainingLogStatusBoundary) ReadTrainingLogs(_ context.Context, source biz.TrainingLogSource, options biz.TrainingLogOptions) (biz.TrainingLogResult, error) {
	b.reads++
	if source != b.original.Source || options.TailLines != 1000 || options.MaxBytes != 65536 {
		b.t.Fatal("status log read escaped the original source or bounded tail")
	}
	return b.original.Result, b.readErr
}

func (b *trainingLogStatusBoundary) ReleaseRecordedTrainingExits(ctx context.Context, plan biz.TrainingPlan, handle biz.TrainingHandle, observation biz.TrainingRuntimeObservation) error {
	b.releases++
	stored, err := lifecycle.New(b.open()).GetRuntime(ctx, b.authority.TenantID, b.authority.ExecutionID)
	if err != nil || plan.ExecutionID != b.authority.ExecutionID || handle != b.observation.Handle || stored.Observation == nil || !reflect.DeepEqual(*stored.Observation, observation) || !reflect.DeepEqual(stored.Observation.RecordedLogs, []biz.RecordedTrainingLog{b.original}) {
		b.t.Fatalf("EXIT_RELEASE_BEFORE_REAL_PG_LOG_COMMIT: releases=%d read=%v", b.releases, err)
	}
	for _, resource := range stored.Observation.Resources {
		if resource.UID == b.original.Source.PodUID && resource.Terminal && resource.ExitCode != nil && *resource.ExitCode == 0 {
			return nil
		}
	}
	b.t.Fatal("exit release lacked original Pod UID/terminal exit in independent PG read")
	return biz.ErrTrainingUncertain
}

func TestManagedStatusCommitsTrainingLogsBeforeExitRelease(t *testing.T) {
	for _, failRead := range []bool{false, true} {
		name := "successful final capture"
		if failRead {
			name = "failed reader retains source and original exit"
		}
		t.Run(name, func(t *testing.T) {
			open, authority, observation, source := trainingLogEvidenceFixture(t)
			ctx := context.Background()
			pool := open()
			claim := biz.BeginManagedExecutionRequest{TenantID: authority.TenantID, ExecutionID: authority.ExecutionID, OperationID: authority.OperationID, SpecHash: authority.SpecHash,
				Association: biz.ManagedStepAssociation{RunID: authority.RunID, NamespaceName: authority.NamespaceName, NamespaceUID: authority.NamespaceUID, WorkflowName: authority.WorkflowName, WorkflowUID: authority.WorkflowUID, PodName: "train-wait-fixture", PodUID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("train-wait")).String()}}
			external := &trainingLogStatusBoundary{artifactsOnlyExternalBoundary: &artifactsOnlyExternalBoundary{t: t, authority: authority, association: claim.Association, role: "train-wait"}, open: open, observation: observation, original: originalTrainingLog(source, observation.ObservedAt)}
			if failRead {
				external.readErr = errors.New("original terminal log reader unavailable")
			}
			steps, err := biz.NewManagedSteps(submission.New(pool), execution.New(pool), external, external)
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := biz.NewManagedRuntime(steps, lifecycle.New(pool), external, external, external)
			if err != nil {
				t.Fatal(err)
			}
			_, statusErr := runtime.WithTrainingLogs(external).Status(ctx, "synthetic-current-bound-token", claim)
			stored, readErr := lifecycle.New(open()).GetRuntime(ctx, authority.TenantID, authority.ExecutionID)
			if readErr != nil || stored.Observation == nil || external.workloadChecks != 1 || external.runChecks != 1 {
				t.Fatalf("public Status did not persist the authenticated original observation: read=%v status=%v", readErr, statusErr)
			}
			originalExit := false
			for _, resource := range stored.Observation.Resources {
				if resource.UID == source.PodUID && resource.Terminal && resource.ExitCode != nil && *resource.ExitCode == 0 {
					originalExit = true
				}
			}
			if !originalExit {
				t.Fatal("public Status lost the independent terminal Pod exit")
			}
			if failRead {
				if !errors.Is(statusErr, external.readErr) || external.reads != 1 || external.releases != 0 || stored.Observation.WritersAbsent || len(stored.Observation.RecordedLogs) != 0 {
					t.Fatalf("FAILED_LOG_READ_RELEASED_SOURCE_OR_LOST_EXIT_FENCE: status=%v reads=%d releases=%d writersAbsent=%t", statusErr, external.reads, external.releases, stored.Observation.WritersAbsent)
				}
			} else if statusErr != nil || external.reads != 1 || external.releases != 1 || !reflect.DeepEqual(stored.Observation.RecordedLogs, []biz.RecordedTrainingLog{external.original}) {
				t.Fatalf("STATUS_DID_NOT_COMMIT_LOG_BEFORE_RELEASE: status=%v reads=%d releases=%d logs=%d", statusErr, external.reads, external.releases, len(stored.Observation.RecordedLogs))
			}
		})
	}
}
