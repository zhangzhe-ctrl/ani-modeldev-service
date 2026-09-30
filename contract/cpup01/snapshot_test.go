package cpup01_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

// This synthetic fixture exercises serialization only. Its .test registry and
// example identities are not an ENV handoff or a usable Release.
func snapshotFixture() cpup01.Snapshot {
	version := "input-version-1"
	return cpup01.Snapshot{
		SchemaVersion: cpup01.SnapshotSchemaVersion,
		Kind:          "GENERAL_TRAINING", DeliveryMode: "SAVE_ARTIFACTS",
		Release: cpup01.ReleaseSnapshot{
			ReleaseID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", ReleaseDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			PresetID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", AcceptedBindingGeneration: 7,
			PipelineID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", PipelineVersionID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
			PipelineIRSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Runtime:          cpup01.RuntimeRef{Name: "cpu-runtime-v1", Kind: "ClusterTrainingRuntime", APIGroup: "trainer.kubeflow.org", ContentSHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", TargetJobs: []string{"trainer"}},
		},
		Input:            cpup01.InputRef{InputVersionID: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", Object: cpup01.FixedObjectRef{StorageConnectionID: "input-store-v1", Bucket: "cpu-inputs", Key: "fixed/input/data.csv", VersionID: &version, SizeBytes: 192456, SHA256: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}, Format: "CSV", SchemaVersion: "ani.cpu.csv.v1", RowCount: 1024, FeatureCount: 16},
		Program:          cpup01.ProgramRef{ImageVersionID: "ffffffff-ffff-4fff-8fff-ffffffffffff", ImageDigest: "registry.example.test/cpu/mlp@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Command: []string{"python", "/opt/mlp/train.py"}, ResolvedArgs: []string{"--data", "/inputs/data.csv", "--output", "/outputs"}, ResolvedParameters: []cpup01.Parameter{{Name: "learning_rate", Type: "DECIMAL", Value: "0.0100"}, {Name: "epochs", Type: "INTEGER", Value: "3"}, {Name: "batch_size", Type: "INTEGER", Value: "64"}}},
		Resources:        cpup01.CPUResources{Nodes: 1, ProcessesPerNode: 1, RequestMillicpu: 1000, LimitMillicpu: 2000, RequestMemoryBytes: 1073741824, LimitMemoryBytes: 2147483648},
		Environment:      cpup01.EnvironmentBindingSnapshot{BindingID: "11111111-1111-4111-8111-111111111111", BindingDigest: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", ClusterID: "22222222-2222-4222-8222-222222222222", NamespaceName: "cpu-contract-fixture", NamespaceUID: "33333333-3333-4333-8333-333333333333", KFPConnectionRef: "kfp-managed-v1", ExperimentID: "44444444-4444-4444-8444-444444444444", Identities: cpup01.RuntimeIdentityRefs{ModeldevControlIdentityRef: "modeldev-control-v1", KFPStepServiceAccount: "cpu-managed-step", TrainerServiceAccount: "cpu-training", VerifierServiceAccount: "cpu-verifier", TenantProxyIdentity: "tenant:contract-fixture"}},
		Workspace:        cpup01.WorkspaceContract{Mode: "EXECUTION_PVC", StorageClass: "task-workspace", CapacityBytes: 2147483648, InputSubpath: "inputs", TrainingSubpath: "training", ReportsSubpath: "reports", PublicationSubpath: "publication"},
		PublicationScope: cpup01.StorageScope{StorageConnectionID: "artifact-store-v1", Bucket: "cpu-artifacts", ApprovedPrefix: "tenant-fixed/executions", CredentialReference: "modeldev-publish-v1"},
		OutputContract:   cpup01.OutputContract{SchemaVersion: "ani.cpu.output.v1", OutputKind: "CHECKPOINT", DeliveryMode: "SAVE_ARTIFACTS", RequiredFiles: []cpup01.RequiredOutput{{Role: "SUMMARY", RelativePath: "summary.json", MaxSizeBytes: 4096}, {Role: "CHECKPOINT", RelativePath: "model.pt", MaxSizeBytes: 1048576}, {Role: "MODEL_CONFIG", RelativePath: "model_config.json", MaxSizeBytes: 4096}, {Role: "METRICS", RelativePath: "metrics.jsonl", MaxSizeBytes: 65536}}, MaxFileCount: 16, MaxTotalBytes: 2097152, CreateTarBundle: true},
		DeadlineAt:       time.Date(2026, 9, 30, 18, 0, 0, 0, time.FixedZone("fixture", 8*60*60)),
	}
}

// The literal is independently specified by cpu-p01-snapshot.md, not marshaled
// by the code under test. Its SHA256 was independently computed by Python
// hashlib on Fedora from the corrected path-sorted literal at 0fd3ae0.
func TestSnapshotCanonicalizesCompleteFrozenConfiguration(t *testing.T) {
	snapshot := snapshotFixture()
	canonical, err := snapshot.Canonical()
	if err != nil {
		t.Fatalf("valid frozen CPU snapshot rejected: %v", err)
	}
	want := `{"schema_version":"ani.modeldev.execution-spec.v1","kind":"GENERAL_TRAINING","delivery_mode":"SAVE_ARTIFACTS","release":{"release_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","release_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","preset_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","accepted_binding_generation":"7","pipeline_id":"cccccccc-cccc-4ccc-8ccc-cccccccccccc","pipeline_version_id":"dddddddd-dddd-4ddd-8ddd-dddddddddddd","pipeline_ir_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","runtime":{"name":"cpu-runtime-v1","kind":"ClusterTrainingRuntime","api_group":"trainer.kubeflow.org","content_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","target_jobs":["trainer"]}},"input":{"input_version_id":"eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee","object":{"storage_connection_id":"input-store-v1","bucket":"cpu-inputs","key":"fixed/input/data.csv","version_id":"input-version-1","size_bytes":"192456","sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"},"format":"CSV","schema_version":"ani.cpu.csv.v1","row_count":1024,"feature_count":16},"program":{"image_version_id":"ffffffff-ffff-4fff-8fff-ffffffffffff","image_digest":"registry.example.test/cpu/mlp@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","command":["python","/opt/mlp/train.py"],"resolved_args":["--data","/inputs/data.csv","--output","/outputs"],"resolved_parameters":[{"name":"batch_size","type":"INTEGER","value":"64"},{"name":"epochs","type":"INTEGER","value":"3"},{"name":"learning_rate","type":"DECIMAL","value":"0.01"}]},"resources":{"nodes":1,"processes_per_node":1,"request_millicpu":"1000","limit_millicpu":"2000","request_memory_bytes":"1073741824","limit_memory_bytes":"2147483648"},"environment":{"binding_id":"11111111-1111-4111-8111-111111111111","binding_digest":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","cluster_id":"22222222-2222-4222-8222-222222222222","namespace_name":"cpu-contract-fixture","namespace_uid":"33333333-3333-4333-8333-333333333333","kfp_connection_ref":"kfp-managed-v1","experiment_id":"44444444-4444-4444-8444-444444444444","identities":{"modeldev_control_identity_ref":"modeldev-control-v1","kfp_step_service_account":"cpu-managed-step","trainer_service_account":"cpu-training","verifier_service_account":"cpu-verifier","tenant_proxy_identity":"tenant:contract-fixture"}},"workspace":{"mode":"EXECUTION_PVC","storage_class":"task-workspace","capacity_bytes":"2147483648","input_subpath":"inputs","training_subpath":"training","reports_subpath":"reports","publication_subpath":"publication"},"publication_scope":{"storage_connection_id":"artifact-store-v1","bucket":"cpu-artifacts","approved_prefix":"tenant-fixed/executions","credential_reference":"modeldev-publish-v1"},"output_contract":{"schema_version":"ani.cpu.output.v1","output_kind":"CHECKPOINT","delivery_mode":"SAVE_ARTIFACTS","required_files":[{"role":"METRICS","relative_path":"metrics.jsonl","max_size_bytes":"65536"},{"role":"CHECKPOINT","relative_path":"model.pt","max_size_bytes":"1048576"},{"role":"MODEL_CONFIG","relative_path":"model_config.json","max_size_bytes":"4096"},{"role":"SUMMARY","relative_path":"summary.json","max_size_bytes":"4096"}],"max_file_count":16,"max_total_bytes":"2097152","create_tar_bundle":true},"deadline_at":"2026-09-30T10:00:00Z"}`
	if string(canonical) != want {
		t.Fatalf("canonical snapshot = %s; want %s", canonical, want)
	}
	digest, err := snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if digest != "972dee14e65202d5d4da7da199cf5f37139b535701a0cb1b3de4d8ef8a5170b9" {
		t.Fatalf("snapshot SHA256 = %q", digest)
	}
	if snapshot.Program.ResolvedParameters[0].Value != "0.0100" || snapshot.OutputContract.RequiredFiles[0].RelativePath != "summary.json" {
		t.Fatal("canonicalization mutated caller-owned configuration")
	}
}

func TestSnapshotRejectsIncompleteOrUnsafeFrozenConfiguration(t *testing.T) {
	// Structural validation must remain usable for historical snapshots after
	// their deadline. Live admission checks accepted_at/deadline separately.
	t.Run("valid historical snapshot", func(t *testing.T) {
		snapshot := snapshotFixture()
		snapshot.DeadlineAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := snapshot.Validate(); err != nil {
			t.Fatalf("complete historical snapshot rejected: %v", err)
		}
	})
	t.Run("fixed managed input copy", func(t *testing.T) {
		snapshot := snapshotFixture()
		fixed := true
		snapshot.Input.Object.VersionID = nil
		snapshot.Input.Object.ImmutableCopy = &fixed
		if err := snapshot.Validate(); err != nil {
			t.Fatalf("managed immutable copy rejected: %v", err)
		}
	})
	cases := []struct {
		name   string
		change func(*cpup01.Snapshot)
	}{
		{"empty snapshot", func(s *cpup01.Snapshot) { *s = cpup01.Snapshot{} }},
		{"wrong schema", func(s *cpup01.Snapshot) { s.SchemaVersion = "ani.modeldev.execution-spec.v2" }},
		{"unsupported kind", func(s *cpup01.Snapshot) { s.Kind = "FINETUNING" }},
		{"unsupported delivery", func(s *cpup01.Snapshot) { s.DeliveryMode = "REGISTER_MODEL" }},
		{"missing release", func(s *cpup01.Snapshot) { s.Release.ReleaseID = "" }},
		{"missing release digest", func(s *cpup01.Snapshot) { s.Release.ReleaseDigest = "" }},
		{"missing preset", func(s *cpup01.Snapshot) { s.Release.PresetID = "" }},
		{"no binding generation", func(s *cpup01.Snapshot) { s.Release.AcceptedBindingGeneration = 0 }},
		{"missing pipeline", func(s *cpup01.Snapshot) { s.Release.PipelineID = "" }},
		{"unknown pipeline version", func(s *cpup01.Snapshot) { s.Release.PipelineVersionID = "UNKNOWN" }},
		{"missing pipeline digest", func(s *cpup01.Snapshot) { s.Release.PipelineIRSHA256 = "" }},
		{"missing runtime name", func(s *cpup01.Snapshot) { s.Release.Runtime.Name = "" }},
		{"unsupported runtime kind", func(s *cpup01.Snapshot) { s.Release.Runtime.Kind = "PyTorchJob" }},
		{"wrong runtime api group", func(s *cpup01.Snapshot) { s.Release.Runtime.APIGroup = "example.test" }},
		{"runtime without content hash", func(s *cpup01.Snapshot) { s.Release.Runtime.ContentSHA256 = "" }},
		{"runtime without target job", func(s *cpup01.Snapshot) { s.Release.Runtime.TargetJobs = nil }},
		{"duplicate runtime targets", func(s *cpup01.Snapshot) { s.Release.Runtime.TargetJobs = []string{"trainer", "trainer"} }},
		{"invalid runtime target", func(s *cpup01.Snapshot) { s.Release.Runtime.TargetJobs = []string{"../trainer"} }},
		{"missing input id", func(s *cpup01.Snapshot) { s.Input.InputVersionID = "" }},
		{"wrong input schema", func(s *cpup01.Snapshot) { s.Input.SchemaVersion = "arbitrary" }},
		{"wrong input format", func(s *cpup01.Snapshot) { s.Input.Format = "JSON" }},
		{"wrong input rows", func(s *cpup01.Snapshot) { s.Input.RowCount = 1023 }},
		{"wrong feature count", func(s *cpup01.Snapshot) { s.Input.FeatureCount = 17 }},
		{"missing input storage", func(s *cpup01.Snapshot) { s.Input.Object.StorageConnectionID = "" }},
		{"missing input bucket", func(s *cpup01.Snapshot) { s.Input.Object.Bucket = "" }},
		{"input key traversal", func(s *cpup01.Snapshot) { s.Input.Object.Key = "../data.csv" }},
		{"mutable input", func(s *cpup01.Snapshot) { s.Input.Object.VersionID = nil }},
		{"empty input version", func(s *cpup01.Snapshot) { v := ""; s.Input.Object.VersionID = &v }},
		{"null S3 input version", func(s *cpup01.Snapshot) { v := "null"; s.Input.Object.VersionID = &v }},
		{"false immutable copy", func(s *cpup01.Snapshot) {
			v := false
			s.Input.Object.VersionID = nil
			s.Input.Object.ImmutableCopy = &v
		}},
		{"two immutability claims", func(s *cpup01.Snapshot) { v := true; s.Input.Object.ImmutableCopy = &v }},
		{"empty input bytes", func(s *cpup01.Snapshot) { s.Input.Object.SizeBytes = 0 }},
		{"negative input bytes", func(s *cpup01.Snapshot) { s.Input.Object.SizeBytes = -1 }},
		{"missing input sha", func(s *cpup01.Snapshot) { s.Input.Object.SHA256 = "" }},
		{"missing image version", func(s *cpup01.Snapshot) { s.Program.ImageVersionID = "" }},
		{"missing image", func(s *cpup01.Snapshot) { s.Program.ImageDigest = "" }},
		{"floating image tag", func(s *cpup01.Snapshot) { s.Program.ImageDigest = "registry.example.test/cpu/mlp:latest" }},
		{"short image digest", func(s *cpup01.Snapshot) { s.Program.ImageDigest = "registry.example.test/cpu/mlp@sha256:eeee" }},
		{"missing command", func(s *cpup01.Snapshot) { s.Program.Command = nil }},
		{"empty command element", func(s *cpup01.Snapshot) { s.Program.Command = []string{"python", ""} }},
		{"command control character", func(s *cpup01.Snapshot) { s.Program.Command = []string{"python\x00"} }},
		{"argument control character", func(s *cpup01.Snapshot) { s.Program.ResolvedArgs = []string{"--data\n/other"} }},
		{"missing resolved parameters", func(s *cpup01.Snapshot) { s.Program.ResolvedParameters = nil }},
		{"incomplete resolved parameters", func(s *cpup01.Snapshot) { s.Program.ResolvedParameters = s.Program.ResolvedParameters[:2] }},
		{"duplicate resolved parameter", func(s *cpup01.Snapshot) { s.Program.ResolvedParameters[1] = s.Program.ResolvedParameters[0] }},
		{"wrong resolved recipe", func(s *cpup01.Snapshot) { s.Program.ResolvedParameters[1].Value = "4" }},
		{"unregistered resolved parameter", func(s *cpup01.Snapshot) { s.Program.ResolvedParameters[1].Name = "command" }},
		{"wrong resolved parameter type", func(s *cpup01.Snapshot) { s.Program.ResolvedParameters[0].Type = "STRING" }},
		{"out of range learning rate", func(s *cpup01.Snapshot) { s.Program.ResolvedParameters[0].Value = "0.1001" }},
		{"multi node", func(s *cpup01.Snapshot) { s.Resources.Nodes = 2 }},
		{"zero process", func(s *cpup01.Snapshot) { s.Resources.ProcessesPerNode = 0 }},
		{"multi process", func(s *cpup01.Snapshot) { s.Resources.ProcessesPerNode = 2 }},
		{"zero cpu request", func(s *cpup01.Snapshot) { s.Resources.RequestMillicpu = 0 }},
		{"cpu limit below request", func(s *cpup01.Snapshot) { s.Resources.LimitMillicpu = s.Resources.RequestMillicpu - 1 }},
		{"zero memory request", func(s *cpup01.Snapshot) { s.Resources.RequestMemoryBytes = 0 }},
		{"memory limit below request", func(s *cpup01.Snapshot) { s.Resources.LimitMemoryBytes = s.Resources.RequestMemoryBytes - 1 }},
		{"unknown environment binding", func(s *cpup01.Snapshot) { s.Environment.BindingID = "UNKNOWN" }},
		{"missing binding digest", func(s *cpup01.Snapshot) { s.Environment.BindingDigest = "" }},
		{"missing cluster", func(s *cpup01.Snapshot) { s.Environment.ClusterID = "" }},
		{"missing namespace", func(s *cpup01.Snapshot) { s.Environment.NamespaceName = "" }},
		{"zero namespace uid", func(s *cpup01.Snapshot) { s.Environment.NamespaceUID = "00000000-0000-0000-0000-000000000000" }},
		{"kfp not ready", func(s *cpup01.Snapshot) { s.Environment.KFPConnectionRef = "NOT_READY" }},
		{"missing experiment", func(s *cpup01.Snapshot) { s.Environment.ExperimentID = "" }},
		{"missing control identity", func(s *cpup01.Snapshot) { s.Environment.Identities.ModeldevControlIdentityRef = "" }},
		{"missing step identity", func(s *cpup01.Snapshot) { s.Environment.Identities.KFPStepServiceAccount = "" }},
		{"missing trainer identity", func(s *cpup01.Snapshot) { s.Environment.Identities.TrainerServiceAccount = "" }},
		{"missing verifier identity", func(s *cpup01.Snapshot) { s.Environment.Identities.VerifierServiceAccount = "" }},
		{"missing tenant proxy", func(s *cpup01.Snapshot) { s.Environment.Identities.TenantProxyIdentity = "" }},
		{"trainer shares step credential", func(s *cpup01.Snapshot) {
			s.Environment.Identities.TrainerServiceAccount = s.Environment.Identities.KFPStepServiceAccount
		}},
		{"verifier shares step credential", func(s *cpup01.Snapshot) {
			s.Environment.Identities.VerifierServiceAccount = s.Environment.Identities.KFPStepServiceAccount
		}},
		{"verifier shares trainer credential", func(s *cpup01.Snapshot) {
			s.Environment.Identities.VerifierServiceAccount = s.Environment.Identities.TrainerServiceAccount
		}},
		{"invalid service account", func(s *cpup01.Snapshot) { s.Environment.Identities.TrainerServiceAccount = "tenant/other" }},
		{"unknown workspace mode", func(s *cpup01.Snapshot) { s.Workspace.Mode = "UNKNOWN" }},
		{"unset storage class", func(s *cpup01.Snapshot) { s.Workspace.StorageClass = "UNSET" }},
		{"zero workspace capacity", func(s *cpup01.Snapshot) { s.Workspace.CapacityBytes = 0 }},
		{"workspace root input", func(s *cpup01.Snapshot) { s.Workspace.InputSubpath = "/" }},
		{"workspace output traversal", func(s *cpup01.Snapshot) { s.Workspace.TrainingSubpath = "../training" }},
		{"workspace overlapping scopes", func(s *cpup01.Snapshot) { s.Workspace.TrainingSubpath = "inputs/training" }},
		{"workspace equal scopes", func(s *cpup01.Snapshot) { s.Workspace.PublicationSubpath = s.Workspace.TrainingSubpath }},
		{"workspace backslash", func(s *cpup01.Snapshot) { s.Workspace.ReportsSubpath = `reports\other` }},
		{"missing publication connection", func(s *cpup01.Snapshot) { s.PublicationScope.StorageConnectionID = "" }},
		{"missing publication bucket", func(s *cpup01.Snapshot) { s.PublicationScope.Bucket = "" }},
		{"publication tenant root", func(s *cpup01.Snapshot) { s.PublicationScope.ApprovedPrefix = "" }},
		{"publication wildcard", func(s *cpup01.Snapshot) { s.PublicationScope.ApprovedPrefix = "tenant/*" }},
		{"missing publication credential reference", func(s *cpup01.Snapshot) { s.PublicationScope.CredentialReference = "" }},
		{"wrong output schema", func(s *cpup01.Snapshot) { s.OutputContract.SchemaVersion = "arbitrary" }},
		{"wrong output kind", func(s *cpup01.Snapshot) { s.OutputContract.OutputKind = "LORA_ADAPTER" }},
		{"output delivery mismatch", func(s *cpup01.Snapshot) { s.OutputContract.DeliveryMode = "REGISTER_MODEL" }},
		{"no required outputs", func(s *cpup01.Snapshot) { s.OutputContract.RequiredFiles = nil }},
		{"missing required output", func(s *cpup01.Snapshot) { s.OutputContract.RequiredFiles = s.OutputContract.RequiredFiles[:3] }},
		{"wrong checkpoint name", func(s *cpup01.Snapshot) { s.OutputContract.RequiredFiles[1].RelativePath = "checkpoint.pt" }},
		{"output path traversal", func(s *cpup01.Snapshot) { s.OutputContract.RequiredFiles[1].RelativePath = "../model.pt" }},
		{"wrong output role", func(s *cpup01.Snapshot) { s.OutputContract.RequiredFiles[1].Role = "LOG" }},
		{"duplicate required output", func(s *cpup01.Snapshot) { s.OutputContract.RequiredFiles[0] = s.OutputContract.RequiredFiles[1] }},
		{"zero output file limit", func(s *cpup01.Snapshot) { s.OutputContract.RequiredFiles[1].MaxSizeBytes = 0 }},
		{"file exceeds total bound", func(s *cpup01.Snapshot) {
			s.OutputContract.RequiredFiles[1].MaxSizeBytes = s.OutputContract.MaxTotalBytes + 1
		}},
		{"file count below required outputs", func(s *cpup01.Snapshot) { s.OutputContract.MaxFileCount = 3 }},
		{"no total output bound", func(s *cpup01.Snapshot) { s.OutputContract.MaxTotalBytes = 0 }},
		{"zero deadline", func(s *cpup01.Snapshot) { s.DeadlineAt = time.Time{} }},
		{"deadline before protobuf timestamp range", func(s *cpup01.Snapshot) { s.DeadlineAt = time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{"unrepresentable deadline", func(s *cpup01.Snapshot) { s.DeadlineAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			snapshot := snapshotFixture()
			test.change(&snapshot)
			if err := snapshot.Validate(); !errors.Is(err, cpup01.ErrInvalidArgument) {
				t.Errorf("invalid snapshot was not rejected by Validate: %v", err)
			}
			if canonical, err := snapshot.Canonical(); !errors.Is(err, cpup01.ErrInvalidArgument) || len(canonical) != 0 {
				t.Errorf("invalid snapshot produced %d canonical bytes: %v", len(canonical), err)
			}
			if digest, err := snapshot.Digest(); !errors.Is(err, cpup01.ErrInvalidArgument) || digest != "" {
				t.Errorf("invalid snapshot produced an execution_spec_hash: %q, %v", digest, err)
			}
		})
	}
}
