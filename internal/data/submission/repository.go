// Package submission owns the execution's durable pipeline submission facts.
package submission

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Reserve deliberately fails until the real PostgreSQL reservation behavior
// has a recorded RED. This candidate is not wired to a product entry point.
func (repository *Repository) Reserve(context.Context, biz.PipelineDispatchRequest) (biz.PipelineDispatchReservation, error) {
	return biz.PipelineDispatchReservation{}, biz.ErrPersistence
}

func (repository *Repository) Get(context.Context, string, string) (biz.PipelineDispatch, error) {
	return biz.PipelineDispatch{}, biz.ErrPersistence
}

var _ biz.PipelineDispatchRepository = (*Repository)(nil)
