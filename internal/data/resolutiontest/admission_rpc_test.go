package resolutiontest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	contractpb "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/admissionfacts"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/server"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/service"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestResolveAdmissionRPCUsesPinnedFactsAndDurableReadyInput(t *testing.T) {
	fixture := prepareResolution(t)
	source := writeManagedFactsFixture(t, fixture)
	reader, err := admissionfacts.Load(fixture.ctx, []admissionfacts.FileSource{source})
	if err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: pinned facts load failed; behavior NOT_RUN: %v", err)
	}
	facts, err := reader.ReadAdmissionFacts(fixture.ctx, fixture.selection.TenantID, fixture.selection.Release.ReleaseID, fixture.selection.Release.ReleaseDigest)
	if err != nil || !reflect.DeepEqual(facts, fixture.facts) {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: actual facts differ from the tenant/Release fixture; behavior NOT_RUN: %v", err)
	}
	beforeIntent, beforeHash, err := cpup01.CanonicalIntent(fixture.selection.Intent)
	if err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: original intent invalid; behavior NOT_RUN: %v", err)
	}
	intent, err := contractpb.EncodeIntent(fixture.selection.Intent)
	if err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: wire intent invalid; behavior NOT_RUN: %v", err)
	}
	request := &modeldevv1.ResolveAdmissionRequest{
		Intent: intent,
		Release: &modeldevv1.AdmissionReleaseSelection{
			ReleaseId: fixture.selection.Release.ReleaseID, ReleaseDigest: fixture.selection.Release.ReleaseDigest,
			BindingGeneration: fixture.selection.Release.BindingGeneration,
		},
		AcceptedAt: timestamppb.New(fixture.selection.AcceptedAt),
	}
	beforeRequest := proto.Clone(request)
	want := expectedManagedSnapshot(fixture)
	wantCanonical, err := want.Canonical()
	if err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: independently authored snapshot invalid; behavior NOT_RUN: %v", err)
	}
	wantDigest := sha256.Sum256(wantCanonical)
	wantHash := hex.EncodeToString(wantDigest[:])
	resolver := biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader)
	client := startAdmissionClient(t, fixture, resolver)
	t.Log("ADMISSION_RPC_PREFLIGHT PASS: actual pinned facts file, recovered READY PostgreSQL input, established mTLS channel")
	ctx := metadata.NewOutgoingContext(fixture.ctx, metadata.Pairs(
		"x-ani-tenant-id", fixture.selection.TenantID,
		"x-ani-actor", "governance:user:42",
		"x-ani-request-id", uuid.NewString(),
	))
	response, err := client.ResolveAdmission(ctx, request)
	if err != nil {
		if status.Code(err) == codes.Unimplemented && status.Convert(err).Message() == "admission resolution not implemented" {
			t.Fatalf("ADMISSION_RPC_BEHAVIOR: registered admission stub reached after authenticated delivery; candidate required: %v", err)
		}
		t.Fatalf("ADMISSION_RPC_BEHAVIOR: no candidate response (not the expected registered-stub RED): %v", err)
	}
	if response == nil {
		t.Fatal("ADMISSION_RPC_BEHAVIOR: missing candidate response")
	}
	snapshot, err := contractpb.DecodeSnapshot(response.Snapshot)
	if err != nil {
		t.Fatalf("ADMISSION_RPC_BEHAVIOR: returned snapshot violates the shared wire contract: %v", err)
	}
	canonical, err := snapshot.Canonical()
	if err != nil || !bytes.Equal(canonical, wantCanonical) || response.ExecutionSpecHash != wantHash {
		t.Fatalf("ADMISSION_RPC_BEHAVIOR: complete candidate or digest changed across the actual Proto roundtrip: %v", err)
	}
	afterIntent, afterHash, err := cpup01.CanonicalIntent(fixture.selection.Intent)
	if err != nil || beforeHash != afterHash || !bytes.Equal(beforeIntent, afterIntent) || !proto.Equal(request, beforeRequest) || request.Intent.GeneralParameters == nil || len(request.Intent.GeneralParameters.Values) != 1 || request.Intent.GeneralParameters.Values[0].GetDecimalValue() != "0.0200" {
		t.Fatal("ADMISSION_RPC_BEHAVIOR: resolution changed original intent or parameter presence")
	}
	stored, err := input.New(fixture.openPool()).Get(fixture.ctx, fixture.imported.TenantID, fixture.imported.InputVersionID)
	if err != nil || !reflect.DeepEqual(stored, fixture.ready) {
		t.Fatalf("ADMISSION_RPC_BEHAVIOR: resolution changed the durable READY input: %v", err)
	}
	requireNoResolvedExecution(t, fixture)
}

func startAdmissionClient(t *testing.T, fixture resolutionFixture, resolver *biz.AdmissionResolver) modeldevv1.ModelDevAdmissionServiceClient {
	t.Helper()
	return modeldevv1.NewModelDevAdmissionServiceClient(startAdmissionConnection(t, fixture, service.NewAdmission(resolver)))
}

func startAdmissionConnection(t *testing.T, fixture resolutionFixture, admission modeldevv1.ModelDevAdmissionServiceServer) *grpc.ClientConn {
	t.Helper()
	certificates := commandtls.New(t)
	s, err := server.NewGovernanceCommandServer(
		&conf.Server_GRPC{Network: "tcp", Addr: "127.0.0.1:0", Timeout: durationpb.New(5 * time.Second)},
		server.CommandTLS{Certificate: certificates.Server, ClientCAs: certificates.Roots, GovernanceDNSName: commandtls.GovernanceDNSName},
		service.NewCommand(execution.New(fixture.openPool())), admission,
	)
	if err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: TLS server construction failed; behavior NOT_RUN: %v", err)
	}
	endpoint, err := s.Endpoint()
	if err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: listener creation failed; behavior NOT_RUN: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Start(context.Background()) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Stop(ctx); err != nil {
			t.Errorf("stop admission server: %v", err)
		}
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				t.Errorf("admission server exited: %v", err)
			}
		case <-ctx.Done():
			t.Error("admission server did not stop")
		}
	})
	connection, err := grpc.NewClient(endpoint.Host, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: certificates.Roots, ServerName: commandtls.ServerDNSName,
		Certificates: []tls.Certificate{certificates.Governance},
	})))
	if err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: client construction failed; behavior NOT_RUN: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	ctx, cancel := context.WithTimeout(fixture.ctx, 5*time.Second)
	defer cancel()
	connection.Connect()
	for state := connection.GetState(); state != connectivity.Ready; state = connection.GetState() {
		if !connection.WaitForStateChange(ctx, state) {
			t.Fatalf("ADMISSION_RPC_PREFLIGHT: mTLS channel did not become ready; behavior NOT_RUN: %v", ctx.Err())
		}
	}
	return connection
}
