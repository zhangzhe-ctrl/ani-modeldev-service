package commandtest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestGovernanceCommandRejectsAmbiguousOrAmbientMetadataBeforePersistence(t *testing.T) {
	openPool := postgres.Prepare(t)
	client, _ := startCommandServer(t, execution.New(openPool()), commandtls.New(t))
	reader := execution.New(openPool())
	type mutation struct {
		name string
		apply func(metadata.MD)
	}
	var cases []mutation
	for _, key := range []string{"x-ani-tenant-id", "x-ani-actor", "x-ani-request-id"} {
		cases = append(cases,
			mutation{"missing " + key, func(md metadata.MD) { md.Delete(key) }},
			mutation{"empty " + key, func(md metadata.MD) { md.Set(key, "") }},
			mutation{"duplicate " + key, func(md metadata.MD) { md.Append(key, md.Get(key)[0]) }},
		)
	}
	for _, key := range []string{"x-ani-tenant-id", "x-ani-request-id"} {
		cases = append(cases,
			mutation{"nil UUID " + key, func(md metadata.MD) { md.Set(key, uuid.Nil.String()) }},
			mutation{"uppercase UUID " + key, func(md metadata.MD) { md.Set(key, strings.ToUpper("abcdefab-1234-4234-8234-abcdefabcdef")) }},
			mutation{"braced UUID " + key, func(md metadata.MD) { md.Set(key, "{" + uuid.NewString() + "}") }},
		)
	}
	// These are credential channels, not audit fields. In particular the IAM
	// SDK's actual workload/delegation keys cannot silently join this delivery.
	for _, key := range []string{"authorization", "proxy-authorization", "cookie", "x-forwarded-client-cert", "ani-workload-token", "ani-delegation"} {
		cases = append(cases, mutation{"ambient " + key, func(md metadata.MD) { md.Set(key, "synthetic-untrusted-credential") }})
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := validCloseRequest()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			md, _ := metadata.FromOutgoingContext(commandContext(ctx, request))
			testCase.apply(md)
			response, err := client.ApplyCloseIntent(metadata.NewOutgoingContext(ctx, md), request)
			if response != nil || status.Code(err) != codes.Unauthenticated {
				t.Errorf("ambiguous or credential-bearing delivery returned code %s, ACK=%t", status.Code(err), response != nil)
			}
			assertNoCommandFacts(t, reader, request)
		})
	}
	for _, actor := range []string{"governance:user:1", "governance:user:4294967295", "governance:access-key:1", "governance:access-key:4294967295"} {
		t.Run("authorized "+actor, func(t *testing.T) {
			request := validCloseRequest()
			request.RequestedActorId = actor
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			response, err := client.ApplyCloseIntent(commandContext(ctx, request), request)
			if err != nil { t.Fatalf("canonical authorized control failed: %v", err) }
			assertCloseResponse(t, response, request, false)
		})
	}
}

func TestGovernanceCommandBodyCannotReplaceVerifiedTenantOrActor(t *testing.T) {
	openPool := postgres.Prepare(t)
	client, _ := startCommandServer(t, execution.New(openPool()), commandtls.New(t))
	reader := execution.New(openPool())
	for _, field := range []string{"tenant", "actor"} {
		t.Run(field, func(t *testing.T) {
			verified := validCloseRequest()
			body := proto.Clone(verified).(*modeldevv1.ApplyCloseIntentRequest)
			if field == "tenant" { body.ResourceTenantId = uuid.NewString() } else { body.RequestedActorId = "governance:user:43" }
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			response, err := client.ApplyCloseIntent(commandContext(ctx, verified), body)
			if response != nil || status.Code(err) != codes.PermissionDenied {
				t.Errorf("body replaced authenticated %s: code %s, ACK=%t", field, status.Code(err), response != nil)
			}
			assertNoCommandFacts(t, reader, body)
			assertNoCommandFacts(t, reader, verified)
		})
	}
}
