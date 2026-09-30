package biz

import "errors"

var ErrInvalidWorkspaceOutput = errors.New("INVALID_WORKSPACE_OUTPUT")

// CollectedFile describes bytes independently read by the collector. A
// workload-supplied result.json is not authoritative for these values.
type CollectedFile struct {
	RelativePath string
	Role         string
	SizeBytes    int64
	SHA256       string
}

// CollectedOutput is a workspace observation, not a publication record or proof
// that writers have stopped. Upload and remote-byte verification are separate.
type CollectedOutput struct {
	ExecutionID       string
	ExecutionSpecHash string
	StorageState      string
	Files             []CollectedFile
}
