package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

const (
	pgUniqueViolation = "23505"

	oneActiveTripPerDriverIndex = "trips_one_active_per_driver_idx"
)

func (r *Repository) Create(ctx context.Context, value model.Trip) error {
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
		pgErr.Code == pgUniqueViolation &&
		pgErr.ConstraintName == oneActiveTripPerDriverIndex {
		return model.ErrDriverBusy
	}

	if err != nil {
		return fmt.Errorf("create trip: %w", err)
	}

	return nil
}

func (r *Repository) AddStatusHistory(
	ctx context.Context,
	tripID uuid.UUID,
	from *model.Status,
	to model.Status,
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

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	value, err := r.queryTrip(ctx, r.sql.Select(tripColumns).
		From("trips").
		Where(sq.Eq{"id": id}))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Trip{}, model.ErrNotFound
	}

	if err != nil {
		return model.Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return value, nil
}

func (r *Repository) Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, error) {
	value, err := r.queryTrip(ctx, r.sql.Update("trips").
		Set("status", model.StatusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(sq.Eq{"id": id, "status": model.StatusActive}).
		Suffix("RETURNING "+tripColumns))
	if err == nil {
		return value, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	existing, err := r.Get(ctx, id)
	if err != nil {
		return model.Trip{}, err
	}

	if existing.Status == model.StatusCompleted {
		return model.Trip{}, model.ErrCompleted
	}

	return model.Trip{}, fmt.Errorf("finish trip: unexpected status %q", existing.Status)
}
