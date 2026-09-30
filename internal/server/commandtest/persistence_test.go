package commandtest

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGovernanceCommandCommitFailureHasNoACKAndRetryCommitsFirst(t *testing.T) {
	openPool := postgres.Prepare(t)
	writer := openPool()
	removeFault := rejectCommandCommit(t, writer)
	certificates := commandtls.New(t)
	client, stop := startCommandServer(t, execution.New(writer), certificates)
	request := validCloseRequest()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	delivery := commandContext(ctx, request)
	response, err := client.ApplyCloseIntent(delivery, request)
	if response != nil || status.Code(err) != codes.Unavailable {
		t.Fatalf("real COMMIT failure returned code %s, ACK=%t", status.Code(err), response != nil)
	}
	md, _ := metadata.FromOutgoingContext(delivery)
	failure := status.Convert(err)
	if failure.Message() != "command persistence unavailable" {
		t.Error("persistence failure did not preserve the safe transport message")
	}
	details := failure.Details()
	if len(details) != 1 {
		t.Fatal("persistence failure omitted its typed safe error detail")
	}
	detail, ok := details[0].(*modeldevv1.ErrorDetail)
	if !ok || detail.Reason != modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE || detail.CorrelationId != md.Get("x-ani-request-id")[0] || detail.SafeMessage != failure.Message() {
		t.Error("persistence failure lost the safe reason or original delivery correlation")
	}
	stop()
	writer.Close()
	reader := execution.New(openPool())
	assertNoCommandFacts(t, reader, request)
	removeFault()
	restarted, _ := startCommandServer(t, execution.New(openPool()), certificates)
	first, err := restarted.ApplyCloseIntent(commandContext(ctx, request), request)
	if err != nil { t.Fatalf("original delivery could not retry after rollback and restart: %v", err) }
	assertCloseResponse(t, first, request, false)
	stored, err := reader.GetCloseIntent(ctx, request.ResourceTenantId, request.Identity.ExecutionId)
	if err != nil || stored.Generation != 1 || stored.SourceGeneration != request.IntentGeneration {
		t.Fatalf("retry ACK did not reflect the first committed close: %v", err)
	}
	replay, err := restarted.ApplyCloseIntent(commandContext(ctx, request), request)
	if err != nil { t.Fatalf("committed retry did not replay: %v", err) }
	assertCloseResponse(t, replay, request, true)
}

// Only the test's exclusive migrated schema is changed. The real INSERT runs;
// a deferred PostgreSQL constraint rejects COMMIT, without replacing the port.
func rejectCommandCommit(t *testing.T, runtimePool *pgxpool.Pool) func() {
	t.Helper()
	runtimeConfig := runtimePool.Config().ConnConfig
	schemaName := runtimeConfig.RuntimeParams["search_path"]
	if !strings.HasPrefix(schemaName, "cpu04_") {
		t.Fatal("CPU_COMMAND_DB_PREFLIGHT: commit fault requires the exclusive test schema; behavior NOT_RUN")
	}
	adminConfig, err := pgx.ParseConfig(os.Getenv("CPU_P01_TEST_DATABASE_ADMIN_URL"))
	if err != nil || adminConfig.Host != runtimeConfig.Host || adminConfig.Port != runtimeConfig.Port || adminConfig.Database != runtimeConfig.Database || adminConfig.User == runtimeConfig.User {
		t.Fatal("CPU_COMMAND_DB_PREFLIGHT: isolated migration role required; behavior NOT_RUN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil { t.Fatal("CPU_COMMAND_DB_PREFLIGHT: migration connection failed; behavior NOT_RUN") }
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = admin.Close(ctx)
	})
	table := pgx.Identifier{schemaName, "modeldev_close_intents"}.Sanitize()
	function := pgx.Identifier{schemaName, "reject_command_commit"}.Sanitize()
	removed := false
	remove := func() {
		t.Helper()
		if removed { return }
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP TRIGGER IF EXISTS reject_command_commit ON "+table); err != nil {
			t.Error("CPU_COMMAND_DB_CLEANUP: isolated trigger removal failed")
			return
		}
		if _, err := admin.Exec(ctx, "DROP FUNCTION IF EXISTS "+function+"()"); err != nil {
			t.Error("CPU_COMMAND_DB_CLEANUP: isolated function removal failed")
			return
		}
		removed = true
	}
	t.Cleanup(remove)
	if _, err := admin.Exec(ctx, "CREATE FUNCTION "+function+"() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected_command_commit_failure' USING ERRCODE = '23514'; END; $$"); err != nil {
		t.Fatal("CPU_COMMAND_DB_PREFLIGHT: isolated commit-fault function failed; behavior NOT_RUN")
	}
	if _, err := admin.Exec(ctx, "CREATE CONSTRAINT TRIGGER reject_command_commit AFTER INSERT ON "+table+" DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION "+function+"()"); err != nil {
		t.Fatal("CPU_COMMAND_DB_PREFLIGHT: isolated deferred trigger failed; behavior NOT_RUN")
	}
	return remove
}
