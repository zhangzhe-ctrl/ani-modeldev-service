package execution

import (
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	executionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution/sqlc"
)

func (r *Repository) ApplyCloseIntent(ctx context.Context, intent biz.CloseIntent) (biz.CloseRecord, error) {
	if err := intent.Validate(); err != nil {
		return biz.CloseRecord{}, err
	}
	tenantID, err := databaseID(intent.TenantID)
	if err != nil {
		return biz.CloseRecord{}, err
	}
	executionID, err := databaseID(intent.ExecutionID)
	if err != nil {
		return biz.CloseRecord{}, err
	}
	operationID, err := databaseID(intent.OperationID)
	if err != nil {
		return biz.CloseRecord{}, err
	}
	transaction, queries, err := r.lockIdentity(ctx, executionsql.InsertExecutionIdentityParams{
		TenantID: tenantID, ExecutionID: executionID, OperationID: operationID, SpecHash: intent.SpecHash,
	})
	if err != nil {
		return biz.CloseRecord{}, err
	}
	defer rollbackExecutionTransaction(transaction)
	sourceGeneration := pgtype.Numeric{Int: new(big.Int).SetUint64(intent.SourceGeneration), Valid: true}
	existing, err := queries.GetCloseIntentBySource(ctx, executionsql.GetCloseIntentBySourceParams{
		TenantID: tenantID, ExecutionID: executionID, SourceGeneration: sourceGeneration,
	})
	if err == nil {
		record, err := closeRecordFromRow(existing)
		if err != nil {
			return biz.CloseRecord{}, err
		}
		if !sameCloseIntent(record.CloseIntent, intent) {
			return biz.CloseRecord{}, biz.ErrAdmissionConflict
		}
		if err := transaction.Commit(ctx); err != nil {
			return biz.CloseRecord{}, biz.ErrPersistence
		}
		return record, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return biz.CloseRecord{}, biz.ErrPersistence
	}
	generation, err := queries.AdvanceCloseGeneration(ctx, executionsql.AdvanceCloseGenerationParams{
		TenantID: tenantID, ExecutionID: executionID,
	})
	if err != nil {
		return biz.CloseRecord{}, biz.ErrPersistence
	}
	row, err := queries.InsertCloseIntent(ctx, executionsql.InsertCloseIntentParams{
		TenantID: tenantID, ExecutionID: executionID, OperationID: operationID, SpecHash: intent.SpecHash,
		SourceGeneration: sourceGeneration,
		OwnerGeneration:  generation,
		RequestedAt:      pgtype.Timestamptz{Time: intent.RequestedAt.UTC(), Valid: true},
		RequestedActor:   intent.RequestedActor,
	})
	if err != nil {
		return biz.CloseRecord{}, biz.ErrPersistence
	}
	record, err := closeRecordFromRow(row)
	if err != nil {
		return biz.CloseRecord{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return biz.CloseRecord{}, biz.ErrPersistence
	}
	return record, nil
}

func sameCloseIntent(stored, requested biz.CloseIntent) bool {
	return strings.EqualFold(stored.TenantID, requested.TenantID) &&
		strings.EqualFold(stored.OperationID, requested.OperationID) &&
		strings.EqualFold(stored.ExecutionID, requested.ExecutionID) &&
		stored.SpecHash == requested.SpecHash &&
		stored.SourceGeneration == requested.SourceGeneration &&
		stored.Reason == requested.Reason &&
		stored.RequestedAt.Equal(requested.RequestedAt) &&
		stored.RequestedActor == requested.RequestedActor
}

func (r *Repository) GetCloseIntent(ctx context.Context, tenant, execution string) (biz.CloseRecord, error) {
	tenantID, err := databaseID(tenant)
	if err != nil {
		return biz.CloseRecord{}, err
	}
	executionID, err := databaseID(execution)
	if err != nil {
		return biz.CloseRecord{}, err
	}
	row, err := executionsql.New(r.pool).GetCloseIntent(ctx, executionsql.GetCloseIntentParams{
		TenantID: tenantID, ExecutionID: executionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.CloseRecord{}, biz.ErrExecutionNotFound
	}
	if err != nil {
		return biz.CloseRecord{}, biz.ErrPersistence
	}
	return closeRecordFromRow(row)
}

func closeRecordFromRow(row executionsql.ModeldevCloseIntent) (biz.CloseRecord, error) {
	if !row.TenantID.Valid || !row.ExecutionID.Valid || !row.OperationID.Valid || !row.RequestedAt.Valid || row.RequestedAt.InfinityModifier != pgtype.Finite || row.SourceKind != "GOVERNANCE" || row.CloseState != string(biz.CloseStateClosing) {
		return biz.CloseRecord{}, biz.ErrPersistence
	}
	sourceGeneration, err := positiveGeneration(row.SourceGeneration)
	if err != nil {
		return biz.CloseRecord{}, err
	}
	generation, err := positiveGeneration(row.OwnerGeneration)
	if err != nil {
		return biz.CloseRecord{}, err
	}
	record := biz.CloseRecord{
		CloseIntent: biz.CloseIntent{
			TenantID: row.TenantID.String(), ExecutionID: row.ExecutionID.String(), OperationID: row.OperationID.String(),
			SpecHash: row.SpecHash, SourceGeneration: sourceGeneration, Reason: biz.CloseReason(row.Reason),
			RequestedAt: row.RequestedAt.Time.UTC(), RequestedActor: row.RequestedActor,
		},
		Generation: generation,
		State:      biz.CloseState(row.CloseState),
	}
	if err := record.CloseIntent.Validate(); err != nil {
		return biz.CloseRecord{}, biz.ErrPersistence
	}
	return record, nil
}

func positiveGeneration(value pgtype.Numeric) (uint64, error) {
	if !value.Valid || value.NaN || value.InfinityModifier != pgtype.Finite || value.Int == nil || value.Exp != 0 || value.Int.Sign() <= 0 || value.Int.BitLen() > 64 {
		return 0, biz.ErrPersistence
	}
	return value.Int.Uint64(), nil
}
