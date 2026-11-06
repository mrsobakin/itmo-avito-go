package finish

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type Usecase struct {
	trips     tripRepository
	txManager txManager
	completed counter
	logger    *slog.Logger
	now       func() time.Time
}

func New(
	trips tripRepository,
	txManager txManager,
	completed counter,
	logger *slog.Logger,
	now func() time.Time,
) *Usecase {
	return &Usecase{
		trips:     trips,
		txManager: txManager,
		completed: completed,
		logger:    logger,
		now:       now,
	}
}

func (u *Usecase) Finish(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	var value trip.Trip

	err := u.txManager.Do(ctx, func(ctx context.Context) error {
		var err error

		value, err = u.trips.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}

		from := value.Status
		if err := value.Finish(u.now()); err != nil {
			return err
		}

		if err := u.trips.Update(ctx, value); err != nil {
			return err
		}

		return u.trips.AddStatusHistory(ctx, model.StatusChange{
			TripID: id,
			From:   &from,
			To:     value.Status,
			Reason: "trip finished",
		})
	})
	if err != nil {
		return trip.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	u.completed.Inc(ctx)
	u.logger.InfoContext(ctx, "trip completed", "trip_id", value.ID)

	return value, nil
}
