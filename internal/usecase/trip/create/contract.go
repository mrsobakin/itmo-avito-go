//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type (
	tripRepository interface {
		Save(ctx context.Context, value trip.Trip) error
		Get(ctx context.Context, id uuid.UUID) (trip.Trip, error)
		AddStatusHistory(ctx context.Context, change model.StatusChange) error
	}

	idempotencyRepository interface {
		Lock(ctx context.Context, key uuid.UUID) error
		Find(ctx context.Context, key uuid.UUID, createdAfter time.Time) (model.IdempotencyRecord, bool, error)
		Save(ctx context.Context, key uuid.UUID, record model.IdempotencyRecord, createdAt time.Time) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	counter interface {
		Inc(ctx context.Context)
	}
)
