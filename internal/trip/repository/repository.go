package repository

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/mrsobakin/itmo-avito-go/internal/transaction"
	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

const tripColumns = `id, user_id, driver_id,
    start_latitude, start_longitude, end_latitude, end_longitude,
    price, status, started_at, finished_at`

type Repository struct {
	pool    transaction.DBTX
	timeout time.Duration
	sql     sq.StatementBuilderType
}

func New(pool transaction.DBTX, timeout time.Duration) *Repository {
	return &Repository{
		pool:    pool,
		timeout: timeout,
		sql:     sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

func (r *Repository) exec(ctx context.Context, builder sq.Sqlizer) error {
	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	_, err = transaction.Executor(ctx, r.pool).Exec(ctx, query, args...)

	return err
}

func (r *Repository) queryRow(ctx context.Context, builder sq.Sqlizer, destination ...any) error {
	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return transaction.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(destination...)
}

func (r *Repository) queryTrip(ctx context.Context, builder sq.Sqlizer) (model.Trip, error) {
	var value model.Trip

	err := r.queryRow(ctx, builder,
		&value.ID,
		&value.UserID,
		&value.DriverID,
		&value.StartPoint.Latitude,
		&value.StartPoint.Longitude,
		&value.EndPoint.Latitude,
		&value.EndPoint.Longitude,
		&value.Price,
		&value.Status,
		&value.StartedAt,
		&value.FinishedAt,
	)
	if err != nil {
		return model.Trip{}, err
	}

	value.StartedAt = value.StartedAt.UTC()
	if value.FinishedAt != nil {
		finishedAt := value.FinishedAt.UTC()
		value.FinishedAt = &finishedAt
	}

	return value, nil
}
