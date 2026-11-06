package postgres

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres/transaction"
)

var SQL = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type DB struct {
	pool    transaction.DBTX
	timeout time.Duration
}

func NewDB(pool transaction.DBTX, timeout time.Duration) *DB {
	return &DB{pool: pool, timeout: timeout}
}

func (db *DB) Exec(ctx context.Context, builder sq.Sqlizer) error {
	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, db.timeout)
	defer cancel()

	_, err = transaction.Executor(ctx, db.pool).Exec(ctx, query, args...)

	return err
}

func (db *DB) QueryRow(ctx context.Context, builder sq.Sqlizer, destination ...any) error {
	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, db.timeout)
	defer cancel()

	return transaction.Executor(ctx, db.pool).QueryRow(ctx, query, args...).Scan(destination...)
}
