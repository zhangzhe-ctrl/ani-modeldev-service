package submission

import (
	"context"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// RecordSubmissionConfirmed is the explicit first-behavior persistence stub.
// The future implementation stores an internal observation of the original
// attempt; it cannot authenticate a KFP response or establish Run authority.
func (repository *Repository) RecordSubmissionConfirmed(ctx context.Context, permit biz.PipelineSendPermit, observation biz.PipelineSubmissionObservation, observedAt time.Time) (biz.PipelineConfirmationReceipt, error) {
	return biz.PipelineConfirmationReceipt{}, biz.ErrPersistence
}
