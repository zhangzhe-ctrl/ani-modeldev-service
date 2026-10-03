package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	contractpb "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/admissionfacts"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/catalogue"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestConfiguredAdmissionResolvesPinnedFilesAndDurableReadyInput(t *testing.T) {
	fixture := prepareConfiguredAdmission(t)
	app, err := buildApp(fixture.config, newRuntimeLogger(io.Discard))
	if err != nil {
		t.Fatalf("ADMISSION_STARTUP_BEHAVIOR: complete real startup materials rejected: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		if err := app.Stop(); err != nil {
			t.Errorf("stop configured admission app: %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("configured admission app exited: %v", err)
			}
		case <-time.After(6 * time.Second):
			t.Error("configured admission app did not stop within its bound")
		}
	}
	t.Cleanup(stop)
	waitForHTTP(t, "http://"+fixture.config.Server.Admin.Addr+"/healthz")
	connection, err := grpc.NewClient(fixture.config.Server.Grpc.Addr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: fixture.certificates.Roots, ServerName: commandtls.ServerDNSName,
		Certificates: []tls.Certificate{fixture.certificates.Governance},
	})))
	if err != nil {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: client configuration failed; behavior NOT_RUN: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection.Connect()
	for state := connection.GetState(); state != connectivity.Ready; state = connection.GetState() {
		if !connection.WaitForStateChange(ctx, state) {
			t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: real mTLS channel unavailable; behavior NOT_RUN: %v", ctx.Err())
		}
	}
	t.Log("ADMISSION_STARTUP_PREFLIGHT PASS: real catalogue and pinned facts files, recovered READY PostgreSQL input, actual buildApp mTLS listener")
	beforeRequest := proto.Clone(fixture.request)
	rpcContext := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-ani-tenant-id", fixture.tenantID, "x-ani-actor", "governance:user:42", "x-ani-request-id", uuid.NewString(),
	))
	response, err := modeldevv1.NewModelDevAdmissionServiceClient(connection).ResolveAdmission(rpcContext, fixture.request)
	if err != nil {
		if status.Code(err) == codes.Unimplemented && status.Convert(err).Message() == "unknown service ani.modeldev.v1.ModelDevAdmissionService" {
			t.Fatalf("ADMISSION_STARTUP_BEHAVIOR: explicit startup materials did not register Admission on the actual listener: %v", err)
		}
		t.Fatalf("ADMISSION_STARTUP_BEHAVIOR: candidate unavailable (not the expected unregistered-service RED): %v", err)
	}
	if response == nil {
		t.Fatal("ADMISSION_STARTUP_BEHAVIOR: configured admission returned no candidate")
	}
	snapshot, err := contractpb.DecodeSnapshot(response.Snapshot)
	if err != nil {
		t.Fatalf("ADMISSION_STARTUP_BEHAVIOR: invalid wire snapshot: %v", err)
	}
	canonical, err := snapshot.Canonical()
	if err != nil || !bytes.Equal(canonical, fixture.wantCanonical) || response.ExecutionSpecHash != fixture.wantHash {
		t.Fatalf("ADMISSION_STARTUP_BEHAVIOR: startup dependencies did not produce the complete independently expected snapshot and hash: %v", err)
	}
	if !proto.Equal(fixture.request, beforeRequest) {
		t.Fatal("ADMISSION_STARTUP_BEHAVIOR: startup resolution changed the caller's request")
	}
	observer := fixture.openPool()
	stored, err := input.New(observer).Get(ctx, fixture.tenantID, fixture.ready.Import.InputVersionID)
	if err != nil || !reflect.DeepEqual(stored, fixture.ready) {
		t.Fatalf("ADMISSION_STARTUP_BEHAVIOR: candidate resolution changed the durable READY input: %v", err)
	}
	var executions int
	if err := observer.QueryRow(ctx, "SELECT count(*) FROM modeldev_executions").Scan(&executions); err != nil || executions != 0 {
		t.Fatalf("ADMISSION_STARTUP_BEHAVIOR: candidate resolution persisted an execution: count=%d, error=%v", executions, err)
	}
	assertProductionNotReadyWithoutBusinessAdapters(t, fixture.config.Server.Admin.Addr)
	if countConfiguredAdmissionConnections(t, observer, fixture.applicationName) == 0 {
		t.Fatal("ADMISSION_STARTUP_PREFLIGHT: no actual configured database connection; cleanup behavior NOT_RUN")
	}
	_ = connection.Close()
	stop()
	cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cleanupCancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for countConfiguredAdmissionConnections(t, observer, fixture.applicationName) != 0 {
		select {
		case <-tick.C:
		case <-cleanupContext.Done():
			t.Fatal("ADMISSION_STARTUP_BEHAVIOR: stopped app retained its owned runtime database connection")
		}
	}
	for _, address := range []string{fixture.config.Server.Grpc.Addr, fixture.config.Server.Admin.Addr} {
		if remaining, err := net.DialTimeout("tcp", address, 250*time.Millisecond); err == nil {
			remaining.Close()
			t.Fatal("ADMISSION_STARTUP_BEHAVIOR: listener remained reachable after app exit")
		}
	}
}

