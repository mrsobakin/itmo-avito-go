package idempotency

import (
	"context"

	sq "github.com/Masterminds/squirrel"
)

type db interface {
	Exec(ctx context.Context, builder sq.Sqlizer) error
	QueryRow(ctx context.Context, builder sq.Sqlizer, destination ...any) error
}
