package trip

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

type db interface {
	Exec(ctx context.Context, builder sq.Sqlizer) error
	QueryRow(ctx context.Context, builder sq.Sqlizer, destination ...any) error
	Query(ctx context.Context, builder sq.Sqlizer, scan func(row pgx.Row) error) error
}
