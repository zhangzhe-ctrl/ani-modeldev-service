// Package input persists fixed managed input imports. It does not authorize
// callers or inspect remote object bytes.
package input

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

type Repository struct { pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) FreezeImport(context.Context, biz.InputImport) (biz.InputVersion, error) {
	return biz.InputVersion{}, biz.ErrPersistence
}

func (r *Repository) Get(context.Context, string, string) (biz.InputVersion, error) {
	return biz.InputVersion{}, biz.ErrPersistence
}
