package position

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
)

const positionsTable = "trip_positions"

type Repository struct {
	db db
}

func New(db db) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Save(ctx context.Context, value trip.Position) (trip.Position, error) {
	err := r.db.QueryRow(ctx, postgres.SQL.Insert(positionsTable).
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(value.TripID, value.Coordinates.Latitude, value.Coordinates.Longitude, value.RecordedAt).
		Suffix("RETURNING id"), &value.ID)
	if err != nil {
		return trip.Position{}, fmt.Errorf("save position: %w", err)
	}

	return value, nil
}

func (r *Repository) List(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error) {
	query := postgres.SQL.Select("id", "trip_id", "latitude", "longitude", "recorded_at").
		From(positionsTable).
		Where(sq.Eq{"trip_id": tripID}).
		OrderBy("recorded_at", "id")

	positions := []trip.Position{}

	err := r.db.Query(ctx, query, func(row pgx.Row) error {
		var value trip.Position
		if err := row.Scan(
			&value.ID, &value.TripID, &value.Coordinates.Latitude, &value.Coordinates.Longitude, &value.RecordedAt,
		); err != nil {
			return err
		}

		value.RecordedAt = value.RecordedAt.UTC()
		positions = append(positions, value)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}

	return positions, nil
}