type configuredAdmissionFixture struct {
	config          *conf.Bootstrap
	openPool        func() *pgxpool.Pool
	certificates    commandtls.Certificates
	applicationName string
	tenantID        string
	ready           biz.InputVersion
	request         *modeldevv1.ResolveAdmissionRequest
	wantCanonical   []byte
	wantHash        string
}

func prepareConfiguredAdmission(t *testing.T) configuredAdmissionFixture {
	t.Helper()
	config, openPool, certificates, applicationName := commandAppFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	release, seed := conformance.ReleaseV1(), conformance.SnapshotV1()
	directory := t.TempDir()
	if _, err := catalogue.ImportRelease(ctx, directory, conformance.ReleaseCanonicalV1(), conformance.ReleaseSHA256V1); err != nil {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: actual catalogue import failed; behavior NOT_RUN: %v", err)
	}
	imported := biz.InputImport{
		TenantID: "11111111-2222-4333-8444-555555555555", RequestID: uuid.NewString(), InputVersionID: seed.Input.InputVersionID,
		Actor: "governance:user:42", RequestedAt: time.Date(2026, 9, 30, 12, 0, 0, 123000, time.UTC),
		Scope:  cpup01.StorageScope{StorageConnectionID: seed.Input.Object.StorageConnectionID, Bucket: seed.Input.Object.Bucket, ApprovedPrefix: "fixed/input"},
		Object: seed.Input.Object,
	}
	writer := openPool()
	repository := input.New(writer)
	if _, err := repository.FreezeImport(ctx, imported); err != nil {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: actual input import failed; behavior NOT_RUN: %v", err)
	}
	// This is a synthetic trusted observation for module preparation, not an
	// assertion that a deployed ENV or remote S3 object has been verified.
	ready, err := repository.RecordVerifiedCSV(ctx, imported, biz.VerifiedCSV{
		VerifiedObject: biz.VerifiedObject{Object: imported.Object, VerifiedAt: imported.RequestedAt.Add(time.Minute)},
		SchemaVersion:  "ani.cpu.csv.v1", RowCount: 1024, FeatureCount: 16,
	})
	if err != nil || ready.State != biz.InputStateReady {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: actual READY persistence failed; behavior NOT_RUN: %v", err)
	}
	writer.Close()
	recovered, err := input.New(openPool()).Get(ctx, imported.TenantID, imported.InputVersionID)
	if err != nil || !reflect.DeepEqual(recovered, ready) {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: fresh connection cannot recover READY; behavior NOT_RUN: %v", err)
	}
	facts := biz.TenantAdmissionFacts{
		TenantID: imported.TenantID, Environment: seed.Environment, InputScope: imported.Scope, PublicationScope: seed.PublicationScope,
		Runtime: release.Runtime, Workspace: release.Workspace, InputFilePath: "/startup-input/data.csv", OutputDirectoryPath: "/startup-output",
	}
	source := writeConfiguredAdmissionFacts(t, release, facts)
	reader, err := admissionfacts.Load(ctx, []admissionfacts.FileSource{source})
	if err != nil {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: actual pinned file load failed; behavior NOT_RUN: %v", err)
	}
	loaded, err := reader.ReadAdmissionFacts(ctx, imported.TenantID, release.ReleaseID, conformance.ReleaseSHA256V1)
	if err != nil || !reflect.DeepEqual(loaded, facts) {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: actual file differs from explicit fixture facts; behavior NOT_RUN: %v", err)
	}
	config.Command.AdmissionResolution = &conf.AdmissionResolution{
		CatalogueDirectory: directory, FactsFiles: []*conf.AdmissionFactsFile{{Path: source.Path, Sha256: source.SHA256}},
	}
	parameters := []cpup01.Parameter{{Name: "learning_rate", Type: "DECIMAL", Value: "0.0200"}}
	intent, err := contractpb.EncodeIntent(cpup01.Intent{
		Name: "configured-admission", Kind: "GENERAL_TRAINING", PresetID: release.PresetID,
		DatasetVersionID: imported.InputVersionID, GeneralParameters: &parameters,
	})
	if err != nil {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: fixture intent invalid; behavior NOT_RUN: %v", err)
	}
	// A complete authored expectation, independent of resolver/template code.
	// The existing Snapshot vector supplies only fixed input/ENV fields; its
	// original Release and digest do not describe this selected Release.
	want := cpup01.Snapshot{
		SchemaVersion: cpup01.SnapshotSchemaVersion, Kind: "GENERAL_TRAINING", DeliveryMode: "SAVE_ARTIFACTS",
		Release: cpup01.ReleaseSnapshot{
			ReleaseID: release.ReleaseID, ReleaseDigest: conformance.ReleaseSHA256V1, PresetID: release.PresetID, AcceptedBindingGeneration: 7,
			PipelineID: release.PipelineID, PipelineVersionID: release.PipelineVersionID, PipelineIRSHA256: release.PipelineIRSHA256, Runtime: release.Runtime,
		},
		Input: seed.Input,
		Program: cpup01.ProgramRef{
			ImageVersionID: release.Program.ImageVersionID, ImageDigest: release.Program.ImageDigest, Command: release.Program.Command,
			ResolvedArgs:       []string{"--data", "/startup-input/data.csv", "--output", "/startup-output", "--expected-input-sha256", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "--expected-input-bytes", "192456", "--learning-rate", "0.02"},
			ResolvedParameters: []cpup01.Parameter{{Name: "batch_size", Type: "INTEGER", Value: "64"}, {Name: "epochs", Type: "INTEGER", Value: "3"}, {Name: "learning_rate", Type: "DECIMAL", Value: "0.02"}},
		},
		Resources: release.Resources, Environment: seed.Environment, Workspace: release.Workspace,
		PublicationScope: seed.PublicationScope, OutputContract: release.OutputContract,
		DeadlineAt: time.Date(2026, 9, 30, 12, 33, 0, 123000, time.UTC),
	}
	canonical, err := want.Canonical()
	if err != nil {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: independently authored expectation invalid; behavior NOT_RUN: %v", err)
	}
	digest := sha256.Sum256(canonical)
	return configuredAdmissionFixture{
		config: config, openPool: openPool, certificates: certificates, applicationName: applicationName, tenantID: imported.TenantID, ready: ready,
		request: &modeldevv1.ResolveAdmissionRequest{
			Intent: intent, Release: &modeldevv1.AdmissionReleaseSelection{ReleaseId: release.ReleaseID, ReleaseDigest: conformance.ReleaseSHA256V1, BindingGeneration: 7},
			AcceptedAt: timestamppb.New(time.Date(2026, 9, 30, 12, 3, 0, 123000, time.UTC)),
		},
		wantCanonical: canonical, wantHash: hex.EncodeToString(digest[:]),
	}
}

