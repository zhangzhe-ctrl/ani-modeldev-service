// Package cpup01 defines the versioned CPU-P01 admission contract shared by
// Governance and ModelDev. It contains no transport, persistence or runtime.
package cpup01

import "errors"

var ErrInvalidArgument = errors.New("INVALID_ARGUMENT")

// Parameter is an explicitly supplied, typed registered-program parameter.
// Value uses a decimal string even for integer values; it never contains JSON.
type Parameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Value string `json:"value"`
}

// Intent preserves optional-field presence before any Release defaults resolve.
// Tenant, actor and idempotency key belong to the trusted admission scope, not
// the intent. In particular this object cannot select a namespace or command.
type Intent struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	PresetID string `json:"preset_id"`
	DatasetVersionID string `json:"dataset_version_id"`
	ImageVersionID *string `json:"image_version_id,omitempty"`
	GeneralParameters *[]Parameter `json:"general_parameters,omitempty"`
	SourceExecutionID *string `json:"source_execution_id,omitempty"`
}

// ParseIntent rejects ambiguous JSON before computing an admission hash.
func ParseIntent(raw []byte) (Intent, error) {
	return Intent{}, ErrInvalidArgument
}

// CanonicalIntent returns the versioned, deterministic intent JSON and its
// lowercase SHA256. It does not resolve or read a current Release.
func CanonicalIntent(intent Intent) ([]byte, string, error) {
	return nil, "", ErrInvalidArgument
}
