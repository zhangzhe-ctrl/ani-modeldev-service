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
	"io"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalidArgument = errors.New("INVALID_ARGUMENT")

// Parameter is an explicitly supplied, typed registered-program parameter.
// Value uses a decimal string even for integer values; it never contains JSON.
type Parameter struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Intent preserves optional-field presence before any Release defaults resolve.
// Tenant, actor and idempotency key belong to the trusted admission scope, not
// the intent. In particular this object cannot select a namespace or command.
type Intent struct {
	Name              string       `json:"name"`
	Kind              string       `json:"kind"`
	PresetID          string       `json:"preset_id"`
	DatasetVersionID  string       `json:"dataset_version_id"`
	ImageVersionID    *string      `json:"image_version_id,omitempty"`
	GeneralParameters *[]Parameter `json:"general_parameters,omitempty"`
	SourceExecutionID *string      `json:"source_execution_id,omitempty"`
}

// ParseIntent rejects ambiguous JSON before computing an admission hash.
func ParseIntent(raw []byte) (Intent, error) {
	if len(raw) > 16384 || !utf8.Valid(raw) {
		return Intent{}, fmt.Errorf("%w: intent encoding or size", ErrInvalidArgument)
	}
	scanner := json.NewDecoder(bytes.NewReader(raw))
	if err := checkJSONValue(scanner, 0); err != nil {
		return Intent{}, err
	}
	if _, err := scanner.Token(); err != io.EOF {
		return Intent{}, fmt.Errorf("%w: trailing JSON", ErrInvalidArgument)
	}
	var intent Intent
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&intent); err != nil {
		return Intent{}, fmt.Errorf("%w: intent JSON", ErrInvalidArgument)
	}
	if err := validateIntent(intent); err != nil {
		return Intent{}, err
	}
	return intent, nil
}

// CanonicalIntent returns the versioned, deterministic intent JSON and its
// lowercase SHA256. It does not resolve or read a current Release.
func CanonicalIntent(intent Intent) ([]byte, string, error) {
	if err := validateIntent(intent); err != nil {
		return nil, "", err
	}
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

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func validUUID(value string) bool {
	return uuidPattern.MatchString(value) && value != "00000000-0000-0000-0000-000000000000"
}

func validateIntent(intent Intent) error {
	invalid := func(field string) error { return fmt.Errorf("%w: %s", ErrInvalidArgument, field) }
	if !utf8.ValidString(intent.Name) || utf8.RuneCountInString(intent.Name) < 1 || utf8.RuneCountInString(intent.Name) > 80 || strings.TrimSpace(intent.Name) != intent.Name {
		return invalid("name")
	}
	for _, r := range intent.Name {
		if unicode.IsControl(r) { return invalid("name") }
	}
	if intent.Kind != "GENERAL_TRAINING" { return invalid("kind") }
	if !validUUID(intent.PresetID) || !validUUID(intent.DatasetVersionID) { return invalid("required version ID") }
	if intent.ImageVersionID != nil && !validUUID(*intent.ImageVersionID) { return invalid("image_version_id") }
	if intent.SourceExecutionID != nil && !validUUID(*intent.SourceExecutionID) { return invalid("source_execution_id") }
	if intent.GeneralParameters == nil { return nil }
	if len(*intent.GeneralParameters) > 3 { return invalid("general_parameters") }
	seen := make(map[string]bool)
	for _, parameter := range *intent.GeneralParameters {
		if seen[parameter.Name] { return invalid("duplicate parameter") }
		seen[parameter.Name] = true
		switch parameter.Name {
		case "epochs":
			if parameter.Type != "INTEGER" || parameter.Value != "3" { return invalid("epochs") }
		case "batch_size":
			if parameter.Type != "INTEGER" || parameter.Value != "64" { return invalid("batch_size") }
		case "learning_rate":
			if parameter.Type != "DECIMAL" || len(parameter.Value) > 32 || !decimalPattern.MatchString(parameter.Value) { return invalid("learning_rate") }
			value, ok := new(big.Rat).SetString(parameter.Value)
			if !ok || value.Sign() <= 0 || value.Cmp(big.NewRat(1, 10)) > 0 { return invalid("learning_rate") }
		default:
			return invalid("unregistered parameter")
		}
	}
	return nil
}

// encoding/json alone accepts duplicate keys and nulls. A bounded token pass
// rejects those ambiguities before the ordinary typed decoder handles shape.
func checkJSONValue(decoder *json.Decoder, depth int) error {
	invalid := func() error { return fmt.Errorf("%w: ambiguous JSON", ErrInvalidArgument) }
	if depth > 8 { return invalid() }
	token, err := decoder.Token()
	if err != nil || token == nil { return invalid() }
	delimiter, composite := token.(json.Delim)
	if !composite { return nil }
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok || seen[key] { return invalid() }
			seen[key] = true
			if err := checkJSONValue(decoder, depth+1); err != nil { return err }
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') { return invalid() }
	case '[':
		for decoder.More() {
			if err := checkJSONValue(decoder, depth+1); err != nil { return err }
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') { return invalid() }
	default:
		return invalid()
	}
	return nil
}
