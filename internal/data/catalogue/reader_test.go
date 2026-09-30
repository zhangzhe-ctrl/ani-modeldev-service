package catalogue_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/catalogue"
)

func TestReadReleaseRetainsExplicitOldVersionAcrossFreshReads(t *testing.T) {
	directory := t.TempDir()
	oldID := "11111111-1111-4111-8111-111111111111"
	newID := "22222222-2222-4222-8222-222222222222"
	oldRaw := []byte(releaseFixtureV1)
	newRaw := []byte(strings.ReplaceAll(strings.ReplaceAll(releaseFixtureV1, oldID, newID),
		"55555555-5555-4555-8555-555555555555", "77777777-7777-4777-8777-777777777777"))
	for id, raw := range map[string][]byte{oldID: oldRaw, newID: newRaw} {
		if err := os.WriteFile(filepath.Join(directory, id+".json"), raw, 0600); err != nil {
			t.Fatalf("CPU04_CATALOGUE_PREFLIGHT: fixture write failed; behavior NOT_RUN: %v", err)
		}
	}
	oldHash := sha256.Sum256(oldRaw)
	newHash := sha256.Sum256(newRaw)
	want := releaseFixtureDocument()

	first, err := catalogue.ReadRelease(context.Background(), directory, oldID, hex.EncodeToString(oldHash[:]))
	if err != nil {
		t.Fatalf("read explicitly selected old Release: %v", err)
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("old Release lost its fixed program/runtime/output contract: got %+v", first)
	}
	newRelease, err := catalogue.ReadRelease(context.Background(), directory, newID, hex.EncodeToString(newHash[:]))
	if err != nil || newRelease.ReleaseID != newID || newRelease.PipelineVersionID != "77777777-7777-4777-8777-777777777777" {
		t.Fatalf("read explicitly selected new Release: got %+v, err %v", newRelease, err)
	}
	// A fresh read has no earlier return value or process-local current pointer.
	// Caller mutation must not change the immutable file or a later result.
	first.Program.Command[0] = "caller-local-mutation"
	again, err := catalogue.ReadRelease(context.Background(), directory, oldID, hex.EncodeToString(oldHash[:]))
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("new version or caller mutation displaced old Release: got %+v, err %v", again, err)
	}
}

// These authored module-only values identify no real PipelineVersion, image,
// Runtime, StorageClass or approved catalogue. They are never production data.
func releaseFixtureDocument() cpup01.ReleaseDocument {
	return cpup01.ReleaseDocument{
		SchemaVersion: cpup01.ReleaseSchemaVersion,
		ReleaseID: "11111111-1111-4111-8111-111111111111",
		PresetID: "33333333-3333-4333-8333-333333333333",
		Kind: "GENERAL_TRAINING", DeliveryMode: "SAVE_ARTIFACTS",
		PipelineID: "44444444-4444-4444-8444-444444444444",
		PipelineVersionID: "55555555-5555-4555-8555-555555555555",
		PipelineIRSHA256: strings.Repeat("a", 64),
		Runtime: cpup01.RuntimeRef{
			Name: "cpu-module-fixture", Kind: "ClusterTrainingRuntime", APIGroup: "trainer.kubeflow.org",
			ContentSHA256: strings.Repeat("b", 64), TargetJobs: []string{"trainer"},
		},
		Program: cpup01.ReleaseProgram{
			ImageVersionID: "66666666-6666-4666-8666-666666666666",
			ImageDigest: "registry.example.test/cpu-mlp@sha256:"+strings.Repeat("c", 64),
			Command: []string{"/opt/venv/bin/python", "-I", "/opt/cpu03/train_mlp.py"},
			ArgsTemplate: []cpup01.ReleaseArgument{
				{Literal: "--data"}, {Source: "INPUT_PATH"},
				{Literal: "--output"}, {Source: "OUTPUT_PATH"},
				{Literal: "--expected-input-sha256"}, {Source: "INPUT_SHA256"},
				{Literal: "--expected-input-bytes"}, {Source: "INPUT_BYTES"},
				{Literal: "--learning-rate"}, {Source: "LEARNING_RATE"},
			},
			ParameterContractVersion: "ani.cpu.mlp.parameters.v1",
			DefaultParameters: []cpup01.Parameter{
				{Name: "batch_size", Type: "INTEGER", Value: "64"},
				{Name: "epochs", Type: "INTEGER", Value: "3"},
				{Name: "learning_rate", Type: "DECIMAL", Value: "0.01"},
			},
		},
		Resources: cpup01.CPUResources{Nodes: 1, ProcessesPerNode: 1,
			RequestMillicpu: 1000, LimitMillicpu: 2000, RequestMemoryBytes: 1073741824, LimitMemoryBytes: 2147483648},
		Workspace: cpup01.WorkspaceContract{Mode: "EXECUTION_PVC", StorageClass: "module-fixture-sc", CapacityBytes: 1073741824,
			InputSubpath: "input", TrainingSubpath: "training", ReportsSubpath: "reports", PublicationSubpath: "publication"},
		OutputContract: cpup01.OutputContract{SchemaVersion: "ani.cpu.output.v1", OutputKind: "CHECKPOINT", DeliveryMode: "SAVE_ARTIFACTS",
			RequiredFiles: []cpup01.RequiredOutput{
				{Role: "METRICS", RelativePath: "metrics.jsonl", MaxSizeBytes: 16777216},
				{Role: "CHECKPOINT", RelativePath: "model.pt", MaxSizeBytes: 33554432},
				{Role: "MODEL_CONFIG", RelativePath: "model_config.json", MaxSizeBytes: 1048576},
				{Role: "SUMMARY", RelativePath: "summary.json", MaxSizeBytes: 1048576},
			},
			MaxFileCount: 4, MaxTotalBytes: 104857600, CreateTarBundle: false,
		},
		ExecutionTimeoutSeconds: 1800,
	}
}

