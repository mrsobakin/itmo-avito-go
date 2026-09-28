package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrsobakin/itmo-avito-go/internal/transaction"
)

const tripColumns = `id, user_id, driver_id,
    start_latitude, start_longitude, end_latitude, end_longitude,
    price, status, started_at, finished_at`

type Repository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
	sql     sq.StatementBuilderType
}

func NewRepository(pool *pgxpool.Pool, timeout time.Duration) *Repository {
	return &Repository{
		pool:    pool,
		timeout: timeout,
		sql:     sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

func (r *Repository) Create(ctx context.Context, value Trip) error {
	err := r.exec(ctx, r.sql.Insert("trips").
		Columns(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude", "end_latitude", "end_longitude",
			"price", "status", "started_at", "finished_at",
		).
		Values(
			value.ID, value.UserID, value.DriverID,
			value.StartPoint.Latitude, value.StartPoint.Longitude,
			value.EndPoint.Latitude, value.EndPoint.Longitude,
			value.Price, value.Status, value.StartedAt, value.FinishedAt,
		))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "trips_one_active_per_driver_idx" {
		return ErrDriverBusy
	}
	if err != nil {
		return fmt.Errorf("create trip: %w", err)
	}
	return nil
}

func (r *Repository) AddStatusHistory(
	ctx context.Context,
	tripID uuid.UUID,
	from *Status,
	to Status,
	reason string,
) error {
	err := r.exec(ctx, r.sql.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, from, to, reason))
	if err != nil {
		return fmt.Errorf("add trip status history: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Trip, error) {
	value, err := r.queryTrip(ctx, r.sql.Select(tripColumns).
		From("trips").
		Where(sq.Eq{"id": id}))
	if errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, ErrNotFound
	}
	if err != nil {
		return Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return value, nil
}

func (r *Repository) Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (Trip, error) {
	value, err := r.queryTrip(ctx, r.sql.Update("trips").
		Set("status", StatusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(sq.Eq{"id": id, "status": StatusActive}).
		Suffix("RETURNING "+tripColumns))
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	existing, err := r.Get(ctx, id)
	if err != nil {
		return Trip{}, err
	}
	if existing.Status == StatusCompleted {
		return Trip{}, ErrCompleted
	}
	return Trip{}, fmt.Errorf("finish trip: unexpected status %q", existing.Status)
}

func (r *Repository) LockIdempotencyKey(ctx context.Context, key uuid.UUID) error {
	err := r.exec(ctx, r.sql.Select().
		Column(sq.Expr("pg_advisory_xact_lock(hashtextextended(?, 0))", key.String())))
	if err != nil {
		return fmt.Errorf("lock idempotency key: %w", err)
	}
	return nil
}

func (r *Repository) FindIdempotencyKey(
	ctx context.Context,
	key uuid.UUID,
	createdAfter time.Time,
) (IdempotencyRecord, bool, error) {
	var record IdempotencyRecord
	err := r.queryRow(ctx, r.sql.Select("request_hash", "trip_id").
		From("idempotency_keys").
		Where(sq.Eq{"key": key}).
		Where(sq.Gt{"created_at": createdAfter}),
		&record.RequestHash, &record.TripID)
	if errors.Is(err, pgx.ErrNoRows) {
		return IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("find idempotency key: %w", err)
	}
	return record, true, nil
}

func (r *Repository) SaveIdempotencyKey(
	ctx context.Context,
	key uuid.UUID,
	requestHash string,
	tripID uuid.UUID,
	createdAt time.Time,
) error {
	err := r.exec(ctx, r.sql.Insert("idempotency_keys").
		Columns("key", "request_hash", "trip_id", "created_at").
		Values(key, requestHash, tripID, createdAt).
		Suffix(`ON CONFLICT (key) DO UPDATE SET
			request_hash = EXCLUDED.request_hash,
			trip_id = EXCLUDED.trip_id,
			created_at = EXCLUDED.created_at`))
	if err != nil {
		return fmt.Errorf("save idempotency key: %w", err)
	}
	return nil
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

func (r *Repository) queryTrip(ctx context.Context, builder sq.Sqlizer) (Trip, error) {
	var value Trip
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
		return Trip{}, err
	}

	value.StartedAt = value.StartedAt.UTC()
	if value.FinishedAt != nil {
		finishedAt := value.FinishedAt.UTC()
		value.FinishedAt = &finishedAt
	}
	return value, nil
}
