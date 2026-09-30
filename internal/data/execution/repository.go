// Package execution implements the ModelDev execution persistence boundary.
package execution

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

type Repository struct {
	pool *pgxpool.Pool
}

var _ biz.ExecutionRepository = (*Repository)(nil)

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Accept(context.Context, biz.Admission) (biz.Execution, error) {
	return biz.Execution{}, biz.ErrNotImplemented
}

func (r *Repository) Get(context.Context, string, string) (biz.Execution, error) {
	return biz.Execution{}, biz.ErrNotImplemented
}
