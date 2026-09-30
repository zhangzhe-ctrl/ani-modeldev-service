package commandtest

import (
	"context"
	"testing"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestGovernanceIdentityCannotReachOtherCapabilitiesOnCommandPort(t *testing.T) {
	openPool := postgres.Prepare(t)
	certificates := commandtls.New(t)
	address, _ := startCommandListener(t, execution.New(openPool()), certificates)
	connection := commandConnection(t, address, commandClientTLS(certificates))
	request := validCloseRequest()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = commandContext(ctx, request)
	// Probe registered transport health and actual shared-contract methods.
	// An unregistered capability is distinct from a registered method denial.
	for _, method := range []string{
		"/grpc.health.v1.Health/Check",
		"/grpc.channelz.v1.Channelz/GetTopChannels",
		modeldevv1.ModelDevQueryService_GetExecution_FullMethodName,
		modeldevv1.ModelDevQueryService_AuthorizeArtifactDownload_FullMethodName,
		modeldevv1.ModelDevStepService_BeginExecution_FullMethodName,
		modeldevv1.ModelDevStepService_EnsureTraining_FullMethodName,
		modeldevv1.ModelDevStepService_RequestExecutionClose_FullMethodName,
	} {
		t.Run(method, func(t *testing.T) {
			err := connection.Invoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{})
			if status.Code(err) != codes.PermissionDenied && status.Code(err) != codes.Unimplemented {
				t.Errorf("command port exposed other capability: %s", status.Code(err))
			}
		})
	}
	for _, method := range []string{
		"/grpc.health.v1.Health/Watch",
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
		"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
	} {
		t.Run(method, func(t *testing.T) {
			stream, err := connection.NewStream(ctx, &grpc.StreamDesc{ClientStreams: true, ServerStreams: true}, method)
			if err != nil { t.Fatalf("stream fixture could not start: %v", err) }
			// Empty is wire-compatible with the empty health/reflection request.
			// Send errors alone do not prove a denial: read the server's status.
			_ = stream.SendMsg(&emptypb.Empty{})
			_ = stream.CloseSend()
			err = stream.RecvMsg(&emptypb.Empty{})
			if status.Code(err) != codes.PermissionDenied && status.Code(err) != codes.Unimplemented {
				t.Errorf("command port exposed stream: %s", status.Code(err))
			}
		})
	}
	// Using the generated health client also verifies a properly encoded probe
	// is denied, rather than relying only on a malformed reflection probe.
	if _, err := healthv1.NewHealthClient(connection).Check(ctx, &healthv1.HealthCheckRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("registered transport health escaped method allowlist: %s", status.Code(err))
	}
	assertNoCommandFacts(t, execution.New(openPool()), request)
	response, err := modeldevv1.NewModelDevCommandServiceClient(connection).ApplyCloseIntent(ctx, request)
	if err != nil { t.Fatalf("authorized control failed after method matrix: %v", err) }
	assertCloseResponse(t, response, request, false)
}
