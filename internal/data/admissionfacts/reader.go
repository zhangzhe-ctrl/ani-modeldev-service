// Package admissionfacts loads explicitly pinned private admission facts.
// Loading bytes is not evidence that an environment or identity is usable.
package admissionfacts

import (
	"context"
	"errors"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// FileSource comes only from trusted startup configuration. SHA256 identifies
// the actual file bytes and is neither an ENV binding nor a Release digest.
type FileSource struct {
	Path   string
	SHA256 string
}

// document is the private file format, not a public RPC or shared contract.
// Evidence references locate independently reviewed source material. Their
// presence and digest shape never establish a successful capability test.
type document struct {
	SchemaVersion       string                            `json:"schema_version"`
	ResourceTenantID    string                            `json:"resource_tenant_id"`
	ReleaseID           string                            `json:"release_id"`
	ReleaseDigest       string                            `json:"release_digest"`
	Environment         cpup01.EnvironmentBindingSnapshot `json:"environment"`
	InputScope          cpup01.StorageScope                `json:"input_scope"`
	PublicationScope    cpup01.StorageScope                `json:"publication_scope"`
	Runtime             cpup01.RuntimeRef                  `json:"runtime"`
	Workspace           cpup01.WorkspaceContract           `json:"workspace"`
	InputFilePath       string                            `json:"input_file_path"`
	OutputDirectoryPath string                            `json:"output_directory_path"`
	EnvironmentEvidence evidenceReference                 `json:"environment_evidence"`
	ApplicationEvidence evidenceReference                 `json:"application_evidence"`
}

type evidenceReference struct {
	Reference string `json:"reference"`
	SHA256    string `json:"sha256"`
}

// Reader owns immutable loaded facts, indexed by tenant and explicit Release.
// It does not maintain a current Release pointer or perform environment probes.
type Reader struct{}

var _ biz.AdmissionFactsReader = (*Reader)(nil)

func Load(ctx context.Context, sources []FileSource) (*Reader, error) {
	return nil, errors.New("managed admission facts loading not implemented")
}

func (reader *Reader) ReadAdmissionFacts(ctx context.Context, tenantID, releaseID, releaseDigest string) (biz.TenantAdmissionFacts, error) {
	return biz.TenantAdmissionFacts{}, errors.New("managed admission facts reading not implemented")
}
