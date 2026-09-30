package main

import (
	"context"
	"crypto/tls"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
	"github.com/google/uuid"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestConfiguredCommandRunsActualTLSAndDurableRepository(t *testing.T) {
	openPool := postgres.Prepare(t)
	pool := openPool()
	certificates := commandtls.New(t)
	files := certificates.WriteServerFiles(t)
	connectionURL, err := url.Parse(os.Getenv("CPU_P01_TEST_DATABASE_URL"))
	if err != nil || connectionURL.Scheme != "postgres" && connectionURL.Scheme != "postgresql" {
		t.Fatal("CPU_COMMAND_PREFLIGHT: URI database fixture required; behavior NOT_RUN")
	}
	query := connectionURL.Query()
	query.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	connectionURL.RawQuery = query.Encode()
	databaseFile := filepath.Join(t.TempDir(), "database")
	if err := os.WriteFile(databaseFile, []byte(connectionURL.String()), 0600); err != nil {
		t.Fatal("CPU_COMMAND_PREFLIGHT: write protected runtime database reference; behavior NOT_RUN")
	}
	config := commandAppConfig(t)
	config.Command = &conf.GovernanceCommand{DatabaseUrlFile: databaseFile, ClientCaFile: files.CAFile, CertificateFile: files.CertificateFile, PrivateKeyFile: files.PrivateKeyFile, GovernanceDnsName: commandtls.GovernanceDNSName}
	app, err := buildApp(config, newRuntimeLogger(io.Discard))
	if err != nil { t.Fatalf("CPU_COMMAND_ASSEMBLY: complete actual dependencies rejected: %v", err) }
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	t.Cleanup(func() {
		if err := app.Stop(); err != nil { t.Errorf("stop assembled app: %v", err) }
		select {
		case err := <-done:
			if err != nil { t.Errorf("assembled app exited: %v", err) }
		case <-time.After(6*time.Second): t.Error("assembled app did not stop")
		}
	})
	waitForHTTP(t, "http://"+config.Server.Admin.Addr+"/healthz")
	connection, err := grpc.NewClient(config.Server.Grpc.Addr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: certificates.Roots, ServerName: commandtls.ServerDNSName, Certificates: []tls.Certificate{certificates.Governance}})))
	if err != nil { t.Fatalf("TLS client fixture: %v", err) }
	defer connection.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tenantID := uuid.NewString()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-ani-tenant-id", tenantID, "x-ani-actor", "governance:user:42", "x-ani-request-id", uuid.NewString()))
	request := &modeldevv1.ApplyCloseIntentRequest{
		Identity: &trainingv1.ExecutionIdentity{OperationId: uuid.NewString(), ExecutionId: uuid.NewString(), ExecutionSpecHash: strings.Repeat("b", 64)},
		ResourceTenantId: tenantID, IntentGeneration: 41, Reason: modeldevv1.CloseReason_CLOSE_REASON_USER_STOP,
		RequestedAt: timestamppb.New(time.Now().UTC().Truncate(time.Microsecond)), RequestedActorId: "governance:user:42",
	}
	response, err := modeldevv1.NewModelDevCommandServiceClient(connection).ApplyCloseIntent(ctx, request)
	if err != nil || response == nil || !response.DurablyRecorded || response.Replayed || response.CloseGeneration != 1 {
		t.Fatalf("CPU_COMMAND_ASSEMBLY: configured listener did not return the real durable receipt (code=%s)", status.Code(err))
	}
	stored, err := execution.New(openPool()).GetCloseIntent(ctx, tenantID, request.Identity.ExecutionId)
	if err != nil || stored.SpecHash != request.Identity.ExecutionSpecHash || stored.SourceGeneration != 41 || stored.Generation != 1 {
		t.Fatalf("CPU_COMMAND_ASSEMBLY: ACK without independently visible close: %v", err)
	}
	assertProductionNotReadyWithoutBusinessAdapters(t, config.Server.Admin.Addr)
}

func TestConfiguredCommandDoesNotFallBackWhenConnectionsAreMissing(t *testing.T) {
	config := commandAppConfig(t)
	root := t.TempDir()
	config.Command = &conf.GovernanceCommand{
		DatabaseUrlFile: filepath.Join(root, "missing-database"), ClientCaFile: filepath.Join(root, "missing-ca.crt"),
		CertificateFile: filepath.Join(root, "missing-tls.crt"), PrivateKeyFile: filepath.Join(root, "missing-tls.key"), GovernanceDnsName: "ani-governance",
	}
	if app, err := buildApp(config, newRuntimeLogger(io.Discard)); err == nil || app != nil {
		t.Fatal("configured command must fail startup when its actual connection materials are missing")
	}
}

func commandAppConfig(t *testing.T) *conf.Bootstrap {
	t.Helper()
	grpcAddress, adminAddress := reserveAddress(t), reserveAddress(t)
	for grpcAddress == adminAddress {
		adminAddress = reserveAddress(t)
	}
	return &conf.Bootstrap{Server: &conf.Server{
		Grpc:            &conf.Server_GRPC{Network: "tcp", Addr: grpcAddress, Timeout: durationpb.New(3 * time.Second)},
		Admin:           &conf.Server_Admin{Network: "tcp", Addr: adminAddress, Timeout: durationpb.New(time.Second)},
		ShutdownTimeout: durationpb.New(5 * time.Second),
	}}
}
