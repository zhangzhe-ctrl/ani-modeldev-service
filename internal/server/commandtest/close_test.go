package commandtest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
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
	certificates := newCommandCertificates(t)
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
		Identity: &trainingv1.ExecutionIdentity{OperationId: uuid.NewString(), ExecutionId: uuid.NewString(), ExecutionSpecHash: strings.Repeat("b", 64)},
		ResourceTenantId: uuid.NewString(), IntentGeneration: 41,
		Reason: modeldevv1.CloseReason_CLOSE_REASON_USER_STOP,
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

type commandCertificates struct {
	roots *x509.CertPool
	server, governance tls.Certificate
}

// All private keys exist only in the Fedora test process. These certificates
// are module fixtures, never evidence of a deployed ENV identity.
func newCommandCertificates(t *testing.T) commandCertificates {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil { t.Fatal(err) }
	now := time.Now().UTC()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "CPU command test CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil { t.Fatal(err) }
	ca, err = x509.ParseCertificate(der)
	if err != nil { t.Fatal(err) }
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	issue := func(serial int64, name string, usage x509.ExtKeyUsage) tls.Certificate {
		leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil { t.Fatal(err) }
		leaf := &x509.Certificate{SerialNumber: big.NewInt(serial), DNSNames: []string{name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
		encoded, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
		if err != nil { t.Fatal(err) }
		return tls.Certificate{Certificate: [][]byte{encoded}, PrivateKey: leafKey}
	}
	return commandCertificates{roots: roots, server: issue(2, "modeldev.test", x509.ExtKeyUsageServerAuth), governance: issue(3, "ani-governance", x509.ExtKeyUsageClientAuth)}
}

func startCommandServer(t *testing.T, repository biz.ExecutionRepository, certificates commandCertificates) (modeldevv1.ModelDevCommandServiceClient, func()) {
	t.Helper()
	s, err := server.NewGovernanceCommandServer(&conf.Server_GRPC{Network: "tcp", Addr: "127.0.0.1:0", Timeout: durationpb.New(5*time.Second)}, server.CommandTLS{Certificate: certificates.server, ClientCAs: certificates.roots, GovernanceDNSName: "ani-governance"}, service.NewCommand(repository))
	if err != nil { t.Fatalf("TLS server fixture: %v", err) }
	endpoint, err := s.Endpoint()
	if err != nil { t.Fatalf("TLS listener fixture: %v", err) }
	done := make(chan error, 1)
	go func() { done <- s.Start(context.Background()) }()
	connection, err := grpc.NewClient(endpoint.Host, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: certificates.roots, ServerName: "modeldev.test", Certificates: []tls.Certificate{certificates.governance}})))
	if err != nil { t.Fatalf("TLS client fixture: %v", err) }
	stopped := false
	stop := func() {
		if stopped { return }
		stopped = true
		connection.Close()
		stopCommandServer(t, s, done)
	}
	t.Cleanup(stop)
	return modeldevv1.NewModelDevCommandServiceClient(connection), stop
}

func stopCommandServer(t *testing.T, s *kratosgrpc.Server, done <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil { t.Errorf("stop command server: %v", err) }
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) { t.Errorf("command server exited: %v", err) }
	case <-ctx.Done(): t.Error("command server did not stop")
	}
}
