package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
)

type CreateTripInput struct {
	Params         trip.Params
	IdempotencyKey *uuid.UUID
}

type CreateTripResult struct {
	Trip    trip.Trip
	Created bool
}

type StatusChange struct {
	TripID uuid.UUID
	From   *trip.Status
	To     trip.Status
	Reason string
}

type IdempotencyRecord struct {
	RequestHash string
	TripID      uuid.UUID
}

type StaleTrip struct {
	ID          uuid.UUID
	DriverID    uuid.UUID
	SilentSince time.Time
}

type SavePositionInput struct {
	TripID      uuid.UUID
	Coordinates trip.Coordinates
	RecordedAt  time.Time
}
