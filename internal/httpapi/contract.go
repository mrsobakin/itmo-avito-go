package httpapi

import (
	"context"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

type tripService interface {
	Create(ctx context.Context, params model.CreateParams, idempotencyKey *uuid.UUID) (model.Trip, bool, error)
	Get(ctx context.Context, id uuid.UUID) (model.Trip, error)
	Finish(ctx context.Context, id uuid.UUID) (model.Trip, error)
}

type pinger interface {
	Ping(ctx context.Context) error
}
