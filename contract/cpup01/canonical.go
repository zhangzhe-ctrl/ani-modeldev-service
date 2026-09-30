package cpup01

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// encodeCanonicalJSON preserves v1 UTF-8 JSON bytes without HTML escapes or a
// trailing newline. Callers validate and order their own contract fields first.
func encodeCanonicalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

// canonicalParameters returns a sorted copy; an explicit empty selection stays
// a JSON array. The caller retains responsibility for optional-field presence.
func canonicalParameters(values []Parameter) []Parameter {
	parameters := append([]Parameter{}, values...)
	for i := range parameters {
		if parameters[i].Type == "DECIMAL" && strings.Contains(parameters[i].Value, ".") {
			parameters[i].Value = strings.TrimRight(strings.TrimRight(parameters[i].Value, "0"), ".")
		}
	}
	sort.Slice(parameters, func(i, j int) bool { return parameters[i].Name < parameters[j].Name })
	return parameters
}
