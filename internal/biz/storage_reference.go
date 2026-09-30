package biz

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidStorageReference checks the shape of owner-selected storage identifiers
// and version references. It does not authorize access or resolve credentials.
func ValidStorageReference(value string) bool {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// ValidStorageKey keeps a key or approved prefix within its relative root.
// Both the import boundary and the actual object reader enforce this shape.
func ValidStorageKey(value string) bool {
	return ValidStorageReference(value) && len(value) <= 1024 && value != "." && value != ".." && !strings.HasPrefix(value, "../") && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && path.Clean(value) == value
}
