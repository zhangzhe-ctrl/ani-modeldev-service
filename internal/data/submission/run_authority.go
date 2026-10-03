package submission

import (
	"context"
	"errors"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

var errRunAuthorityNotImplemented = errors.New("RUN_AUTHORITY_NOT_IMPLEMENTED")

func (repository *Repository) BindRunAuthority(context.Context, biz.RunAuthorityCandidate) (biz.RunAuthorityReceipt, error) {
	return biz.RunAuthorityReceipt{}, errRunAuthorityNotImplemented
}

func (repository *Repository) GetRunAuthority(context.Context, string, string) (biz.RunAuthority, error) {
	return biz.RunAuthority{}, errRunAuthorityNotImplemented
}
