//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package save

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
)

type (
	tripRepository interface {
		GetForUpdate(ctx context.Context, id uuid.UUID) (trip.Trip, error)
		UpdateLastPosition(ctx context.Context, id uuid.UUID, at time.Time) error
	}

	positionRepository interface {
		Save(ctx context.Context, value trip.Position) (trip.Position, error)
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	counter interface {
		Inc(ctx context.Context)
	}

	positionRequests interface {
		Cancel(ctx context.Context, tripID uuid.UUID)
	}
)
