package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

const (
	keysTable = "idempotency_keys"
	lockSeed  = 0
)

type Repository struct {
	db db
}

func New(db db) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Lock(ctx context.Context, key uuid.UUID) error {
	err := r.db.Exec(ctx, postgres.SQL.Select().
		Column(sq.Expr("pg_advisory_xact_lock(hashtextextended(?, ?))", key.String(), lockSeed)))
	if err != nil {
		return fmt.Errorf("lock idempotency key: %w", err)
	}

	return nil
}

func (r *Repository) Find(ctx context.Context, key uuid.UUID, createdAfter time.Time) (model.IdempotencyRecord, bool, error) {
	var record model.IdempotencyRecord

	err := r.db.QueryRow(ctx, postgres.SQL.Select("request_hash", "trip_id").
		From(keysTable).
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

func (r *Repository) Save(ctx context.Context, key uuid.UUID, record model.IdempotencyRecord, createdAt time.Time) error {
	err := r.db.Exec(ctx, postgres.SQL.Insert(keysTable).
		Columns("key", "request_hash", "trip_id", "created_at").
		Values(key, record.RequestHash, record.TripID, createdAt).
		Suffix(`ON CONFLICT (key) DO UPDATE SET
			request_hash = EXCLUDED.request_hash,
			trip_id = EXCLUDED.trip_id,
			created_at = EXCLUDED.created_at`))
	if err != nil {
		return fmt.Errorf("save idempotency key: %w", err)
	}

	return nil
}
