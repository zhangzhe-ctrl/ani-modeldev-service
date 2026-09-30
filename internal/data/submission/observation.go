package submission

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	submissionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission/sqlc"
)

// lockedObservation keeps the original attempt and all observations under the
// same identity lock used by reservation and close. It never creates an identity.
type lockedObservation struct {
	transaction pgx.Tx
	queries     *submissionsql.Queries
	row         submissionsql.ModeldevPipelineDispatch
	admission   submissionsql.ModeldevExecution
	dispatch    biz.PipelineDispatch
}

func (repository *Repository) lockObservation(ctx context.Context, permit biz.PipelineSendPermit, observedAt time.Time) (_ *lockedObservation, err error) {
	ids, err := observationPermitIDs(permit)
	if err != nil {
		return nil, err
	}
	if !validObservationTime(observedAt) {
		return nil, biz.ErrInvalidAdmission
	}
	if repository == nil || repository.pool == nil {
		return nil, biz.ErrPersistence
	}
	transaction, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, biz.ErrPersistence
	}
	defer func() {
		if err != nil {
			rollback(transaction)
		}
	}()
	queries := submissionsql.New(transaction)
	identity, err := queries.LockExecutionIdentity(ctx, submissionsql.LockExecutionIdentityParams{TenantID: ids.tenant, ExecutionID: ids.execution})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.ErrExecutionNotFound
	}
	if err != nil {
		return nil, biz.ErrPersistence
	}
	row, err := queries.GetPipelineDispatch(ctx, submissionsql.GetPipelineDispatchParams{TenantID: ids.tenant, ExecutionID: ids.execution})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.ErrExecutionNotFound
	}
	if err != nil {
		return nil, biz.ErrPersistence
	}
	if row.TenantID != identity.TenantID || row.ExecutionID != identity.ExecutionID || row.OperationID != identity.OperationID || row.SpecHash != identity.SpecHash {
		return nil, biz.ErrPersistence
	}
	if row.AttemptID != ids.attempt || row.PlanHash != permit.PlanHash {
		return nil, biz.ErrAdmissionConflict
	}
	admission, err := queries.GetAdmission(ctx, submissionsql.GetAdmissionParams{TenantID: ids.tenant, ExecutionID: ids.execution})
	if err != nil {
		return nil, biz.ErrPersistence
	}
	dispatch, err := readDispatch(ctx, queries, row, admission)
	if err != nil {
		return nil, err
	}
	if observedAt.Before(dispatch.ReservedAt) {
		return nil, biz.ErrInvalidAdmission
	}
	// CreationOpen and deadline fence new sends, not facts from the already
	// reserved attempt. Late results must remain available to close/reconcile.
	return &lockedObservation{transaction: transaction, queries: queries, row: row, admission: admission, dispatch: dispatch}, nil
}

type observationIDs struct {
	tenant pgtype.UUID
	execution pgtype.UUID
	attempt pgtype.UUID
}

// Clock sampling and observation writes accept the same exact permit shape.
// Parsing never proves the corresponding reservation exists or authorizes a send.
func observationPermitIDs(permit biz.PipelineSendPermit) (observationIDs, error) {
	tenant, tenantErr := databaseID(permit.TenantID)
	execution, executionErr := databaseID(permit.ExecutionID)
	attempt, attemptErr := databaseID(permit.AttemptID)
	if tenantErr != nil || executionErr != nil || attemptErr != nil || len(permit.PlanHash) != 64 || strings.ToLower(permit.PlanHash) != permit.PlanHash {
		return observationIDs{}, biz.ErrInvalidAdmission
	}
	if _, err := hex.DecodeString(permit.PlanHash); err != nil {
		return observationIDs{}, biz.ErrInvalidAdmission
	}
	return observationIDs{tenant: tenant, execution: execution, attempt: attempt}, nil
}

func validObservationTime(value time.Time) bool {
	value = value.UTC()
	return !value.IsZero() && value.Year() >= 1 && value.Year() <= 9999 && value.Nanosecond()%1000 == 0
}
