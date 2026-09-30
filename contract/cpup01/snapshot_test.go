package cpup01_test

import (
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
		Kind: "GENERAL_TRAINING", DeliveryMode: "SAVE_ARTIFACTS",
		Release: cpup01.ReleaseSnapshot{
			ReleaseID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", ReleaseDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			PresetID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", AcceptedBindingGeneration: 7,
			PipelineID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", PipelineVersionID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
			PipelineIRSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Runtime: cpup01.RuntimeRef{Name: "cpu-runtime-v1", Kind: "ClusterTrainingRuntime", APIGroup: "trainer.kubeflow.org", ContentSHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", TargetJobs: []string{"trainer"}},
		},
		Input: cpup01.InputRef{InputVersionID: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", Object: cpup01.FixedObjectRef{StorageConnectionID: "input-store-v1", Bucket: "cpu-inputs", Key: "fixed/input/data.csv", VersionID: &version, SizeBytes: 192456, SHA256: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}, Format: "CSV", SchemaVersion: "ani.cpu.csv.v1", RowCount: 1024, FeatureCount: 16},
		Program: cpup01.ProgramRef{ImageVersionID: "ffffffff-ffff-4fff-8fff-ffffffffffff", ImageDigest: "registry.example.test/cpu/mlp@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Command: []string{"python", "/opt/mlp/train.py"}, ResolvedArgs: []string{"--data", "/inputs/data.csv", "--output", "/outputs"}, ResolvedParameters: []cpup01.Parameter{{Name: "learning_rate", Type: "DECIMAL", Value: "0.0100"}, {Name: "epochs", Type: "INTEGER", Value: "3"}, {Name: "batch_size", Type: "INTEGER", Value: "64"}}},
		Resources: cpup01.CPUResources{Nodes: 1, ProcessesPerNode: 1, RequestMillicpu: 1000, LimitMillicpu: 2000, RequestMemoryBytes: 1073741824, LimitMemoryBytes: 2147483648},
		Environment: cpup01.EnvironmentBindingSnapshot{BindingID: "11111111-1111-4111-8111-111111111111", BindingDigest: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", ClusterID: "22222222-2222-4222-8222-222222222222", NamespaceName: "cpu-contract-fixture", NamespaceUID: "33333333-3333-4333-8333-333333333333", KFPConnectionRef: "kfp-managed-v1", ExperimentID: "44444444-4444-4444-8444-444444444444", Identities: cpup01.RuntimeIdentityRefs{ModeldevControlIdentityRef: "modeldev-control-v1", KFPStepServiceAccount: "cpu-managed-step", TrainerServiceAccount: "cpu-training", VerifierServiceAccount: "cpu-verifier", TenantProxyIdentity: "tenant:contract-fixture"}},
		Workspace: cpup01.WorkspaceContract{Mode: "EXECUTION_PVC", StorageClass: "task-workspace", CapacityBytes: 2147483648, InputSubpath: "inputs", TrainingSubpath: "training", ReportsSubpath: "reports", PublicationSubpath: "publication"},
		PublicationScope: cpup01.StorageScope{StorageConnectionID: "artifact-store-v1", Bucket: "cpu-artifacts", ApprovedPrefix: "tenant-fixed/executions", CredentialReference: "modeldev-publish-v1"},
		OutputContract: cpup01.OutputContract{SchemaVersion: "ani.cpu.output.v1", OutputKind: "CHECKPOINT", DeliveryMode: "SAVE_ARTIFACTS", RequiredFiles: []cpup01.RequiredOutput{{Role: "SUMMARY", RelativePath: "summary.json", MaxSizeBytes: 4096}, {Role: "CHECKPOINT", RelativePath: "model.pt", MaxSizeBytes: 1048576}, {Role: "MODEL_CONFIG", RelativePath: "model_config.json", MaxSizeBytes: 4096}, {Role: "METRICS", RelativePath: "metrics.jsonl", MaxSizeBytes: 65536}}, MaxFileCount: 16, MaxTotalBytes: 2097152, CreateTarBundle: true},
		DeadlineAt: time.Date(2026, 9, 30, 18, 0, 0, 0, time.FixedZone("fixture", 8*60*60)),
	}
}

// The literal is independently specified by cpu-p01-snapshot.md, not marshaled
// by the code under test. Obtain its SHA256 independently on Fedora before
// adding the Digest assertion; never invent or derive it in production code.
func TestSnapshotCanonicalizesCompleteFrozenConfiguration(t *testing.T) {
	snapshot := snapshotFixture()
	canonical, err := snapshot.Canonical()
	if err != nil { t.Fatalf("valid frozen CPU snapshot rejected: %v", err) }
	want := `{"schema_version":"ani.modeldev.execution-spec.v1","kind":"GENERAL_TRAINING","delivery_mode":"SAVE_ARTIFACTS","release":{"release_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","release_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","preset_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","accepted_binding_generation":"7","pipeline_id":"cccccccc-cccc-4ccc-8ccc-cccccccccccc","pipeline_version_id":"dddddddd-dddd-4ddd-8ddd-dddddddddddd","pipeline_ir_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","runtime":{"name":"cpu-runtime-v1","kind":"ClusterTrainingRuntime","api_group":"trainer.kubeflow.org","content_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","target_jobs":["trainer"]}},"input":{"input_version_id":"eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee","object":{"storage_connection_id":"input-store-v1","bucket":"cpu-inputs","key":"fixed/input/data.csv","version_id":"input-version-1","size_bytes":"192456","sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"},"format":"CSV","schema_version":"ani.cpu.csv.v1","row_count":1024,"feature_count":16},"program":{"image_version_id":"ffffffff-ffff-4fff-8fff-ffffffffffff","image_digest":"registry.example.test/cpu/mlp@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","command":["python","/opt/mlp/train.py"],"resolved_args":["--data","/inputs/data.csv","--output","/outputs"],"resolved_parameters":[{"name":"batch_size","type":"INTEGER","value":"64"},{"name":"epochs","type":"INTEGER","value":"3"},{"name":"learning_rate","type":"DECIMAL","value":"0.01"}]},"resources":{"nodes":1,"processes_per_node":1,"request_millicpu":"1000","limit_millicpu":"2000","request_memory_bytes":"1073741824","limit_memory_bytes":"2147483648"},"environment":{"binding_id":"11111111-1111-4111-8111-111111111111","binding_digest":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","cluster_id":"22222222-2222-4222-8222-222222222222","namespace_name":"cpu-contract-fixture","namespace_uid":"33333333-3333-4333-8333-333333333333","kfp_connection_ref":"kfp-managed-v1","experiment_id":"44444444-4444-4444-8444-444444444444","identities":{"modeldev_control_identity_ref":"modeldev-control-v1","kfp_step_service_account":"cpu-managed-step","trainer_service_account":"cpu-training","verifier_service_account":"cpu-verifier","tenant_proxy_identity":"tenant:contract-fixture"}},"workspace":{"mode":"EXECUTION_PVC","storage_class":"task-workspace","capacity_bytes":"2147483648","input_subpath":"inputs","training_subpath":"training","reports_subpath":"reports","publication_subpath":"publication"},"publication_scope":{"storage_connection_id":"artifact-store-v1","bucket":"cpu-artifacts","approved_prefix":"tenant-fixed/executions","credential_reference":"modeldev-publish-v1"},"output_contract":{"schema_version":"ani.cpu.output.v1","output_kind":"CHECKPOINT","delivery_mode":"SAVE_ARTIFACTS","required_files":[{"role":"METRICS","relative_path":"metrics.jsonl","max_size_bytes":"65536"},{"role":"CHECKPOINT","relative_path":"model.pt","max_size_bytes":"1048576"},{"role":"MODEL_CONFIG","relative_path":"model_config.json","max_size_bytes":"4096"},{"role":"SUMMARY","relative_path":"summary.json","max_size_bytes":"4096"}],"max_file_count":16,"max_total_bytes":"2097152","create_tar_bundle":true},"deadline_at":"2026-09-30T10:00:00Z"}`
	if string(canonical) != want { t.Fatalf("canonical snapshot = %s; want %s", canonical, want) }
	if snapshot.Program.ResolvedParameters[0].Value != "0.0100" || snapshot.OutputContract.RequiredFiles[0].RelativePath != "summary.json" { t.Fatal("canonicalization mutated caller-owned configuration") }
}
