package resolutiontest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/admissionfacts"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
)

func TestResolveManagedAdmissionUsesPinnedFactsWithDurableInput(t *testing.T) {
	// Reuse actual catalogue import and restricted-role PostgreSQL READY
	// persistence/recovery. Their failures are preflight, not this behavior RED.
	fixture := prepareResolution(t)
	source := writeManagedFactsFixture(t, fixture)
	beforeIntent, beforeHash, err := cpup01.CanonicalIntent(fixture.selection.Intent)
	if err != nil {
		t.Fatalf("MANAGED_FACTS_PREFLIGHT: invalid original intent; behavior NOT_RUN: %v", err)
	}
	t.Log("MANAGED_FACTS_PREFLIGHT PASS: real catalogue, recovered PG READY input, actual pinned fixture file")

	reader, err := admissionfacts.Load(fixture.ctx, []admissionfacts.FileSource{source})
	if err != nil {
		t.Fatalf("load explicitly pinned managed facts for the selected tenant and Release: %v", err)
	}
	facts, err := reader.ReadAdmissionFacts(fixture.ctx, fixture.selection.TenantID, fixture.selection.Release.ReleaseID, fixture.selection.Release.ReleaseDigest)
	if err != nil || !reflect.DeepEqual(facts, fixture.facts) {
		t.Fatalf("loaded facts differ from the pinned tenant/Release document: %v", err)
	}
	if facts.Environment.BindingDigest == source.SHA256 {
		t.Fatal("file byte SHA replaced the independent ENV binding digest")
	}
	// A caller may mutate its returned facts without changing the owner's copy.
	facts.Runtime.TargetJobs[0] = "consumer-change"
	facts.Environment.Identities.TrainerServiceAccount = "consumer-change"

	resolver := biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader)
	resolved, err := resolver.ResolveManaged(fixture.ctx, fixture.selection)
	if err != nil {
		t.Fatalf("resolve from the loaded facts owner and actual durable input: %v", err)
	}
	want := expectedManagedSnapshot(fixture)
	wantCanonical, err := want.Canonical()
	if err != nil {
		t.Fatalf("authored expected snapshot violates the shared contract: %v", err)
	}
	wantDigest := sha256.Sum256(wantCanonical)
	wantHash := hex.EncodeToString(wantDigest[:])
	if !reflect.DeepEqual(resolved.Snapshot, want) || resolved.SpecHash != wantHash {
		t.Fatalf("managed resolution changed the complete expected snapshot or digest: got %+v", resolved)
	}
	resolved.Snapshot.Release.Runtime.TargetJobs[0] = "consumer-change"
	resolved.Snapshot.Program.Command[0] = "consumer-change"
	*resolved.Snapshot.Input.Object.VersionID = "consumer-change"
	again, err := resolver.ResolveManaged(fixture.ctx, fixture.selection)
	if err != nil || !reflect.DeepEqual(again.Snapshot, want) || again.SpecHash != wantHash {
		t.Fatalf("returned value mutation corrupted the managed facts or repeated candidate: %v", err)
	}
	afterIntent, afterHash, err := cpup01.CanonicalIntent(fixture.selection.Intent)
	if err != nil || beforeHash != afterHash || !bytes.Equal(beforeIntent, afterIntent) || (*fixture.selection.Intent.GeneralParameters)[0].Value != "0.0200" {
		t.Fatal("managed resolution changed original user intent or optional-field presence")
	}
	stored, err := input.New(fixture.openPool()).Get(fixture.ctx, fixture.imported.TenantID, fixture.imported.InputVersionID)
	if err != nil || !reflect.DeepEqual(stored, fixture.ready) {
		t.Fatalf("managed resolution changed the durable READY input: %v", err)
	}
	requireNoResolvedExecution(t, fixture)
}

func expectedManagedSnapshot(fixture resolutionFixture) cpup01.Snapshot {
	release, imported := fixture.release, fixture.imported
	// The complete expectation is authored independently of either resolver.
	// Only unchanged immutable source fields are copied; argv, parameters,
	// generation and deadline are fixed examples, not computed by product code.
	return cpup01.Snapshot{
		SchemaVersion: cpup01.SnapshotSchemaVersion, Kind: "GENERAL_TRAINING", DeliveryMode: "SAVE_ARTIFACTS",
		Release: cpup01.ReleaseSnapshot{
			ReleaseID: release.ReleaseID, ReleaseDigest: conformance.ReleaseSHA256V1, PresetID: release.PresetID, AcceptedBindingGeneration: 7,
			PipelineID: release.PipelineID, PipelineVersionID: release.PipelineVersionID, PipelineIRSHA256: release.PipelineIRSHA256, Runtime: release.Runtime,
		},
		Input: cpup01.InputRef{InputVersionID: imported.InputVersionID, Object: imported.Object, Format: "CSV", SchemaVersion: "ani.cpu.csv.v1", RowCount: 1024, FeatureCount: 16},
		Program: cpup01.ProgramRef{
			ImageVersionID: release.Program.ImageVersionID, ImageDigest: release.Program.ImageDigest, Command: release.Program.Command,
			ResolvedArgs:       []string{"--data", "/prepared-fixture/data.csv", "--output", "/trained-fixture", "--expected-input-sha256", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "--expected-input-bytes", "65536", "--learning-rate", "0.02"},
			ResolvedParameters: []cpup01.Parameter{{Name: "batch_size", Type: "INTEGER", Value: "64"}, {Name: "epochs", Type: "INTEGER", Value: "3"}, {Name: "learning_rate", Type: "DECIMAL", Value: "0.02"}},
		},
		Resources: release.Resources, Environment: fixture.facts.Environment, Workspace: release.Workspace,
		PublicationScope: fixture.facts.PublicationScope, OutputContract: release.OutputContract,
		DeadlineAt: time.Date(2026, 9, 30, 12, 33, 0, 123000, time.UTC),
	}
}

// The test authors the private JSON shape independently of the reader's
// unexported decoder. Values reuse the existing explicit module fixture; none
// identifies an approved ENV or constitutes actual application evidence.
func writeManagedFactsFixture(t *testing.T, fixture resolutionFixture) admissionfacts.FileSource {
	t.Helper()
	type evidence struct {
		Reference string `json:"reference"`
		SHA256    string `json:"sha256"`
	}
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
		SchemaVersion:    "ani.modeldev.managed-admission-facts.v1",
		ResourceTenantID: fixture.selection.TenantID,
		ReleaseID:        fixture.selection.Release.ReleaseID, ReleaseDigest: fixture.selection.Release.ReleaseDigest,
		Environment: fixture.facts.Environment, InputScope: fixture.facts.InputScope, PublicationScope: fixture.facts.PublicationScope,
		Runtime: fixture.facts.Runtime, Workspace: fixture.facts.Workspace,
		InputFilePath: fixture.facts.InputFilePath, OutputDirectoryPath: fixture.facts.OutputDirectoryPath,
		EnvironmentEvidence: evidence{Reference: "module-fixture:environment-handoff", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		ApplicationEvidence: evidence{Reference: "module-fixture:application-verification", SHA256: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("MANAGED_FACTS_PREFLIGHT: fixture encoding failed; behavior NOT_RUN: %v", err)
	}
	file := filepath.Join(t.TempDir(), "managed-admission-facts.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatalf("MANAGED_FACTS_PREFLIGHT: fixture write failed; behavior NOT_RUN: %v", err)
	}
	digest := sha256.Sum256(raw)
	return admissionfacts.FileSource{Path: file, SHA256: hex.EncodeToString(digest[:])}
}
