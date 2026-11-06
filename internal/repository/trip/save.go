package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

const (
	pgUniqueViolation           = "23505"
	oneActiveTripPerDriverIndex = "trips_one_active_per_driver_idx"
)

func (r *Repository) Save(ctx context.Context, value trip.Trip) error {
	err := r.db.Exec(ctx, postgres.SQL.Insert(tripsTable).
		Columns(tripColumns...).
		Values(tripValues(value)...))

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == oneActiveTripPerDriverIndex {
		return trip.ErrDriverBusy
	}

	if err != nil {
		return fmt.Errorf("save trip: %w", err)
	}

	return nil
}

func (r *Repository) Update(ctx context.Context, value trip.Trip) error {
	err := r.db.Exec(ctx, postgres.SQL.Update(tripsTable).
		Set("status", value.Status).
		Set("finished_at", value.FinishedAt).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": value.ID}))
	if err != nil {
		return fmt.Errorf("update trip: %w", err)
	}

	return nil
}

func (r *Repository) AddStatusHistory(ctx context.Context, change model.StatusChange) error {
	err := r.db.Exec(ctx, postgres.SQL.Insert(historyTable).
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(change.TripID, change.From, change.To, change.Reason))
	if err != nil {
		return fmt.Errorf("add trip status history: %w", err)
	}

	return nil
}
