package execution_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
)

func TestUserStopFailureAtCommitReturnsNoReceiptAndRetryIsFirstCommit(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	writerPool := openRuntimePool()
	repository := execution.New(writerPool)
	removeCommitFailure := rejectCloseAtCommit(t, writerPool)
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

// The fault belongs only to this test's randomly allocated schema. A deferred
// constraint trigger lets the real INSERT succeed and rejects the real COMMIT;
// neither the repository nor its transaction is replaced by a test double.
func rejectCloseAtCommit(t *testing.T, runtimePool *pgxpool.Pool) func() {
	t.Helper()
	runtimeConfig := runtimePool.Config().ConnConfig
	schemaName := runtimeConfig.RuntimeParams["search_path"]
	if !strings.HasPrefix(schemaName, "cpu04_") {
		t.Fatal("CPU04_DB_PREFLIGHT: commit fault requires this test's isolated schema; behavior NOT_RUN")
	}
	adminConfig, err := pgx.ParseConfig(os.Getenv("CPU_P01_TEST_DATABASE_ADMIN_URL"))
	if err != nil || adminConfig.Host != runtimeConfig.Host || adminConfig.Port != runtimeConfig.Port || adminConfig.Database != runtimeConfig.Database || adminConfig.User == runtimeConfig.User {
		t.Fatal("CPU04_DB_PREFLIGHT: commit fault requires the isolated migration role; behavior NOT_RUN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: commit-fault migration connection failed; behavior NOT_RUN")
	}
	t.Cleanup(func() {
		closeContext, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = admin.Close(closeContext)
	})
	table := pgx.Identifier{schemaName, "modeldev_close_intents"}.Sanitize()
	function := pgx.Identifier{schemaName, "reject_close_receipt_commit"}.Sanitize()
	removed := false
	remove := func() {
		t.Helper()
		if removed {
			return
		}
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupContext, "DROP TRIGGER IF EXISTS reject_close_receipt_commit ON "+table); err != nil {
			t.Error("CPU04_DB_CLEANUP: isolated commit-fault trigger removal failed")
			return
		}
		if _, err := admin.Exec(cleanupContext, "DROP FUNCTION IF EXISTS "+function+"()"); err != nil {
			t.Error("CPU04_DB_CLEANUP: isolated commit-fault function removal failed")
			return
		}
		removed = true
	}
	t.Cleanup(remove)
	if _, err := admin.Exec(ctx, "CREATE FUNCTION "+function+"() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected_close_commit_failure' USING ERRCODE = '23514'; END; $$"); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: isolated commit-fault function creation failed; behavior NOT_RUN")
	}
	if _, err := admin.Exec(ctx, "CREATE CONSTRAINT TRIGGER reject_close_receipt_commit AFTER INSERT ON "+table+" DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION "+function+"()"); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: isolated commit-fault trigger creation failed; behavior NOT_RUN")
	}
	return remove
}
