package catalogue

import (
	"context"
	"errors"
)

// ImportResult records a file installation, not an enabled or verified Release.
type ImportResult struct {
	ReleaseID string
	Digest string
	Created bool
}

// ImportRelease installs canonical bytes under their immutable Release ID.
// A trusted T02 caller must first authorize the operation and verify the real
// environment/material evidence. This adapter does not grant those facts.
func ImportRelease(ctx context.Context, directory string, raw []byte, expectedDigest string) (ImportResult, error) {
	return ImportResult{}, errors.New("RELEASE_IMPORT_NOT_IMPLEMENTED")
}
