package save

import (
	"context"
	"fmt"
	"time"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type Usecase struct {
	trips     tripRepository
	positions positionRepository
	txManager txManager
	saved     counter
	requests  positionRequests
	now       func() time.Time
}

func New(
	trips tripRepository,
	positions positionRepository,
	txManager txManager,
	saved counter,
	requests positionRequests,
	now func() time.Time,
) *Usecase {
	return &Usecase{
		trips:     trips,
		positions: positions,
		txManager: txManager,
		saved:     saved,
		requests:  requests,
		now:       now,
	}
}

func (u *Usecase) Save(ctx context.Context, input model.SavePositionInput) (trip.Position, error) {
	value, err := trip.NewPosition(input.TripID, input.Coordinates, input.RecordedAt, u.now())
	if err != nil {
		return trip.Position{}, err
	}

	err = u.txManager.Do(ctx, func(ctx context.Context) error {
		current, err := u.trips.GetForUpdate(ctx, input.TripID)
		if err != nil {
			return err
		}

		if err := current.CanRecordPosition(); err != nil {
			return err
		}

		value, err = u.positions.Save(ctx, value)
		if err != nil {
			return err
		}

		return u.trips.UpdateLastPosition(ctx, input.TripID, value.RecordedAt)
	})
	if err != nil {
		return trip.Position{}, fmt.Errorf("save position: %w", err)
	}

	u.saved.Inc(ctx)
	u.requests.Cancel(ctx, input.TripID)

	return value, nil
}
