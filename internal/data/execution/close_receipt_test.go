package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestUserStopFailureAtCommitReturnsNoReceiptAndRetryIsFirstCommit(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	writerPool := openRuntimePool()
	repository := execution.New(writerPool)
	removeCommitFailure := postgres.RejectCloseCommit(t, writerPool)
	intent := userStopIntent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	failed, err := repository.ApplyCloseIntent(ctx, intent)
	assertEmptyCloseReceiptFailure(t, failed, err, biz.ErrPersistence)
	writerPool.Close()
	reconnected := execution.New(openRuntimePool())
	absent, err := reconnected.GetCloseIntent(ctx, intent.TenantID, intent.ExecutionID)
	assertEmptyCloseFailure(t, absent, err, biz.ErrExecutionNotFound)

	removeCommitFailure()
	first, err := reconnected.ApplyCloseIntent(ctx, intent)
	if err != nil {
		t.Fatalf("the original command must remain retryable after a failed commit: %v", err)
	}
	assertInitialCloseTombstone(t, first.CloseRecord, intent)
	if first.Replayed {
		t.Fatal("the failed transaction must not consume the first committed receipt")
	}
	stored, err := execution.New(openRuntimePool()).GetCloseIntent(ctx, intent.TenantID, intent.ExecutionID)
	if err != nil {
		t.Fatalf("the successful retry must be independently readable: %v", err)
	}
	assertInitialCloseTombstone(t, stored, intent)
	replayed, err := execution.New(openRuntimePool()).ApplyCloseIntent(ctx, intent)
	if err != nil || !replayed.Replayed {
		t.Fatalf("only the retry committed; its next delivery must be a replay: %v", err)
	}
	assertInitialCloseTombstone(t, replayed.CloseRecord, intent)
}
