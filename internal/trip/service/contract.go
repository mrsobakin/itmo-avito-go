package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

type repository interface {
	Create(ctx context.Context, value model.Trip) error
	AddStatusHistory(ctx context.Context, tripID uuid.UUID, from *model.Status, to model.Status, reason string) error
	Get(ctx context.Context, id uuid.UUID) (model.Trip, error)
	Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, error)

	LockIdempotencyKey(ctx context.Context, key uuid.UUID) error
	FindIdempotencyKey(ctx context.Context, key uuid.UUID, createdAfter time.Time) (model.IdempotencyRecord, bool, error)
	SaveIdempotencyKey(ctx context.Context, key uuid.UUID, requestHash string, tripID uuid.UUID, createdAt time.Time) error
}
