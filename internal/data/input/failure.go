package input

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	inputsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input/sqlc"
)

// RecordValidationFailure records a trusted verifier's finite observation.
// Source unavailability remains retryable against the same fixed object;
// rejected content is terminal and must never be promoted by a late verifier.
func (r *Repository) RecordValidationFailure(ctx context.Context, expected biz.InputImport, failure biz.InputValidationFailure) (biz.InputVersion, error) {
	command, err := importCommand(expected)
	if err != nil {
		return biz.InputVersion{}, err
	}
	if err := failure.ValidateFor(expected); err != nil {
		return biz.InputVersion{}, err
	}
	if r == nil || r.pool == nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	defer tx.Rollback(ctx)
	queries := inputsql.New(tx)
	row, err := queries.LockInputVersion(ctx, inputsql.LockInputVersionParams{TenantID: command.TenantID, InputVersionID: command.InputVersionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.InputVersion{}, biz.ErrInputNotFound
	}
	if err != nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	if !sameImport(row, command) {
		return biz.InputVersion{}, biz.ErrInputConflict
	}
	version, err := versionFromRow(row)
	if err != nil {
		return biz.InputVersion{}, err
	}
	failure.ObservedAt = failure.ObservedAt.UTC().Truncate(time.Microsecond)
	// Terminal facts win over late observations. For retryable failures retain
	// the newest observed outage; an older error cannot erase newer evidence.
	if version.State == biz.InputStateValidating && (version.Failure == nil || failure.Code == biz.InputFailureContentRejected || failure.ObservedAt.After(version.Failure.ObservedAt)) {
		state := biz.InputStateValidating
		if failure.Code == biz.InputFailureContentRejected {
			state = biz.InputStateRejected
		}
		row, err = queries.RecordValidationFailure(ctx, inputsql.RecordValidationFailureParams{
			TenantID: command.TenantID, InputVersionID: command.InputVersionID, RequestID: command.RequestID,
			State: string(state), FailureCode: pgtype.Text{String: string(failure.Code), Valid: true},
			FailureObservedAt: pgtype.Timestamptz{Time: failure.ObservedAt, Valid: true},
		})
		if err != nil {
			return biz.InputVersion{}, biz.ErrPersistence
		}
		version, err = versionFromRow(row)
		if err != nil {
			return biz.InputVersion{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	return version, nil
}
