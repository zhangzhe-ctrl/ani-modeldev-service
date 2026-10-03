package lifecycle

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	lifecyclesql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle/sqlc"
)

var _ biz.PendingCloseRepository = (*Repository)(nil)

func (repository *Repository) ListPendingClosures(ctx context.Context, binding biz.PipelineDispatchBinding, after string, limit int) ([]string, error) {
	if ctx == nil || binding.Validate() != nil || limit < 1 || limit > 100 {
		return nil, biz.ErrInvalidDispatchWorker
	}
	if repository == nil || repository.pool == nil {
		return nil, biz.ErrPersistence
	}
	tenant, err := databaseID(binding.TenantID)
	if err != nil {
		return nil, err
	}
	cursor := pgtype.UUID{}
	if after != "" {
		cursor, err = databaseID(after)
		if err != nil || cursor.String() != after {
			return nil, biz.ErrInvalidAdmission
		}
	}
	environment, err := json.Marshal(binding.Environment)
	if err != nil {
		return nil, biz.ErrInvalidDispatchWorker
	}
	rows, err := lifecyclesql.New(repository.pool).ListPendingBoundClosures(ctx, lifecyclesql.ListPendingBoundClosuresParams{TenantID: tenant, Environment: environment, AfterExecutionID: cursor, BatchSize: int32(limit)})
	if err != nil {
		return nil, biz.ErrPersistence
	}
	ids := make([]string, len(rows))
	for i, id := range rows {
		if !id.Valid {
			return nil, biz.ErrPersistence
		}
		ids[i] = id.String()
	}
	return ids, nil
}
