package submission

import (
	"context"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func (repository *Repository) MarkSubmissionNotSent(ctx context.Context, permit biz.PipelineSendPermit, observedAt time.Time) (biz.PipelineDispatch, error) {
	// Explicit first-behavior stub. No migration/query is presumed available,
	// and a transient NOT_SENT must not be acknowledged as a durable fact.
	return biz.PipelineDispatch{}, biz.ErrPersistence
}
