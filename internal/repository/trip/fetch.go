package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
)

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	return r.fetch(ctx, postgres.SQL.Select(tripColumns...).From(tripsTable).Where(sq.Eq{"id": id}))
}

func (r *Repository) GetForUpdate(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	return r.fetch(ctx, postgres.SQL.Select(tripColumns...).From(tripsTable).Where(sq.Eq{"id": id}).Suffix("FOR UPDATE"))
}

func (r *Repository) fetch(ctx context.Context, query sq.SelectBuilder) (trip.Trip, error) {
	var value trip.Trip

	err := r.db.QueryRow(ctx, query, tripDestinations(&value)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, trip.ErrTripNotFound
	}

	if err != nil {
		return trip.Trip{}, fmt.Errorf("fetch trip: %w", err)
	}

	return normalize(value), nil
}
