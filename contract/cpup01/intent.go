// Package cpup01 defines the versioned CPU-P01 admission contract shared by
// Governance and ModelDev. It contains no transport, persistence or runtime.
package cpup01

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

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
	var intent Intent
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&intent); err != nil {
		return Intent{}, fmt.Errorf("%w: intent JSON", ErrInvalidArgument)
	}
	return intent, nil
}

// CanonicalIntent returns the versioned, deterministic intent JSON and its
// lowercase SHA256. It does not resolve or read a current Release.
func CanonicalIntent(intent Intent) ([]byte, string, error) {
	intent.PresetID = strings.ToLower(intent.PresetID)
	intent.DatasetVersionID = strings.ToLower(intent.DatasetVersionID)
	if intent.ImageVersionID != nil {
		value := strings.ToLower(*intent.ImageVersionID)
		intent.ImageVersionID = &value
	}
	if intent.SourceExecutionID != nil {
		value := strings.ToLower(*intent.SourceExecutionID)
		intent.SourceExecutionID = &value
	}
	if intent.GeneralParameters != nil {
		parameters := append([]Parameter{}, (*intent.GeneralParameters)...)
		for i := range parameters {
			if parameters[i].Type == "DECIMAL" && strings.Contains(parameters[i].Value, ".") {
				parameters[i].Value = strings.TrimRight(strings.TrimRight(parameters[i].Value, "0"), ".")
			}
		}
		sort.Slice(parameters, func(i, j int) bool { return parameters[i].Name < parameters[j].Name })
		intent.GeneralParameters = &parameters
	}
	value := struct {
		Schema string `json:"schema"`
		Intent
	}{Schema: "ani.modeldev.intent.v1", Intent: intent}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, "", fmt.Errorf("%w: intent encoding", ErrInvalidArgument)
	}
	canonical := bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))
	digest := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(digest[:]), nil
}
