package cpup01

import (
	"errors"
	"time"
)

const SnapshotSchemaVersion = "ani.modeldev.execution-spec.v1"

// Snapshot is the complete immutable admission configuration. It deliberately
// excludes operation/execution/tenant/actor: those belong to the authenticated
// command envelope. It also excludes all resources created after admission.
type Snapshot struct {
	SchemaVersion string `json:"schema_version"`
	Kind string `json:"kind"`
	DeliveryMode string `json:"delivery_mode"`
	Release ReleaseSnapshot `json:"release"`
	Input InputRef `json:"input"`
	Program ProgramRef `json:"program"`
	Resources CPUResources `json:"resources"`
	Environment EnvironmentBindingSnapshot `json:"environment"`
	Workspace WorkspaceContract `json:"workspace"`
	PublicationScope StorageScope `json:"publication_scope"`
	OutputContract OutputContract `json:"output_contract"`
	DeadlineAt time.Time `json:"deadline_at"`
}

type ReleaseSnapshot struct {
	ReleaseID string `json:"release_id"`
	ReleaseDigest string `json:"release_digest"`
	PresetID string `json:"preset_id"`
	AcceptedBindingGeneration uint64 `json:"accepted_binding_generation,string"`
	PipelineID string `json:"pipeline_id"`
	PipelineVersionID string `json:"pipeline_version_id"`
	PipelineIRSHA256 string `json:"pipeline_ir_sha256"`
	Runtime RuntimeRef `json:"runtime"`
}

type RuntimeRef struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	APIGroup string `json:"api_group"`
	ContentSHA256 string `json:"content_sha256"`
	TargetJobs []string `json:"target_jobs"`
}

type FixedObjectRef struct {
	StorageConnectionID string `json:"storage_connection_id"`
	Bucket string `json:"bucket"`
	Key string `json:"key"`
	VersionID *string `json:"version_id,omitempty"`
	ImmutableCopy *bool `json:"immutable_copy,omitempty"`
	SizeBytes int64 `json:"size_bytes,string"`
	SHA256 string `json:"sha256"`
}

type InputRef struct {
	InputVersionID string `json:"input_version_id"`
	Object FixedObjectRef `json:"object"`
	Format string `json:"format"`
	SchemaVersion string `json:"schema_version"`
	RowCount uint32 `json:"row_count"`
	FeatureCount uint32 `json:"feature_count"`
}

type ProgramRef struct {
	ImageVersionID string `json:"image_version_id"`
	ImageDigest string `json:"image_digest"`
	Command []string `json:"command"`
	ResolvedArgs []string `json:"resolved_args"`
	ResolvedParameters []Parameter `json:"resolved_parameters"`
}

type CPUResources struct {
	Nodes uint32 `json:"nodes"`
	ProcessesPerNode uint32 `json:"processes_per_node"`
	RequestMillicpu int64 `json:"request_millicpu,string"`
	LimitMillicpu int64 `json:"limit_millicpu,string"`
	RequestMemoryBytes int64 `json:"request_memory_bytes,string"`
	LimitMemoryBytes int64 `json:"limit_memory_bytes,string"`
}

type RuntimeIdentityRefs struct {
	ModeldevControlIdentityRef string `json:"modeldev_control_identity_ref"`
	KFPStepServiceAccount string `json:"kfp_step_service_account"`
	TrainerServiceAccount string `json:"trainer_service_account"`
	VerifierServiceAccount string `json:"verifier_service_account"`
	TenantProxyIdentity string `json:"tenant_proxy_identity"`
}

type EnvironmentBindingSnapshot struct {
	BindingID string `json:"binding_id"`
	BindingDigest string `json:"binding_digest"`
	ClusterID string `json:"cluster_id"`
	NamespaceName string `json:"namespace_name"`
	NamespaceUID string `json:"namespace_uid"`
	KFPConnectionRef string `json:"kfp_connection_ref"`
	ExperimentID string `json:"experiment_id"`
	Identities RuntimeIdentityRefs `json:"identities"`
}

type WorkspaceContract struct {
	Mode string `json:"mode"`
	StorageClass string `json:"storage_class"`
	CapacityBytes int64 `json:"capacity_bytes,string"`
	InputSubpath string `json:"input_subpath"`
	TrainingSubpath string `json:"training_subpath"`
	ReportsSubpath string `json:"reports_subpath"`
	PublicationSubpath string `json:"publication_subpath"`
}

type StorageScope struct {
	StorageConnectionID string `json:"storage_connection_id"`
	Bucket string `json:"bucket"`
	ApprovedPrefix string `json:"approved_prefix"`
	CredentialReference string `json:"credential_reference"`
}

type RequiredOutput struct {
	Role string `json:"role"`
	RelativePath string `json:"relative_path"`
	MaxSizeBytes int64 `json:"max_size_bytes,string"`
}

type OutputContract struct {
	SchemaVersion string `json:"schema_version"`
	OutputKind string `json:"output_kind"`
	DeliveryMode string `json:"delivery_mode"`
	RequiredFiles []RequiredOutput `json:"required_files"`
	MaxFileCount uint32 `json:"max_file_count"`
	MaxTotalBytes int64 `json:"max_total_bytes,string"`
	CreateTarBundle bool `json:"create_tar_bundle"`
}

var errSnapshotNotImplemented = errors.New("snapshot canonicalization not implemented")

// Canonical returns v1 canonical UTF-8 bytes after validating the frozen
// configuration. It never resolves current defaults or performs network I/O.
func (snapshot Snapshot) Canonical() ([]byte, error) {
	return nil, errSnapshotNotImplemented
}

// Digest returns lowercase SHA256 of Canonical, with no trailing newline.
func (snapshot Snapshot) Digest() (string, error) {
	return "", errSnapshotNotImplemented
}

// Validate checks the local contract only. A valid shape is not evidence that
// an environment, immutable object, image, or Runtime actually exists.
func (snapshot Snapshot) Validate() error {
	return errSnapshotNotImplemented
}
