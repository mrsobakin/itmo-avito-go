package list

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
)

type Usecase struct {
	trips     tripRepository
	positions positionRepository
}

func New(trips tripRepository, positions positionRepository) *Usecase {
	return &Usecase{trips: trips, positions: positions}
}

func (u *Usecase) List(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error) {
	if _, err := u.trips.Get(ctx, tripID); err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}

	positions, err := u.positions.List(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}

	return positions, nil
}
