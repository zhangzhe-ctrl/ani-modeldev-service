package commandtest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/service"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestGovernanceCommandRejectsInvalidBodiesBeforePersistence(t *testing.T) {
	openPool := postgres.Prepare(t)
	client, _ := startCommandServer(t, execution.New(openPool()), commandtls.New(t))
	reader := execution.New(openPool())
	for _, testCase := range []struct {
		name string
		change func(*modeldevv1.ApplyCloseIntentRequest)
	}{
		{"missing identity", func(r *modeldevv1.ApplyCloseIntentRequest) { r.Identity = nil }},
		{"missing timestamp", func(r *modeldevv1.ApplyCloseIntentRequest) { r.RequestedAt = nil }},
		{"unknown command field", func(r *modeldevv1.ApplyCloseIntentRequest) { r.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01}) }},
		{"unknown identity field", func(r *modeldevv1.ApplyCloseIntentRequest) { r.Identity.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01}) }},
		{"unknown timestamp field", func(r *modeldevv1.ApplyCloseIntentRequest) { r.RequestedAt.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01}) }},
		{"unknown reason", func(r *modeldevv1.ApplyCloseIntentRequest) { r.Reason = modeldevv1.CloseReason(999) }},
		{"unspecified reason", func(r *modeldevv1.ApplyCloseIntentRequest) { r.Reason = modeldevv1.CloseReason_CLOSE_REASON_UNSPECIFIED }},
		{"deadline is not Governance user stop", func(r *modeldevv1.ApplyCloseIntentRequest) { r.Reason = modeldevv1.CloseReason_CLOSE_REASON_DEADLINE }},
		{"zero source generation", func(r *modeldevv1.ApplyCloseIntentRequest) { r.IntentGeneration = 0 }},
		{"timestamp invalid seconds", func(r *modeldevv1.ApplyCloseIntentRequest) { r.RequestedAt.Seconds = 253402300800 }},
		{"timestamp invalid nanos", func(r *modeldevv1.ApplyCloseIntentRequest) { r.RequestedAt.Nanos = -1 }},
		{"timestamp loses database precision", func(r *modeldevv1.ApplyCloseIntentRequest) { r.RequestedAt.Nanos++ }},
		{"nil operation", func(r *modeldevv1.ApplyCloseIntentRequest) { r.Identity.OperationId = uuid.Nil.String() }},
		{"invalid execution", func(r *modeldevv1.ApplyCloseIntentRequest) { r.Identity.ExecutionId = "untrusted-execution" }},
		{"invalid spec hash", func(r *modeldevv1.ApplyCloseIntentRequest) { r.Identity.ExecutionSpecHash = "untrusted-hash" }},
		{"invalid resource tenant", func(r *modeldevv1.ApplyCloseIntentRequest) { r.ResourceTenantId = "untrusted-tenant" }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			original := validCloseRequest()
			request := proto.Clone(original).(*modeldevv1.ApplyCloseIntentRequest)
			testCase.change(request)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			response, err := client.ApplyCloseIntent(commandContext(ctx, original), request)
			if response != nil || status.Code(err) != codes.InvalidArgument {
				t.Errorf("malformed command returned code %s, ACK=%t", status.Code(err), response != nil)
			}
			assertNoCommandFacts(t, reader, original)
		})
	}
}

func TestRawMetadataCannotInvokeCommandHandlerWithoutVerifiedContext(t *testing.T) {
	openPool := postgres.Prepare(t)
	repository := execution.New(openPool())
	handler := service.NewCommand(repository)
	request := validCloseRequest()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	md, _ := metadata.FromOutgoingContext(commandContext(ctx, request))
	for _, candidate := range []context.Context{ctx, metadata.NewIncomingContext(ctx, md)} {
		response, err := handler.ApplyCloseIntent(candidate, request)
		if response != nil || status.Code(err) != codes.Unauthenticated {
			t.Errorf("direct handler without typed delivery returned code %s, ACK=%t", status.Code(err), response != nil)
		}
	}
	// Nil cannot carry protobuf data over the socket. Exercise the adapter's
	// actual nil boundary with an explicit already-verified in-process context.
	verified := service.WithVerifiedGovernanceDelivery(ctx, service.GovernanceDelivery{TenantID: request.ResourceTenantId, Actor: request.RequestedActorId, RequestID: uuid.NewString()})
	response, err := handler.ApplyCloseIntent(verified, nil)
	if response != nil || status.Code(err) != codes.InvalidArgument {
		t.Errorf("nil command returned code %s, ACK=%t", status.Code(err), response != nil)
	}
	assertNoCommandFacts(t, execution.New(openPool()), request)
}
