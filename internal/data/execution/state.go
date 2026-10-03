package execution

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
)

// The Admission, Close and version have already been read under this identity
// lock or RR snapshot. Reuse the submission adapter's complete persisted-fact
// validation; missing and corrupt submission facts are different outcomes.
func executionStates(ctx context.Context, transaction pgx.Tx, execution biz.Execution) (biz.ExecutionStates, error) {
	facts := biz.ExecutionStateFacts{}
	if execution.Close != nil {
		facts.Close = &execution.Close.State
	}
	dispatch, err := submission.ReadInTransaction(ctx, transaction, execution.TenantID, execution.ExecutionID)
	switch {
	case err == nil:
		if dispatch.OwnerRevision != execution.OwnerRevision {
			return biz.ExecutionStates{}, biz.ErrPersistence
		}
		facts.Submission = &dispatch.State
	case errors.Is(err, biz.ErrExecutionNotFound):
		// No reservation exists in this same snapshot.
	default:
		return biz.ExecutionStates{}, biz.ErrPersistence
	}
	states, err := facts.Project()
	if err != nil {
		return biz.ExecutionStates{}, biz.ErrPersistence
	}
	return states, nil
}
