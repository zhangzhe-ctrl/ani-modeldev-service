package biz

import (
	"errors"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var ErrInvalidWorkspaceOutput = errors.New("INVALID_WORKSPACE_OUTPUT")

// CollectedFile describes bytes independently read by the collector. A
// workload-supplied result.json is not authoritative for these values.
type CollectedFile = cpup01.OutputFile

// CollectedOutput is a workspace observation, not a publication record or proof
// that writers have stopped. Upload and remote-byte verification are separate.
type CollectedOutput struct {
	ExecutionID       string
	ExecutionSpecHash string
	StorageState      string
	Files             []CollectedFile
	Manifest          []byte
	ManifestSHA256    string
}