const releaseFixtureV1 = `{"schema_version":"ani.modeldev.release.v1","release_id":"11111111-1111-4111-8111-111111111111","preset_id":"33333333-3333-4333-8333-333333333333","kind":"GENERAL_TRAINING","delivery_mode":"SAVE_ARTIFACTS","pipeline_id":"44444444-4444-4444-8444-444444444444","pipeline_version_id":"55555555-5555-4555-8555-555555555555","pipeline_ir_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","runtime":{"name":"cpu-module-fixture","kind":"ClusterTrainingRuntime","api_group":"trainer.kubeflow.org","content_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","target_jobs":["trainer"]},"program":{"image_version_id":"66666666-6666-4666-8666-666666666666","image_digest":"registry.example.test/cpu-mlp@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","command":["/opt/venv/bin/python","-I","/opt/cpu03/train_mlp.py"],"args_template":[{"literal":"--data"},{"source":"INPUT_PATH"},{"literal":"--output"},{"source":"OUTPUT_PATH"},{"literal":"--expected-input-sha256"},{"source":"INPUT_SHA256"},{"literal":"--expected-input-bytes"},{"source":"INPUT_BYTES"},{"literal":"--learning-rate"},{"source":"LEARNING_RATE"}],"parameter_contract_version":"ani.cpu.mlp.parameters.v1","default_parameters":[{"name":"batch_size","type":"INTEGER","value":"64"},{"name":"epochs","type":"INTEGER","value":"3"},{"name":"learning_rate","type":"DECIMAL","value":"0.01"}]},"resources":{"nodes":1,"processes_per_node":1,"request_millicpu":"1000","limit_millicpu":"2000","request_memory_bytes":"1073741824","limit_memory_bytes":"2147483648"},"workspace":{"mode":"EXECUTION_PVC","storage_class":"module-fixture-sc","capacity_bytes":"1073741824","input_subpath":"input","training_subpath":"training","reports_subpath":"reports","publication_subpath":"publication"},"output_contract":{"schema_version":"ani.cpu.output.v1","output_kind":"CHECKPOINT","delivery_mode":"SAVE_ARTIFACTS","required_files":[{"role":"METRICS","relative_path":"metrics.jsonl","max_size_bytes":"16777216"},{"role":"CHECKPOINT","relative_path":"model.pt","max_size_bytes":"33554432"},{"role":"MODEL_CONFIG","relative_path":"model_config.json","max_size_bytes":"1048576"},{"role":"SUMMARY","relative_path":"summary.json","max_size_bytes":"1048576"}],"max_file_count":4,"max_total_bytes":"104857600","create_tar_bundle":false},"execution_timeout_seconds":1800}`
