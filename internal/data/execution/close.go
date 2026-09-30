package execution

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func (r *Repository) ApplyCloseIntent(context.Context, biz.CloseIntent) (biz.CloseRecord, error) {
	return biz.CloseRecord{}, biz.ErrNotImplemented
}

func (r *Repository) GetCloseIntent(context.Context, string, string) (biz.CloseRecord, error) {
	return biz.CloseRecord{}, biz.ErrNotImplemented
}
