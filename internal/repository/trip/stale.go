package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

const silentSince = "GREATEST(last_position_at, started_at)"

func (r *Repository) UpdateLastPosition(ctx context.Context, id uuid.UUID, at time.Time) error {
	err := r.db.Exec(ctx, postgres.SQL.Update(tripsTable).
		Set("last_position_at", sq.Expr("GREATEST(last_position_at, ?::timestamptz)", at)).
		Where(sq.Eq{"id": id}))
	if err != nil {
		return fmt.Errorf("update last position: %w", err)
	}

	return nil
}

func (r *Repository) ListStale(
	ctx context.Context, staleAfter time.Duration, after *model.StaleTrip, limit uint64,
) ([]model.StaleTrip, error) {
	query := postgres.SQL.Select("id", "driver_id", silentSince).
		From(tripsTable).
		Where("status = 'active'").
		Where(silentSince+" < now() - make_interval(secs => ?)", staleAfter.Seconds()).
		OrderBy(silentSince, "id").
		Limit(limit)

	if after != nil {
		query = query.Where("("+silentSince+", id) > (?, ?)", after.SilentSince, after.ID)
	}

	var trips []model.StaleTrip

	err := r.db.Query(ctx, query, func(row pgx.Row) error {
		var value model.StaleTrip
		if err := row.Scan(&value.ID, &value.DriverID, &value.SilentSince); err != nil {
			return err
		}

		value.SilentSince = value.SilentSince.UTC()
		trips = append(trips, value)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list stale trips: %w", err)
	}

	return trips, nil
}
