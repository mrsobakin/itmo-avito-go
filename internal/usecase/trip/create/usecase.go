package create

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type Usecase struct {
	trips     tripRepository
	keys      idempotencyRepository
	txManager txManager
	created   counter
	logger    *slog.Logger
	keyTTL    time.Duration
	now       func() time.Time
}

func New(
	trips tripRepository,
	keys idempotencyRepository,
	txManager txManager,
	created counter,
	logger *slog.Logger,
	keyTTL time.Duration,
	now func() time.Time,
) *Usecase {
	return &Usecase{
		trips:     trips,
		keys:      keys,
		txManager: txManager,
		created:   created,
		logger:    logger,
		keyTTL:    keyTTL,
		now:       now,
	}
}

func (u *Usecase) Create(ctx context.Context, input model.CreateTripInput) (model.CreateTripResult, error) {
	value, err := trip.New(uuid.New(), input.Params, u.now())
	if err != nil {
		return model.CreateTripResult{}, err
	}

	result := model.CreateTripResult{Trip: value, Created: true}

	err = u.txManager.Do(ctx, func(ctx context.Context) error {
		if input.IdempotencyKey == nil {
			return u.save(ctx, value)
		}

		var err error
		result, err = u.createIdempotent(ctx, value, input.Params, *input.IdempotencyKey)

		return err
	})
	if err != nil {
		return model.CreateTripResult{}, fmt.Errorf("create trip: %w", err)
	}

	if result.Created {
		u.created.Inc(ctx)
		u.logger.InfoContext(ctx, "trip created", "trip_id", result.Trip.ID, "driver_id", result.Trip.DriverID)
	}

	return result, nil
}

func (u *Usecase) save(ctx context.Context, value trip.Trip) error {
	if err := u.trips.Save(ctx, value); err != nil {
		return err
	}

	return u.trips.AddStatusHistory(ctx, model.StatusChange{
		TripID: value.ID,
		To:     trip.StatusActive,
		Reason: "trip created",
	})
}

func (u *Usecase) createIdempotent(
	ctx context.Context,
	value trip.Trip,
	params trip.Params,
	key uuid.UUID,
) (model.CreateTripResult, error) {
	hash := requestHash(params)

	if err := u.keys.Lock(ctx, key); err != nil {
		return model.CreateTripResult{}, err
	}

	stored, found, err := u.keys.Find(ctx, key, value.StartedAt.Add(-u.keyTTL))
	if err != nil {
		return model.CreateTripResult{}, err
	}

	if found {
		if stored.RequestHash != hash {
			return model.CreateTripResult{}, trip.ErrIdempotencyConflict
		}

		existing, err := u.trips.Get(ctx, stored.TripID)

		return model.CreateTripResult{Trip: existing}, err
	}

	if err := u.save(ctx, value); err != nil {
		return model.CreateTripResult{}, err
	}

	record := model.IdempotencyRecord{RequestHash: hash, TripID: value.ID}
	if err := u.keys.Save(ctx, key, record, value.StartedAt); err != nil {
		return model.CreateTripResult{}, err
	}

	return model.CreateTripResult{Trip: value, Created: true}, nil
}

func requestHash(params trip.Params) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%+v", params))

	return hex.EncodeToString(sum[:])
}
