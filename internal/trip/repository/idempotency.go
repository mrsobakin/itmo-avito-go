package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

const idempotencyLockSeed = 0

func (r *Repository) LockIdempotencyKey(ctx context.Context, key uuid.UUID) error {
	err := r.exec(ctx, r.sql.Select().
		Column(sq.Expr("pg_advisory_xact_lock(hashtextextended(?, ?))", key.String(), idempotencyLockSeed)))
	if err != nil {
		return fmt.Errorf("lock idempotency key: %w", err)
	}

	return nil
}

func (r *Repository) FindIdempotencyKey(
	ctx context.Context,
	key uuid.UUID,
	createdAfter time.Time,
) (model.IdempotencyRecord, bool, error) {
	var record model.IdempotencyRecord

	err := r.queryRow(ctx, r.sql.Select("request_hash", "trip_id").
		From("idempotency_keys").
		Where(sq.Eq{"key": key}).
		Where(sq.Gt{"created_at": createdAfter}),
		&record.RequestHash, &record.TripID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.IdempotencyRecord{}, false, nil
	}

	if err != nil {
		return model.IdempotencyRecord{}, false, fmt.Errorf("find idempotency key: %w", err)
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
