// Package input persists fixed managed input imports. It does not authorize
// callers or inspect remote object bytes.
package input

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	inputsql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input/sqlc"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) FreezeImport(ctx context.Context, request biz.InputImport) (biz.InputVersion, error) {
	if err := request.Validate(); err != nil {
		return biz.InputVersion{}, err
	}
	if r == nil || r.pool == nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	tenant, err := databaseID(request.TenantID)
	if err != nil {
		return biz.InputVersion{}, err
	}
	inputID, err := databaseID(request.InputVersionID)
	if err != nil {
		return biz.InputVersion{}, err
	}
	requestID, err := databaseID(request.RequestID)
	if err != nil {
		return biz.InputVersion{}, err
	}
	// One immutable row is the complete import command. The autocommit INSERT
	// must finish before this method can acknowledge the VALIDATING receipt.
	command := inputsql.InsertFrozenImportParams{
		TenantID: tenant, InputVersionID: inputID, RequestID: requestID,
		Actor: request.Actor, RequestedAt: pgtype.Timestamptz{Time: request.RequestedAt.UTC(), Valid: true},
		StorageConnectionID: request.Scope.StorageConnectionID, Bucket: request.Scope.Bucket, ApprovedPrefix: request.Scope.ApprovedPrefix,
		CredentialReference: request.Scope.CredentialReference,
		ObjectKey: request.Object.Key, ObjectVersionID: *request.Object.VersionID, SizeBytes: request.Object.SizeBytes, Sha256: request.Object.SHA256,
	}
	queries := inputsql.New(r.pool)
	row, err := queries.InsertFrozenImport(ctx, command)
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT waits for a competing insertion to finish. Immutable
		// request fields can then be compared through this explicit tenant read.
		row, err = queries.GetInputVersion(ctx, inputsql.GetInputVersionParams{TenantID:tenant,InputVersionID:inputID})
		if errors.Is(err,pgx.ErrNoRows) { return biz.InputVersion{},biz.ErrInputConflict }
		if err != nil { return biz.InputVersion{},biz.ErrPersistence }
		if !sameImport(row,command) { return biz.InputVersion{},biz.ErrInputConflict }
	}
	if err != nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	return versionFromRow(row)
}

func sameImport(row inputsql.ModeldevInputVersion, command inputsql.InsertFrozenImportParams) bool {
	return row.TenantID==command.TenantID && row.InputVersionID==command.InputVersionID && row.RequestID==command.RequestID &&
		row.Actor==command.Actor && row.RequestedAt.Valid && row.RequestedAt.InfinityModifier==pgtype.Finite && row.RequestedAt.Time.Equal(command.RequestedAt.Time) &&
		row.StorageConnectionID==command.StorageConnectionID && row.Bucket==command.Bucket && row.ApprovedPrefix==command.ApprovedPrefix && row.CredentialReference==command.CredentialReference &&
		row.ObjectKey==command.ObjectKey && row.ObjectVersionID==command.ObjectVersionID && row.SizeBytes==command.SizeBytes && row.Sha256==command.Sha256
}

func (r *Repository) Get(ctx context.Context, tenantID, inputVersionID string) (biz.InputVersion, error) {
	tenant, err := databaseID(tenantID)
	if err != nil {
		return biz.InputVersion{}, err
	}
	inputID, err := databaseID(inputVersionID)
	if err != nil {
		return biz.InputVersion{}, err
	}
	if r == nil || r.pool == nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	row, err := inputsql.New(r.pool).GetInputVersion(ctx, inputsql.GetInputVersionParams{TenantID: tenant, InputVersionID: inputID})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.InputVersion{}, biz.ErrInputNotFound
	}
	if err != nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	return versionFromRow(row)
}

func databaseID(value string) (pgtype.UUID, error) {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return pgtype.UUID{}, biz.ErrInvalidInput
	}
	var id pgtype.UUID
	if id.Scan(value) != nil || !id.Valid || id.Bytes == [16]byte{} {
		return pgtype.UUID{}, biz.ErrInvalidInput
	}
	return id, nil
}

func versionFromRow(row inputsql.ModeldevInputVersion) (biz.InputVersion, error) {
	if !row.TenantID.Valid || !row.InputVersionID.Valid || !row.RequestID.Valid || !row.RequestedAt.Valid || row.RequestedAt.InfinityModifier != pgtype.Finite || row.State != string(biz.InputStateValidating) {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	request := biz.InputImport{
		TenantID: row.TenantID.String(), RequestID: row.RequestID.String(), InputVersionID: row.InputVersionID.String(), Actor: row.Actor, RequestedAt: row.RequestedAt.Time.UTC(),
		Scope:  cpup01.StorageScope{StorageConnectionID: row.StorageConnectionID, Bucket: row.Bucket, ApprovedPrefix: row.ApprovedPrefix,CredentialReference:row.CredentialReference},
		Object: cpup01.FixedObjectRef{StorageConnectionID: row.StorageConnectionID, Bucket: row.Bucket, Key: row.ObjectKey, VersionID: &row.ObjectVersionID, SizeBytes: row.SizeBytes, SHA256: row.Sha256},
	}
	if request.Validate() != nil {
		return biz.InputVersion{}, biz.ErrPersistence
	}
	return biz.InputVersion{Import: request, State: biz.InputStateValidating}, nil
}
