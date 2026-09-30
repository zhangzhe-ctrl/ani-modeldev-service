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

// RecordVerifiedCSV is an internal persistence boundary for a trusted verifier
// observation. It must match the entire frozen import before publishing READY.
// No request transport exposes a state or verification-proof override.
func (r *Repository) RecordVerifiedCSV(ctx context.Context, expected biz.InputImport, verified biz.VerifiedCSV) (biz.InputVersion, error) {
	command, err := importCommand(expected)
	if err != nil {
		return biz.InputVersion{}, err
	}
	if err := verified.ValidateFor(expected); err != nil {
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
	if version.State == biz.InputStateRejected {
		return version, biz.ErrInputVerification
	}
	if version.State == biz.InputStateValidating {
		row, err = queries.RecordVerifiedCSV(ctx, inputsql.RecordVerifiedCSVParams{
			TenantID: command.TenantID, InputVersionID: command.InputVersionID, RequestID: command.RequestID,
			VerifiedAt:            pgtype.Timestamptz{Time: verified.VerifiedAt.UTC().Truncate(time.Microsecond), Valid: true},
			VerifiedSchemaVersion: pgtype.Text{String: verified.SchemaVersion, Valid: true},
			VerifiedRowCount:      pgtype.Int4{Int32: int32(verified.RowCount), Valid: true},
			VerifiedFeatureCount:  pgtype.Int4{Int32: int32(verified.FeatureCount), Valid: true},
		})
		if err != nil {
			return biz.InputVersion{}, biz.ErrPersistence
		}
		version, err = versionFromRow(row)
		if err != nil {
			return biz.InputVersion{}, err
		}
	}
	// Equivalent concurrent/restarted verification returns the original proof
	// without changing its timestamp or downgrading an already READY version.
	if err := tx.Commit(ctx); err != nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	return version, nil
}