func writeConfiguredAdmissionFacts(t *testing.T, release cpup01.ReleaseDocument, facts biz.TenantAdmissionFacts) admissionfacts.FileSource {
	t.Helper()
	type evidence struct {
		Reference string `json:"reference"`
		SHA256    string `json:"sha256"`
	}
	// The private file fixture is explicitly synthetic and authored without
	// depending on the production reader's unexported decoder structure.
	document := struct {
		SchemaVersion       string                            `json:"schema_version"`
		ResourceTenantID    string                            `json:"resource_tenant_id"`
		ReleaseID           string                            `json:"release_id"`
		ReleaseDigest       string                            `json:"release_digest"`
		Environment         cpup01.EnvironmentBindingSnapshot `json:"environment"`
		InputScope          cpup01.StorageScope               `json:"input_scope"`
		PublicationScope    cpup01.StorageScope               `json:"publication_scope"`
		Runtime             cpup01.RuntimeRef                 `json:"runtime"`
		Workspace           cpup01.WorkspaceContract          `json:"workspace"`
		InputFilePath       string                            `json:"input_file_path"`
		OutputDirectoryPath string                            `json:"output_directory_path"`
		EnvironmentEvidence evidence                          `json:"environment_evidence"`
		ApplicationEvidence evidence                          `json:"application_evidence"`
	}{
		SchemaVersion: "ani.modeldev.managed-admission-facts.v1", ResourceTenantID: facts.TenantID,
		ReleaseID: release.ReleaseID, ReleaseDigest: conformance.ReleaseSHA256V1,
		Environment: facts.Environment, InputScope: facts.InputScope, PublicationScope: facts.PublicationScope,
		Runtime: facts.Runtime, Workspace: facts.Workspace, InputFilePath: facts.InputFilePath, OutputDirectoryPath: facts.OutputDirectoryPath,
		EnvironmentEvidence: evidence{Reference: "module-fixture:startup-environment", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		ApplicationEvidence: evidence{Reference: "module-fixture:startup-application", SHA256: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("ADMISSION_STARTUP_PREFLIGHT: fixture encoding failed; behavior NOT_RUN: %v", err)
	}
	file := filepath.Join(t.TempDir(), "managed-admission-facts.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal("ADMISSION_STARTUP_PREFLIGHT: fixture file creation failed; behavior NOT_RUN")
	}
	digest := sha256.Sum256(raw)
	return admissionfacts.FileSource{Path: file, SHA256: hex.EncodeToString(digest[:])}
}

func countConfiguredAdmissionConnections(t *testing.T, observer *pgxpool.Pool, applicationName string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var count int
	if err := observer.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE application_name = $1 AND usename = current_user AND datname = current_database()", applicationName).Scan(&count); err != nil {
		t.Fatal("ADMISSION_STARTUP_PREFLIGHT: inspect exact task-owned database connection; cleanup behavior NOT_RUN")
	}
	return count
}
