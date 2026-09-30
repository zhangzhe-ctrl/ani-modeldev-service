package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
)

func TestLateAdmissionCarriesCommittedCloseFactsAcrossNewConnections(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	pool := openRuntimePool()
	repository := execution.New(pool)
	admission, stop := validAdmission(t), userStopIntent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := repository.ApplyCloseIntent(ctx, stop); err != nil {
		t.Fatalf("early USER_STOP: %v", err)
	}
	accepted, err := repository.Accept(ctx, admission)
	if err != nil {
		t.Fatalf("matching late Admission must retain its immutable facts: %v", err)
	}
	assertOriginalAdmission(t, accepted, admission)
	assertExecutionClose(t, accepted, stop)
	pool.Close()

	repository = execution.New(openRuntimePool())
	stored, err := repository.Get(ctx, admission.TenantID, admission.ExecutionID)
	if err != nil {
		t.Fatalf("Get late Admission after reconnect: %v", err)
	}
	assertOriginalAdmission(t, stored, admission)
	assertExecutionClose(t, stored, stop)
	replayed, err := repository.Accept(ctx, admission)
	if err != nil {
		t.Fatalf("late Admission replay: %v", err)
	}
	assertOriginalAdmission(t, replayed, admission)
	assertExecutionClose(t, replayed, stop)
}

func TestConcurrentAdmissionAndUserStopConvergeToClosingFacts(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	admissions := execution.New(openRuntimePool())
	closes := execution.New(openRuntimePool())
	reader := execution.New(openRuntimePool())
	admission, stop := validAdmission(t), userStopIntent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	admissionDone := make(chan admissionOutcome, 1)
	closeDone := make(chan closeOutcome, 1)
	go func() {
		<-start
		record, err := admissions.Accept(ctx, admission)
		admissionDone <- admissionOutcome{execution: record, err: err}
	}()
	go func() {
		<-start
		record, err := closes.ApplyCloseIntent(ctx, stop)
		closeDone <- closeOutcome{record: record, err: err}
	}()
	close(start)
	// Both calls must finish before the final read; ordering is established by
	// completion channels rather than sleeps or assumed database scheduling.
	var admitted admissionOutcome
	select {
	case admitted = <-admissionDone:
	case <-ctx.Done():
		t.Fatal("concurrent Admission did not finish within its bounded context")
	}
	var closed closeOutcome
	select {
	case closed = <-closeDone:
	case <-ctx.Done():
		t.Fatal("concurrent USER_STOP did not finish within its bounded context")
	}
	if admitted.err != nil || closed.err != nil {
		t.Fatalf("both matching facts must commit: admission=%v close=%v", admitted.err, closed.err)
	}
	assertOriginalAdmission(t, admitted.execution, admission)
	assertInitialCloseTombstone(t, closed.record.CloseRecord, stop)
	if admitted.execution.Close != nil {
		assertExecutionClose(t, admitted.execution, stop)
	}
	stored, err := reader.Get(ctx, admission.TenantID, admission.ExecutionID)
	if err != nil {
		t.Fatalf("Get after both concurrent commits: %v", err)
	}
	assertOriginalAdmission(t, stored, admission)
	assertExecutionClose(t, stored, stop)
}

func TestLateAdmissionCannotTakeOverTombstoneIdentityOrSpec(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*biz.Admission)
	}{
		{"operation", func(a *biz.Admission) { a.OperationID = otherOperation }},
		{"execution", func(a *biz.Admission) { a.ExecutionID = otherExecution }},
		{"tenant", func(a *biz.Admission) { a.TenantID = otherTenant }},
		{"snapshot", func(a *biz.Admission) { a.Snapshot.DeadlineAt = a.Snapshot.DeadlineAt.Add(time.Minute) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repository := execution.New(preparePostgreSQL(t)())
			stop := userStopIntent(t)
			candidate := validAdmission(t)
			testCase.mutate(&candidate)
			refreshAdmissionHashes(t, &candidate)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := repository.ApplyCloseIntent(ctx, stop); err != nil {
				t.Fatalf("early USER_STOP: %v", err)
			}
			got, err := repository.Accept(ctx, candidate)
			assertEmptyFailure(t, got, err, biz.ErrAdmissionConflict)
			stored, err := repository.GetCloseIntent(ctx, stop.TenantID, stop.ExecutionID)
			if err != nil {
				t.Fatalf("Get original tombstone after conflict: %v", err)
			}
			assertInitialCloseTombstone(t, stored, stop)
			absent, err := repository.Get(ctx, candidate.TenantID, candidate.ExecutionID)
			assertEmptyFailure(t, absent, err, biz.ErrExecutionNotFound)
		})
	}
}

func assertExecutionClose(t *testing.T, got biz.Execution, want biz.CloseIntent) {
	t.Helper()
	if got.Close == nil {
		t.Fatal("Admission must carry the durable closing facts instead of appearing open")
	}
	assertInitialCloseTombstone(t, *got.Close, want)
}
