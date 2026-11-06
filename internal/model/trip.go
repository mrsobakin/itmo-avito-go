package model

import (
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
