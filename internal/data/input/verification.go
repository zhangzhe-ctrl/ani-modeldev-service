package input

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// RecordVerifiedCSV is an internal persistence boundary for a trusted verifier
// observation. It must match the entire frozen import before publishing READY.
// No request transport exposes a state or verification-proof override.
func (r *Repository) RecordVerifiedCSV(ctx context.Context, expected biz.InputImport, verified biz.VerifiedCSV) (biz.InputVersion, error) {
	return biz.InputVersion{}, biz.ErrPersistence
}
