package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
)

func TestUserStopBeforeAdmissionPersistsClosingTombstoneAcrossNewConnections(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	writerPool := openRuntimePool()
	writer := execution.New(writerPool)
	admission := validAdmission(t)
	intent := biz.CloseIntent{
		TenantID:         admission.TenantID,
		OperationID:      admission.OperationID,
		ExecutionID:      admission.ExecutionID,
		SpecHash:         admission.SpecHash,
		SourceGeneration: 41,
		Reason:           biz.CloseReasonUserStop,
		RequestedAt:      admission.AcceptedAt.Add(time.Minute),
		RequestedActor:   "governance:user:42",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	absent, err := writer.Get(ctx, admission.TenantID, admission.ExecutionID)
	assertEmptyFailure(t, absent, err, biz.ErrExecutionNotFound)
	receipt, err := writer.ApplyCloseIntent(ctx, intent)
	if err != nil {
		t.Fatalf("USER_STOP before Admission must commit a closing tombstone: %v", err)
	}
	assertInitialCloseTombstone(t, receipt, intent)
	writerPool.Close()

	reader := execution.New(openRuntimePool())
	readContext, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()
	stored, err := reader.GetCloseIntent(readContext, intent.TenantID, intent.ExecutionID)
	if err != nil {
		t.Fatalf("GetCloseIntent after reconnect: %v", err)
	}
	assertInitialCloseTombstone(t, stored, intent)
	// A close tombstone cannot invent the missing immutable Admission. Actual
	// late-Admission fencing and runtime creation will be separate behaviors.
	absent, err = reader.Get(readContext, admission.TenantID, admission.ExecutionID)
	assertEmptyFailure(t, absent, err, biz.ErrExecutionNotFound)
}

func TestUserStopDuplicateAfterReconnectReturnsOriginalFenceAndFacts(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	writerPool := openRuntimePool()
	admission := validAdmission(t)
	intent := biz.CloseIntent{
		TenantID:         admission.TenantID,
		OperationID:      admission.OperationID,
		ExecutionID:      admission.ExecutionID,
		SpecHash:         admission.SpecHash,
		SourceGeneration: 41,
		Reason:           biz.CloseReasonUserStop,
		RequestedAt:      admission.AcceptedAt.Add(time.Minute),
		RequestedActor:   "governance:user:42",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, err := execution.New(writerPool).ApplyCloseIntent(ctx, intent)
	if err != nil {
		t.Fatalf("initial USER_STOP: %v", err)
	}
	assertInitialCloseTombstone(t, first, intent)
	writerPool.Close()

	repository := execution.New(openRuntimePool())
	replayContext, replayCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer replayCancel()
	replayed, err := repository.ApplyCloseIntent(replayContext, intent)
	if err != nil {
		t.Fatalf("exact USER_STOP replay must return the original receipt: %v", err)
	}
	assertInitialCloseTombstone(t, replayed, intent)
	stored, err := repository.GetCloseIntent(replayContext, intent.TenantID, intent.ExecutionID)
	if err != nil {
		t.Fatalf("GetCloseIntent after replay: %v", err)
	}
	assertInitialCloseTombstone(t, stored, intent)
	absent, err := repository.Get(replayContext, intent.TenantID, intent.ExecutionID)
	assertEmptyFailure(t, absent, err, biz.ErrExecutionNotFound)
}

func assertInitialCloseTombstone(t *testing.T, got biz.CloseRecord, want biz.CloseIntent) {
	t.Helper()
	if got.TenantID != want.TenantID || got.OperationID != want.OperationID || got.ExecutionID != want.ExecutionID || got.SpecHash != want.SpecHash {
		t.Fatal("close tombstone lost its original tenant/operation/execution/spec binding")
	}
	if got.SourceGeneration != want.SourceGeneration || got.Reason != want.Reason || got.RequestedActor != want.RequestedActor || !got.RequestedAt.Equal(want.RequestedAt) {
		t.Fatal("close tombstone changed the original source intent or audit facts")
	}
	if got.Generation != 1 {
		t.Fatalf("first ModelDev fence generation = %d, want 1 independently of source generation %d", got.Generation, want.SourceGeneration)
	}
	if got.State != biz.CloseStateClosing {
		t.Fatalf("durable stop is only CLOSING without writer reconciliation, got %s", got.State)
	}
}
