package runtimeproof

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// VerifyOwnerWritersAbsent observes owner-initiated close independently of a
// managed close Pod and its token.
func (verifier *Verifier) VerifyOwnerWritersAbsent(ctx context.Context, execution biz.Execution, authority biz.RunAuthorityCandidate, workspace *biz.WorkspaceBinding) (biz.ManagedCloseEvidence, error) {
	return biz.ManagedCloseEvidence{}, biz.ErrRuntimeNotReady
}
