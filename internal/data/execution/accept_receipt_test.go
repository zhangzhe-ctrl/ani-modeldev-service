package execution_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
)

func TestAcceptReceiptConcurrentDeliveriesHaveOneFirstAcceptanceAndDurableReplay(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	firstPool, secondPool := openRuntimePool(), openRuntimePool()
	repositories := []*execution.Repository{execution.New(firstPool), execution.New(secondPool)}
	command := validAdmission(t)
	command.Actor = "governance:user:42"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	type outcome struct {
		receipt biz.AcceptReceipt
		err     error
	}
	start := make(chan struct{})
	results := make(chan outcome, len(repositories))
	for _, repository := range repositories {
		go func(repository *execution.Repository) {
			<-start
			receipt, err := repository.Accept(ctx, command)
			results <- outcome{receipt: receipt, err: err}
		}(repository)
	}
	close(start)
	firstAcceptances, replays := 0, 0
	var original biz.Execution
	for range repositories {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("matching concurrent Accept: %v", result.err)
			}
			assertOriginalAdmission(t, result.receipt.Execution, command)
			if result.receipt.Close != nil {
				t.Fatal("an admission receipt invented a close fact")
			}
			original = result.receipt.Execution
			if result.receipt.Replayed {
				replays++
			} else {
				firstAcceptances++
			}
		case <-ctx.Done():
			t.Fatal("concurrent Accept did not finish within its bounded context")
		}
	}
	if firstAcceptances != 1 || replays != 1 {
		t.Errorf("concurrent receipts reported first=%d replayed=%d, want exactly one of each", firstAcceptances, replays)
	}
	firstPool.Close()
	secondPool.Close()

	// The third pool must recover the same committed fact without retaining
	// either original connection or the delivery-specific replay receipt.
	reconnected := execution.New(openRuntimePool())
	stored, err := reconnected.Get(ctx, command.TenantID, command.ExecutionID)
	if err != nil {
		t.Fatalf("Get committed admission after reconnect: %v", err)
	}
	if !reflect.DeepEqual(stored, original) {
		t.Fatal("Get after reconnect changed the original execution fact")
	}
	replayed, err := reconnected.Accept(ctx, command)
	if err != nil {
		t.Fatalf("Accept replay after reconnect: %v", err)
	}
	if !replayed.Replayed {
		t.Error("a delivery after reconnect reported a new acceptance")
	}
	if !reflect.DeepEqual(replayed.Execution, stored) {
		t.Fatal("replayed receipt changed the original committed execution fact")
	}
}

func TestAcceptReceiptAfterEarlierCloseIsFirstAndPreservesCloseOnReplay(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	writerPool := openRuntimePool()
	repository := execution.New(writerPool)
	command, stop := validAdmission(t), userStopIntent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	closed, err := repository.ApplyCloseIntent(ctx, stop)
	if err != nil {
		t.Fatalf("commit earlier close fixture: %v", err)
	}
	assertInitialCloseTombstone(t, closed.CloseRecord, stop)
	first, err := repository.Accept(ctx, command)
	if err != nil || first.Replayed {
		t.Fatalf("the first Admission after an earlier close must report first acceptance: %v", err)
	}
	assertOriginalAdmission(t, first.Execution, command)
	if !reflect.DeepEqual(first.Close, &closed.CloseRecord) {
		t.Fatal("first admission receipt lost or changed the earlier close")
	}
	writerPool.Close()

	reconnected := execution.New(openRuntimePool())
	stored, err := reconnected.Get(ctx, command.TenantID, command.ExecutionID)
	if err != nil || !reflect.DeepEqual(stored, first.Execution) {
		t.Fatalf("reconnected Get changed the admitted execution or earlier close: %v", err)
	}
	replayed, err := reconnected.Accept(ctx, command)
	if err != nil || !replayed.Replayed || !reflect.DeepEqual(replayed.Execution, stored) {
		t.Fatalf("reconnected delivery must replay the original execution and close: %v", err)
	}
	persistedClose, err := reconnected.GetCloseIntent(ctx, stop.TenantID, stop.ExecutionID)
	if err != nil || !reflect.DeepEqual(persistedClose, closed.CloseRecord) {
		t.Fatalf("admission or replay changed the original independent close fact: %v", err)
	}
}

func TestAcceptReceiptConflictReturnsEntireZeroReceiptAndPreservesOriginal(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	repository := execution.New(openRuntimePool())
	command := validAdmission(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	first, err := repository.Accept(ctx, command)
	if err != nil || first.Replayed {
		t.Fatalf("initial Admission fixture must commit once: %v", err)
	}
	candidate := command
	candidate.Intent.Name = "conflicting-admission-receipt"
	refreshAdmissionHashes(t, &candidate)
	failed, err := repository.Accept(ctx, candidate)
	assertEmptyAcceptReceiptFailure(t, failed, err, biz.ErrAdmissionConflict)
	reconnected := execution.New(openRuntimePool())
	stored, err := reconnected.Get(ctx, command.TenantID, command.ExecutionID)
	if err != nil || !reflect.DeepEqual(stored, first.Execution) {
		t.Fatalf("a rejected conflict changed the original committed execution: %v", err)
	}
	replayed, err := reconnected.Accept(ctx, command)
	if err != nil || !replayed.Replayed || !reflect.DeepEqual(replayed.Execution, stored) {
		t.Fatalf("a rejected conflict changed the original command's replay: %v", err)
	}
}

func assertEmptyAcceptReceiptFailure(t *testing.T, got biz.AcceptReceipt, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || err.Error() != want.Error() {
		t.Errorf("want stable %s without storage/identity details, got %v", want, err)
	}
	if !reflect.DeepEqual(got, biz.AcceptReceipt{}) {
		t.Error("failed admission returned a nonzero command receipt")
	}
}
