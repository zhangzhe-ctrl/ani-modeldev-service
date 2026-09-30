// Package execution implements the ModelDev execution persistence boundary.
package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	executionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution/sqlc"
)

type Repository struct {
	pool *pgxpool.Pool
}

var _ biz.ExecutionRepository = (*Repository)(nil)

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Accept(ctx context.Context, admission biz.Admission) (biz.Execution, error) {
	intent, snapshot, err := admission.CanonicalPayloads()
	if err != nil {
		return biz.Execution{}, err
	}
	tenantID, err := databaseID(admission.TenantID)
	if err != nil {
		return biz.Execution{}, err
	}
	executionID, err := databaseID(admission.ExecutionID)
	if err != nil {
		return biz.Execution{}, err
	}
	operationID, err := databaseID(admission.OperationID)
	if err != nil {
		return biz.Execution{}, err
	}
	command := executionsql.InsertExecutionParams{
		TenantID:          tenantID,
		ExecutionID:       executionID,
		OperationID:       operationID,
		Actor:             admission.Actor,
		IntentCanonical:   intent,
		IntentHash:        admission.IntentHash,
		SnapshotCanonical: snapshot,
		SpecHash:          admission.SpecHash,
		AcceptedAt:        pgtype.Timestamptz{Time: admission.AcceptedAt.UTC(), Valid: true},
	}
	queries := executionsql.New(r.pool)
	// This single statement is committed by PostgreSQL before its successful
	// result is acknowledged; no in-memory receipt can substitute for the row.
	row, err := queries.InsertExecution(ctx, command)
	if err != nil {
		var databaseError *pgconn.PgError
		if !errors.As(err, &databaseError) || databaseError.Code != "23505" {
			return biz.Execution{}, biz.ErrPersistence
		}
		// PostgreSQL resolves a competing unique-key insert before reporting
		// this violation. A fresh statement can now read the committed winner,
		// still scoped to the caller's tenant and original execution identity.
		row, err = queries.GetExecution(ctx, executionsql.GetExecutionParams{
			TenantID: tenantID, ExecutionID: executionID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// The conflicting identity belongs to another execution or tenant.
			// Do not query outside the trusted scope or disclose that record.
			return biz.Execution{}, biz.ErrAdmissionConflict
		}
		if err != nil {
			return biz.Execution{}, biz.ErrPersistence
		}
		if !sameAdmission(row, command) {
			return biz.Execution{}, biz.ErrAdmissionConflict
		}
	}
	return executionFromRow(row)
}

func sameAdmission(row executionsql.ModeldevExecution, command executionsql.InsertExecutionParams) bool {
	return row.TenantID == command.TenantID &&
		row.ExecutionID == command.ExecutionID &&
		row.OperationID == command.OperationID &&
		row.Actor == command.Actor &&
		row.IntentHash == command.IntentHash &&
		row.SpecHash == command.SpecHash &&
		bytes.Equal(row.IntentCanonical, command.IntentCanonical) &&
		bytes.Equal(row.SnapshotCanonical, command.SnapshotCanonical) &&
		row.AcceptedAt.Valid && row.AcceptedAt.InfinityModifier == pgtype.Finite &&
		row.AcceptedAt.Time.Equal(command.AcceptedAt.Time)
}

func (r *Repository) Get(ctx context.Context, tenant, execution string) (biz.Execution, error) {
	tenantID, err := databaseID(tenant)
	if err != nil {
		return biz.Execution{}, err
	}
	executionID, err := databaseID(execution)
	if err != nil {
		return biz.Execution{}, err
	}
	row, err := executionsql.New(r.pool).GetExecution(ctx, executionsql.GetExecutionParams{
		TenantID: tenantID, ExecutionID: executionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.Execution{}, biz.ErrExecutionNotFound
	}
	if err != nil {
		return biz.Execution{}, biz.ErrPersistence
	}
	return executionFromRow(row)
}

func databaseID(value string) (pgtype.UUID, error) {
	// pgtype.UUID.Scan accepts arbitrary characters at the separator positions.
	// Enforce the public standard UUID shape before the driver decodes its hex.
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return pgtype.UUID{}, biz.ErrInvalidAdmission
	}
	var id pgtype.UUID
	if id.Scan(value) != nil || !id.Valid || id.Bytes == [16]byte{} {
		return pgtype.UUID{}, biz.ErrInvalidAdmission
	}
	return id, nil
}

func executionFromRow(row executionsql.ModeldevExecution) (biz.Execution, error) {
	if !row.TenantID.Valid || !row.ExecutionID.Valid || !row.OperationID.Valid || !row.AcceptedAt.Valid || row.AcceptedAt.InfinityModifier != pgtype.Finite {
		return biz.Execution{}, biz.ErrPersistence
	}
	execution := biz.Execution{Admission: biz.Admission{
		TenantID: row.TenantID.String(), Actor: row.Actor,
		OperationID: row.OperationID.String(), ExecutionID: row.ExecutionID.String(),
		IntentHash: row.IntentHash, SpecHash: row.SpecHash,
		AcceptedAt: row.AcceptedAt.Time.UTC(),
	}}
	// CanonicalIntent's schema field is checked by canonical byte comparison
	// below. Unknown or noncanonical stored fields cannot silently disappear.
	if json.Unmarshal(row.IntentCanonical, &execution.Intent) != nil || json.Unmarshal(row.SnapshotCanonical, &execution.Snapshot) != nil {
		return biz.Execution{}, biz.ErrPersistence
	}
	intent, snapshot, err := execution.Admission.CanonicalPayloads()
	if err != nil || !bytes.Equal(intent, row.IntentCanonical) || !bytes.Equal(snapshot, row.SnapshotCanonical) {
		return biz.Execution{}, biz.ErrPersistence
	}
	return execution, nil
}
