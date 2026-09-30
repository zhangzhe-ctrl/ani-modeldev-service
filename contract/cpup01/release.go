package cpup01

import "errors"

const ReleaseSchemaVersion = "ani.modeldev.release.v1"

// ReleaseDocument is a versioned catalogue file, not an accepted execution or
// a current binding. Loading it proves no environment or publication readiness.
// Its digest is external: SHA256 of the complete canonical file bytes.
type ReleaseDocument struct {
	SchemaVersion string `json:"schema_version"`
	ReleaseID string `json:"release_id"`
	PresetID string `json:"preset_id"`
	Kind string `json:"kind"`
	DeliveryMode string `json:"delivery_mode"`
	PipelineID string `json:"pipeline_id"`
	PipelineVersionID string `json:"pipeline_version_id"`
	PipelineIRSHA256 string `json:"pipeline_ir_sha256"`
	Runtime RuntimeRef `json:"runtime"`
	Program ReleaseProgram `json:"program"`
	Resources CPUResources `json:"resources"`
	Workspace WorkspaceContract `json:"workspace"`
	OutputContract OutputContract `json:"output_contract"`
	ExecutionTimeoutSeconds uint32 `json:"execution_timeout_seconds"`
}

// ReleaseProgram is the single registered CPU MLP program in this Release.
// Parameters remain governed by the existing CPU-P01 intent grammar.
type ReleaseProgram struct {
	ImageVersionID string `json:"image_version_id"`
	ImageDigest string `json:"image_digest"`
	Command []string `json:"command"`
	ArgsTemplate []ReleaseArgument `json:"args_template"`
	ParameterContractVersion string `json:"parameter_contract_version"`
	DefaultParameters []Parameter `json:"default_parameters"`
}

// ReleaseArgument is exactly one literal or one whole-argument source token.
// The CPU contract has a finite token set; it never performs shell expansion.
type ReleaseArgument struct {
	Literal string `json:"literal,omitempty"`
	Source string `json:"source,omitempty"`
}

// ParseRelease will accept only canonical v1 JSON and return its actual byte
// digest. The explicit stub is the first catalogue-read TDD candidate.
func ParseRelease(raw []byte) (ReleaseDocument, string, error) {
	return ReleaseDocument{}, "", errors.New("RELEASE_NOT_IMPLEMENTED")
}
