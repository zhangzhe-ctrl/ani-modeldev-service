package objectstore

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// VerifyCSV verifies the exact version's bytes and fixed CPU input structure.
// Import authorization, version fixation and durable READY are separate steps.
func (v *Verifier) VerifyCSV(context.Context, cpup01.StorageScope, cpup01.FixedObjectRef) (biz.VerifiedCSV, error) {
	return biz.VerifiedCSV{}, biz.ErrInputVerification
}
