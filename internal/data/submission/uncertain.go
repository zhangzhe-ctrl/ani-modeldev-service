package submission

import (
	"context"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// MarkSubmissionUncertain is a deliberate stub until the original dispatch's
// durable uncertainty has a real PostgreSQL behavior RED. It never sends HTTP.
func (repository *Repository) MarkSubmissionUncertain(context.Context, biz.PipelineSendPermit, time.Time) (biz.PipelineDispatch, error) {
	return biz.PipelineDispatch{}, biz.ErrPersistence
}
