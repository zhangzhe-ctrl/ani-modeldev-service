package submission

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	submissionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission/sqlc"
)

// SubmissionObservationTime samples the database clock after an outbound
// observation, not when KFP created a Run. The immutable reservation is matched
// without holding any transaction across the network call. Observation writers
// still revalidate the complete original facts under their shared identity lock.
func (repository *Repository) SubmissionObservationTime(ctx context.Context, permit biz.PipelineSendPermit) (time.Time, error) {
	ids, err := observationPermitIDs(permit)
	if err != nil {
		return time.Time{}, err
	}
	if ctx == nil {
		return time.Time{}, biz.ErrInvalidAdmission
	}
	if repository == nil || repository.pool == nil {
		return time.Time{}, biz.ErrPersistence
	}
	row, err := submissionsql.New(repository.pool).GetSubmissionObservationTime(ctx, submissionsql.GetSubmissionObservationTimeParams{TenantID: ids.tenant, ExecutionID: ids.execution})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, biz.ErrExecutionNotFound
	}
	if err != nil {
		return time.Time{}, biz.ErrPersistence
	}
	if row.AttemptID != ids.attempt || row.PlanHash != permit.PlanHash {
		return time.Time{}, biz.ErrAdmissionConflict
	}
	if !row.ReservedAt.Valid || row.ReservedAt.InfinityModifier != pgtype.Finite || !validObservationTime(row.ReservedAt.Time) ||
		!row.ObservedAt.Valid || row.ObservedAt.InfinityModifier != pgtype.Finite || !validObservationTime(row.ObservedAt.Time) || row.ObservedAt.Time.Before(row.ReservedAt.Time) {
		// Database clock reversal is a recording failure. Never substitute the
		// application clock, clamp to reserved_at, or authorize another POST.
		return time.Time{}, biz.ErrPersistence
	}
	return row.ObservedAt.Time.UTC(), nil
}
