package commandtest

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"
	"time"

	kratosgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/google/uuid"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/server"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/service"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGovernanceCloseCommitsBeforeACKAndReplaysAfterServerRestart(t *testing.T) {
	openPool := postgres.Prepare(t)
	certificates := commandtls.New(t)
	request := validCloseRequest()
	writer := openPool()
	repository := execution.New(writer)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := repository.GetCloseIntent(ctx, request.ResourceTenantId, request.Identity.ExecutionId); !errors.Is(err, biz.ErrExecutionNotFound) {
		t.Fatalf("close fixture is not empty; delivery behavior NOT_RUN: %v", err)
	}
	client, stop := startCommandServer(t, repository, certificates)
	first, err := client.ApplyCloseIntent(commandContext(ctx, request), request)
	if err != nil {
		t.Fatalf("CPU_COMMAND_BEHAVIOR: authenticated close did not receive a durable ACK: %v", err)
	}
	assertCloseResponse(t, first, request, false)
	stored, err := execution.New(openPool()).GetCloseIntent(ctx, request.ResourceTenantId, request.Identity.ExecutionId)
	if err != nil || stored.Generation != 1 || stored.SourceGeneration != 41 || stored.RequestedActor != request.RequestedActorId || stored.SpecHash != request.Identity.ExecutionSpecHash || !stored.RequestedAt.Equal(request.RequestedAt.AsTime()) {
		t.Fatalf("CPU_COMMAND_BEHAVIOR: ACK was not backed by original committed close facts: %v", err)
	}
	stop()
	writer.Close()
	reconnected, _ := startCommandServer(t, execution.New(openPool()), certificates)
	replayed, err := reconnected.ApplyCloseIntent(commandContext(ctx, request), request)
	if err != nil {
		t.Fatalf("CPU_COMMAND_BEHAVIOR: close replay after server and pool restart: %v", err)
	}
	assertCloseResponse(t, replayed, request, true)
	if _, err := execution.New(openPool()).Get(ctx, request.ResourceTenantId, request.Identity.ExecutionId); !errors.Is(err, biz.ErrExecutionNotFound) {
		t.Fatalf("close delivery invented an Admission: %v", err)
	}
}

func validCloseRequest() *modeldevv1.ApplyCloseIntentRequest {
	return &modeldevv1.ApplyCloseIntentRequest{
		Identity:         &trainingv1.ExecutionIdentity{OperationId: uuid.NewString(), ExecutionId: uuid.NewString(), ExecutionSpecHash: strings.Repeat("b", 64)},
		ResourceTenantId: uuid.NewString(), IntentGeneration: 41,
		Reason:      modeldevv1.CloseReason_CLOSE_REASON_USER_STOP,
		RequestedAt: timestamppb.New(time.Date(2026, 9, 30, 10, 0, 0, 123000, time.UTC)), RequestedActorId: "governance:user:42",
	}
}

func commandContext(ctx context.Context, request *modeldevv1.ApplyCloseIntentRequest) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("x-ani-tenant-id", request.ResourceTenantId, "x-ani-actor", request.RequestedActorId, "x-ani-request-id", uuid.NewString()))
}

func assertCloseResponse(t *testing.T, response *modeldevv1.ApplyCloseIntentResponse, request *modeldevv1.ApplyCloseIntentRequest, replayed bool) {
	t.Helper()
	if response == nil || !proto.Equal(response.Identity, request.Identity) || response.CloseGeneration != 1 || response.CloseState != modeldevv1.CloseState_CLOSE_STATE_CLOSING || response.Replayed != replayed || !response.DurablyRecorded {
		t.Fatalf("CPU_COMMAND_BEHAVIOR: wrong durable close receipt: %v", response)
	}
}

func startCommandServer(t *testing.T, repository biz.ExecutionRepository, certificates commandtls.Certificates) (modeldevv1.ModelDevCommandServiceClient, func()) {
	t.Helper()
	address, stopServer := startCommandListener(t, repository, certificates)
	connection := commandConnection(t, address, commandClientTLS(certificates))
	stop := func() {
		_ = connection.Close()
		stopServer()
	}
	return modeldevv1.NewModelDevCommandServiceClient(connection), stop
}

func commandClientTLS(certificates commandtls.Certificates) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: certificates.Roots, ServerName: commandtls.ServerDNSName, Certificates: []tls.Certificate{certificates.Governance}}
}

func commandConnection(t *testing.T, address string, tlsConfig *tls.Config) *grpc.ClientConn {
	t.Helper()
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		t.Fatalf("TLS client fixture: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func startCommandListener(t *testing.T, repository biz.ExecutionRepository, certificates commandtls.Certificates) (string, func()) {
	t.Helper()
	s, err := server.NewGovernanceCommandServer(&conf.Server_GRPC{Network: "tcp", Addr: "127.0.0.1:0", Timeout: durationpb.New(5 * time.Second)}, server.CommandTLS{Certificate: certificates.Server, ClientCAs: certificates.Roots, GovernanceDNSName: commandtls.GovernanceDNSName}, service.NewCommand(repository), nil)
	if err != nil {
		t.Fatalf("TLS server fixture: %v", err)
	}
	endpoint, err := s.Endpoint()
	if err != nil {
		t.Fatalf("TLS listener fixture: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Start(context.Background()) }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		stopCommandServer(t, s, done)
	}
	t.Cleanup(stop)
	return endpoint.Host, stop
}

func stopCommandServer(t *testing.T, s *kratosgrpc.Server, done <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Errorf("stop command server: %v", err)
	}
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("command server exited: %v", err)
		}
	case <-ctx.Done():
		t.Error("command server did not stop")
	}
}
