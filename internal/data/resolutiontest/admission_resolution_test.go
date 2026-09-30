package resolutiontest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/catalogue"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestResolveAdmissionFreezesSelectedReleaseAndReadyInput(t *testing.T) {
	openPool := postgres.Prepare(t)
	writer := openPool()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	directory := t.TempDir()
	release := conformance.ReleaseV1()
	if _, err := catalogue.ImportRelease(ctx, directory, conformance.ReleaseCanonicalV1(), conformance.ReleaseSHA256V1); err != nil {
		t.Fatalf("RESOLUTION_PREFLIGHT: real catalogue import failed; behavior NOT_RUN: %v", err)
	}
	version := "synthetic-fixed-input-version"
	request := biz.InputImport{
		TenantID:  "11111111-2222-4333-8444-555555555555",
		RequestID: "22222222-2222-4222-8222-222222222222", InputVersionID: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		Actor: "governance:user:42", RequestedAt: time.Date(2026, 9, 30, 12, 0, 0, 123000, time.UTC),
		Scope: cpup01.StorageScope{StorageConnectionID: "fixture-input-store", Bucket: "fixture-inputs", ApprovedPrefix: "tenant/input"},
		Object: cpup01.FixedObjectRef{StorageConnectionID: "fixture-input-store", Bucket: "fixture-inputs", Key: "tenant/input/data.csv",
			VersionID: &version, SizeBytes: 65536, SHA256: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"},
	}
	repository := input.New(writer)
	if _, err := repository.FreezeImport(ctx, request); err != nil {
		t.Fatalf("RESOLUTION_PREFLIGHT: fixed import failed; behavior NOT_RUN: %v", err)
	}
	// This synthetic trusted observation exercises the real durable READY path.
	// This test does not claim remote byte verification or real ENV readiness.
	proof := biz.VerifiedCSV{
		VerifiedObject: biz.VerifiedObject{Object: request.Object, VerifiedAt: request.RequestedAt.Add(time.Minute)},
		SchemaVersion:  "ani.cpu.csv.v1", RowCount: 1024, FeatureCount: 16,
	}
	ready, err := repository.RecordVerifiedCSV(ctx, request, proof)
	if err != nil || ready.State != biz.InputStateReady {
		t.Fatalf("RESOLUTION_PREFLIGHT: READY persistence failed; behavior NOT_RUN: %v", err)
	}
	writer.Close()
	reader := input.New(openPool())
	recovered, err := reader.Get(ctx, request.TenantID, request.InputVersionID)
	if err != nil || !reflect.DeepEqual(recovered, ready) {
		t.Fatalf("RESOLUTION_PREFLIGHT: new connection cannot recover READY; behavior NOT_RUN: %v", err)
	}

	// All configuration below is a module fixture. These values identify no
	// approved environment, path mapping, namespace, Runtime or publication.
	facts := biz.TenantAdmissionFacts{
		TenantID: request.TenantID,
		Environment: cpup01.EnvironmentBindingSnapshot{
			BindingID:     "11111111-1111-4111-8111-111111111111",
			BindingDigest: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			ClusterID:     "module-fixture-cluster", NamespaceName: "module-fixture-tenant",
			NamespaceUID: "33333333-3333-4333-8333-333333333333", KFPConnectionRef: "fixture-kfp-v1",
			ExperimentID: "44444444-4444-4444-8444-444444444444",
			Identities: cpup01.RuntimeIdentityRefs{
				ModeldevControlIdentityRef: "fixture-modeldev-control", KFPStepServiceAccount: "fixture-managed-step",
				TrainerServiceAccount: "fixture-training", VerifierServiceAccount: "fixture-verifier", TenantProxyIdentity: "fixture:tenant",
			},
		},
		PublicationScope: cpup01.StorageScope{StorageConnectionID: "fixture-publication-store", Bucket: "fixture-artifacts", ApprovedPrefix: "tenant/artifacts", CredentialReference: "fixture-publisher-ref"},
		InputScope:       request.Scope, Runtime: release.Runtime, Workspace: release.Workspace,
		InputFilePath: "/prepared-fixture/data.csv", OutputDirectoryPath: "/trained-fixture",
	}
	parameters := []cpup01.Parameter{{Name: "learning_rate", Type: "DECIMAL", Value: "0.0200"}}
	intent := cpup01.Intent{
		Name: "fixed-resolution", Kind: "GENERAL_TRAINING", PresetID: release.PresetID,
		DatasetVersionID: request.InputVersionID, GeneralParameters: &parameters,
	}
	beforeIntent, beforeHash, err := cpup01.CanonicalIntent(intent)
	if err != nil {
		t.Fatalf("RESOLUTION_PREFLIGHT: intent fixture invalid; behavior NOT_RUN: %v", err)
	}
	selection := biz.AdmissionResolutionRequest{
		TenantID: request.TenantID, Intent: intent,
		Release:    biz.AdmissionReleaseSelection{ReleaseID: release.ReleaseID, ReleaseDigest: conformance.ReleaseSHA256V1, BindingGeneration: 7},
		AcceptedAt: time.Date(2026, 9, 30, 12, 3, 0, 123000, time.UTC),
	}
	releaseReader := &retainedReleaseReader{delegate: catalogue.NewReader(directory)}
	inputReader := &retainedInputReader{delegate: reader}
	resolver := biz.NewAdmissionResolver(releaseReader, inputReader)
	resolved, err := resolver.Resolve(ctx, selection, facts)
	if err != nil {
		t.Fatalf("resolve the selected immutable Release and persisted READY input: %v", err)
	}
	// The expected argv and deadline are authored explicitly; no resolver helper
	// or template expansion builds this expectation. Other fixed facts retain
	// their selected immutable sources without allocating command/runtime IDs.
	want := cpup01.Snapshot{
		SchemaVersion: cpup01.SnapshotSchemaVersion, Kind: "GENERAL_TRAINING", DeliveryMode: "SAVE_ARTIFACTS",
		Release: cpup01.ReleaseSnapshot{
			ReleaseID: release.ReleaseID, ReleaseDigest: conformance.ReleaseSHA256V1, PresetID: release.PresetID, AcceptedBindingGeneration: 7,
			PipelineID: release.PipelineID, PipelineVersionID: release.PipelineVersionID, PipelineIRSHA256: release.PipelineIRSHA256, Runtime: release.Runtime,
		},
		Input: cpup01.InputRef{InputVersionID: request.InputVersionID, Object: request.Object, Format: "CSV", SchemaVersion: "ani.cpu.csv.v1", RowCount: 1024, FeatureCount: 16},
		Program: cpup01.ProgramRef{
			ImageVersionID: release.Program.ImageVersionID, ImageDigest: release.Program.ImageDigest, Command: release.Program.Command,
			ResolvedArgs:       []string{"--data", "/prepared-fixture/data.csv", "--output", "/trained-fixture", "--expected-input-sha256", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "--expected-input-bytes", "65536", "--learning-rate", "0.02"},
			ResolvedParameters: []cpup01.Parameter{{Name: "batch_size", Type: "INTEGER", Value: "64"}, {Name: "epochs", Type: "INTEGER", Value: "3"}, {Name: "learning_rate", Type: "DECIMAL", Value: "0.02"}},
		},
		Resources: release.Resources, Environment: facts.Environment, Workspace: release.Workspace,
		PublicationScope: facts.PublicationScope, OutputContract: release.OutputContract,
		DeadlineAt: time.Date(2026, 9, 30, 12, 33, 0, 123000, time.UTC),
	}
	if !reflect.DeepEqual(resolved.Snapshot, want) {
		t.Fatalf("resolution changed or omitted frozen facts: got %+v; want %+v", resolved.Snapshot, want)
	}
	canonical, err := want.Canonical()
	if err != nil {
		t.Fatalf("authored expected snapshot violates the shared contract: %v", err)
	}
	wantHash := sha256.Sum256(canonical)
	if resolved.SpecHash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("candidate digest does not bind the complete expected snapshot")
	}
	afterIntent, afterHash, err := cpup01.CanonicalIntent(intent)
	if err != nil || beforeHash != afterHash || !bytes.Equal(beforeIntent, afterIntent) || parameters[0].Value != "0.0200" || intent.ImageVersionID != nil {
		t.Fatal("resolution changed the caller's original intent or optional-field presence")
	}
	stored, err := input.New(openPool()).Get(ctx, request.TenantID, request.InputVersionID)
	if err != nil || !reflect.DeepEqual(stored, ready) {
		t.Fatalf("read-only resolution changed the persisted input: %v", err)
	}
	var executions int
	if err := openPool().QueryRow(ctx, "SELECT count(*) FROM modeldev_executions").Scan(&executions); err != nil || executions != 0 {
		t.Fatalf("resolution persisted an execution instead of returning a candidate: count=%d, error=%v", executions, err)
	}
	// Retain the actual file/PG read values, then mutate the candidate returned
	// to the consumer. This must not corrupt upstream facts or a later resolve.
	resolved.Snapshot.Program.Command[0] = "consumer-change"
	resolved.Snapshot.Release.Runtime.TargetJobs[0] = "consumer-change"
	resolved.Snapshot.OutputContract.RequiredFiles[0].RelativePath = "consumer-change"
	*resolved.Snapshot.Input.Object.VersionID = "consumer-change"
	if !reflect.DeepEqual(releaseReader.last, release) || !reflect.DeepEqual(inputReader.last, ready) || !reflect.DeepEqual(facts.Runtime, release.Runtime) {
		t.Fatal("returned candidate aliases an upstream immutable value")
	}
	again, err := resolver.Resolve(ctx, selection, facts)
	if err != nil || !reflect.DeepEqual(again.Snapshot, want) || again.SpecHash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("consumer mutation changed repeated fixed-fact resolution: %v", err)
	}
}

type retainedReleaseReader struct {
	delegate biz.AdmissionReleaseReader
	last cpup01.ReleaseDocument
}

func (reader *retainedReleaseReader) ReadRelease(ctx context.Context, id, digest string) (cpup01.ReleaseDocument, error) {
	value, err := reader.delegate.ReadRelease(ctx, id, digest)
	reader.last = value
	return value, err
}

type retainedInputReader struct {
	delegate biz.AdmissionInputReader
	last biz.InputVersion
}

func (reader *retainedInputReader) Get(ctx context.Context, tenant, id string) (biz.InputVersion, error) {
	value, err := reader.delegate.Get(ctx, tenant, id)
	reader.last = value
	return value, err
}
