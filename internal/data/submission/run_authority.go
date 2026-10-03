package submission

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	submissionsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission/sqlc"
)

// BindRunAuthority commits the first immutable association under the same
// identity lock as Admission, submission and close. The upstream adapter owns
// actual workload identity verification; these strings cannot perform it.
// A successful binding is not a permit to create training resources.
func (repository *Repository) BindRunAuthority(ctx context.Context, candidate biz.RunAuthorityCandidate) (biz.RunAuthorityReceipt, error) {
	candidate, err := canonicalRunAuthorityCandidate(candidate)
	if err != nil || ctx == nil {
		return biz.RunAuthorityReceipt{}, biz.ErrInvalidAdmission
	}
	if repository == nil || repository.pool == nil {
		return biz.RunAuthorityReceipt{}, biz.ErrPersistence
	}
	tenantID, _ := databaseID(candidate.TenantID)
	executionID, _ := databaseID(candidate.ExecutionID)
	transaction, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return biz.RunAuthorityReceipt{}, biz.ErrPersistence
	}
	defer rollback(transaction)
	queries := submissionsql.New(transaction)
	identity, err := queries.LockExecutionIdentity(ctx, submissionsql.LockExecutionIdentityParams{TenantID: tenantID, ExecutionID: executionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.RunAuthorityReceipt{}, biz.ErrExecutionNotFound
	}
	if err != nil {
		return biz.RunAuthorityReceipt{}, biz.ErrPersistence
	}
	if identity.OperationID.String() != candidate.OperationID || identity.SpecHash != candidate.SpecHash {
		return biz.RunAuthorityReceipt{}, biz.ErrAdmissionConflict
	}
	dispatch, err := readInTransaction(ctx, transaction, tenantID, executionID)
	if err != nil {
		return biz.RunAuthorityReceipt{}, err
	}
	if !runAuthorityMatchesDispatch(candidate, dispatch) {
		return biz.RunAuthorityReceipt{}, biz.ErrAdmissionConflict
	}
	row, err := queries.GetRunAuthorityRow(ctx, submissionsql.GetRunAuthorityRowParams{TenantID: tenantID, ExecutionID: executionID})
	if err == nil {
		authority, err := runAuthorityFromRow(row, dispatch)
		if err != nil {
			return biz.RunAuthorityReceipt{}, err
		}
		if authority.RunAuthorityCandidate != candidate {
			return biz.RunAuthorityReceipt{}, biz.ErrRunAuthorityConflict
		}
		// An exact replay preserves the first binding after close/deadline, but
		// returns no new creation permission and does not advance the revision.
		if err := transaction.Commit(ctx); err != nil {
			return biz.RunAuthorityReceipt{}, biz.ErrPersistence
		}
		return biz.RunAuthorityReceipt{RunAuthority: authority, Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return biz.RunAuthorityReceipt{}, biz.ErrPersistence
	}
	if !identity.CreationOpen {
		return biz.RunAuthorityReceipt{}, biz.ErrPipelineDispatchBlocked
	}
	operationID, _ := databaseID(candidate.OperationID)
	attemptID, _ := databaseID(candidate.AttemptID)
	runID, _ := databaseID(candidate.RunID)
	namespaceUID, _ := databaseID(candidate.NamespaceUID)
	inserted, err := queries.InsertRunAuthority(ctx, submissionsql.InsertRunAuthorityParams{
		TenantID: tenantID, ExecutionID: executionID, OperationID: operationID, SpecHash: candidate.SpecHash,
		AttemptID: attemptID, PlanHash: candidate.PlanHash, RunID: runID,
		NamespaceName: candidate.NamespaceName, NamespaceUid: namespaceUID,
		WorkflowName: candidate.WorkflowName, WorkflowUid: candidate.WorkflowUID,
		DeadlineAt: pgtype.Timestamptz{Time: dispatch.Plan.DeadlineAt.UTC(), Valid: true},
	})
	if err != nil || inserted < 0 || inserted > 1 {
		return biz.RunAuthorityReceipt{}, biz.ErrPersistence
	}
	if inserted == 0 {
		return biz.RunAuthorityReceipt{}, biz.ErrPipelineDispatchBlocked
	}
	dispatch.OwnerRevision, err = advanceOwnerRevision(ctx, queries, tenantID, executionID)
	if err != nil {
		return biz.RunAuthorityReceipt{}, err
	}
	row, err = queries.GetRunAuthorityRow(ctx, submissionsql.GetRunAuthorityRowParams{TenantID: tenantID, ExecutionID: executionID})
	if err != nil {
		return biz.RunAuthorityReceipt{}, biz.ErrPersistence
	}
	authority, err := runAuthorityFromRow(row, dispatch)
	if err != nil || authority.RunAuthorityCandidate != candidate {
		return biz.RunAuthorityReceipt{}, biz.ErrPersistence
	}
	if err := transaction.Commit(ctx); err != nil {
		return biz.RunAuthorityReceipt{}, biz.ErrPersistence
	}
	return biz.RunAuthorityReceipt{RunAuthority: authority}, nil
}

func (repository *Repository) GetRunAuthority(ctx context.Context, tenant, execution string) (biz.RunAuthority, error) {
	tenantID, tenantErr := databaseID(tenant)
	executionID, executionErr := databaseID(execution)
	if tenantErr != nil || executionErr != nil || ctx == nil {
		return biz.RunAuthority{}, biz.ErrInvalidAdmission
	}
	if repository == nil || repository.pool == nil {
		return biz.RunAuthority{}, biz.ErrPersistence
	}
	transaction, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return biz.RunAuthority{}, biz.ErrPersistence
	}
	defer rollback(transaction)
	queries := submissionsql.New(transaction)
	row, err := queries.GetRunAuthorityRow(ctx, submissionsql.GetRunAuthorityRowParams{TenantID: tenantID, ExecutionID: executionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.RunAuthority{}, biz.ErrExecutionNotFound
	}
	if err != nil {
		return biz.RunAuthority{}, biz.ErrPersistence
	}
	dispatch, err := readInTransaction(ctx, transaction, tenantID, executionID)
	if err != nil {
		return biz.RunAuthority{}, biz.ErrPersistence
	}
	authority, err := runAuthorityFromRow(row, dispatch)
	if err != nil {
		return biz.RunAuthority{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return biz.RunAuthority{}, biz.ErrPersistence
	}
	return authority, nil
}

func runAuthorityFromRow(row submissionsql.GetRunAuthorityRowRow, dispatch biz.PipelineDispatch) (biz.RunAuthority, error) {
	candidate := biz.RunAuthorityCandidate{
		TenantID: row.TenantID.String(), ExecutionID: row.ExecutionID.String(), OperationID: row.OperationID.String(),
		SpecHash: row.SpecHash, AttemptID: row.AttemptID.String(), PlanHash: row.PlanHash, RunID: row.RunID.String(),
		NamespaceName: row.NamespaceName, NamespaceUID: row.NamespaceUid.String(),
		WorkflowName: row.WorkflowName, WorkflowUID: row.WorkflowUid,
	}
	canonical, err := canonicalRunAuthorityCandidate(candidate)
	if err != nil || canonical != candidate || !runAuthorityMatchesDispatch(candidate, dispatch) ||
		!row.BoundAt.Valid || row.BoundAt.InfinityModifier != pgtype.Finite || !validObservationTime(row.BoundAt.Time) ||
		row.BoundAt.Time.Before(dispatch.ReservedAt) || !row.BoundAt.Time.Before(dispatch.Plan.DeadlineAt) {
		return biz.RunAuthority{}, biz.ErrPersistence
	}
	revision, err := positiveOwnerRevision(row.OwnerRevision)
	if err != nil || revision != dispatch.OwnerRevision {
		return biz.RunAuthority{}, biz.ErrPersistence
	}
	return biz.RunAuthority{RunAuthorityCandidate: candidate, BoundAt: row.BoundAt.Time.UTC(), OwnerRevision: revision}, nil
}

func runAuthorityMatchesDispatch(candidate biz.RunAuthorityCandidate, dispatch biz.PipelineDispatch) bool {
	return candidate.TenantID == dispatch.Plan.TenantID && candidate.ExecutionID == dispatch.Plan.ExecutionID &&
		candidate.OperationID == dispatch.Plan.OperationID && candidate.SpecHash == dispatch.Plan.SpecHash &&
		candidate.AttemptID == dispatch.AttemptID && candidate.PlanHash == dispatch.PlanHash &&
		candidate.NamespaceName == dispatch.Plan.Environment.NamespaceName && candidate.NamespaceUID == dispatch.Plan.Environment.NamespaceUID
}

func canonicalRunAuthorityCandidate(candidate biz.RunAuthorityCandidate) (biz.RunAuthorityCandidate, error) {
	for _, field := range []*string{&candidate.TenantID, &candidate.ExecutionID, &candidate.OperationID, &candidate.AttemptID, &candidate.RunID, &candidate.NamespaceUID} {
		id, err := databaseID(*field)
		if err != nil {
			return biz.RunAuthorityCandidate{}, biz.ErrInvalidAdmission
		}
		*field = id.String()
	}
	for _, hash := range []string{candidate.SpecHash, candidate.PlanHash} {
		if len(hash) != 64 || strings.ToLower(hash) != hash {
			return biz.RunAuthorityCandidate{}, biz.ErrInvalidAdmission
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return biz.RunAuthorityCandidate{}, biz.ErrInvalidAdmission
		}
	}
	if len(validation.IsDNS1123Label(candidate.NamespaceName)) != 0 || len(validation.IsDNS1123Subdomain(candidate.WorkflowName)) != 0 ||
		len(candidate.WorkflowUID) == 0 || len(candidate.WorkflowUID) > 128 {
		return biz.RunAuthorityCandidate{}, biz.ErrInvalidAdmission
	}
	for _, character := range candidate.WorkflowUID {
		if character < '!' || character > '~' {
			return biz.RunAuthorityCandidate{}, biz.ErrInvalidAdmission
		}
	}
	return candidate, nil
}
