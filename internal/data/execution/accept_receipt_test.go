package execution_test

import (
	"context"
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
