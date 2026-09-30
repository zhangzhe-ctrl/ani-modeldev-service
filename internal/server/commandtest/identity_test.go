package commandtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGovernanceDeliveryRejectsNonCanonicalAuditActorsBeforePersistence(t *testing.T) {
	openPool := postgres.Prepare(t)
	client, _ := startCommandServer(t, execution.New(openPool()), commandtls.New(t))
	reader := execution.New(openPool())
	for _, actor := range []string{
		"system:operator", "governance:machine:42", "governance:user:0",
		"governance:user:042", "governance:user:4294967296",
		"governance:access-key:0", "governance:access-key:042", "governance:access-key:4294967296",
	} {
		t.Run(actor, func(t *testing.T) {
			request := validCloseRequest()
			request.RequestedActorId = actor
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			// The body and metadata agree, so a mere equality comparison cannot
			// turn an arbitrary audit string into a Governance user/API-key actor.
			response, err := client.ApplyCloseIntent(commandContext(ctx, request), request)
			if response != nil || status.Code(err) != codes.Unauthenticated {
				t.Errorf("CPU_COMMAND_AUTH_BEHAVIOR: invalid Governance actor received code %s, ACK=%t", status.Code(err), response != nil)
			}
			if _, err := reader.GetCloseIntent(ctx, request.ResourceTenantId, request.Identity.ExecutionId); !errors.Is(err, biz.ErrExecutionNotFound) {
				t.Errorf("CPU_COMMAND_AUTH_BEHAVIOR: rejected actor left a close fact: %v", err)
			}
		})
	}
}
