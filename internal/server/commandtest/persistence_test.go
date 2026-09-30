package commandtest

import (
	"context"
	"testing"
	"time"

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
	removeFault := postgres.RejectCloseCommit(t, writer)
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
	if err != nil {
		t.Fatalf("original delivery could not retry after rollback and restart: %v", err)
	}
	assertCloseResponse(t, first, request, false)
	stored, err := reader.GetCloseIntent(ctx, request.ResourceTenantId, request.Identity.ExecutionId)
	if err != nil || stored.Generation != 1 || stored.SourceGeneration != request.IntentGeneration {
		t.Fatalf("retry ACK did not reflect the first committed close: %v", err)
	}
	replay, err := restarted.ApplyCloseIntent(commandContext(ctx, request), request)
	if err != nil {
		t.Fatalf("committed retry did not replay: %v", err)
	}
	assertCloseResponse(t, replay, request, true)
}
