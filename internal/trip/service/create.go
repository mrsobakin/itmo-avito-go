package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

func (s *Service) Create(ctx context.Context, params model.CreateParams, idempotencyKey *uuid.UUID) (model.Trip, bool, error) {
	if err := params.Validate(); err != nil {
		return model.Trip{}, false, err
	}

	var value model.Trip
	var created bool

	err := s.txManager.Do(ctx, func(ctx context.Context) error {
		var err error

		if idempotencyKey == nil {
			value, err = s.create(ctx, params)
			created = true

			return err
		}

		value, created, err = s.createIdempotent(ctx, params, *idempotencyKey)

		return err
	})
	if err != nil {
		return model.Trip{}, false, fmt.Errorf("create trip: %w", err)
	}

	return value, created, nil
}

func (s *Service) create(ctx context.Context, params model.CreateParams) (model.Trip, error) {
	value := model.Trip{
		ID:         uuid.New(),
		UserID:     params.UserID,
		DriverID:   params.DriverID,
		StartPoint: params.StartPoint,
		EndPoint:   params.EndPoint,
		Price:      params.Price,
		Status:     model.StatusActive,
		StartedAt:  s.now(),
	}

	if err := s.repository.Create(ctx, value); err != nil {
		return model.Trip{}, err
	}

	if err := s.repository.AddStatusHistory(ctx, value.ID, nil, model.StatusActive, "trip created"); err != nil {
		return model.Trip{}, err
	}

	return value, nil
}

func (s *Service) createIdempotent(ctx context.Context, params model.CreateParams, key uuid.UUID) (model.Trip, bool, error) {
	hash, err := requestHash(params)
	if err != nil {
		return model.Trip{}, false, err
	}

	if err := s.repository.LockIdempotencyKey(ctx, key); err != nil {
		return model.Trip{}, false, err
	}

	stored, found, err := s.repository.FindIdempotencyKey(ctx, key, s.now().Add(-s.idempotencyKeyTTL))
	if err != nil {
		return model.Trip{}, false, err
	}

	if found {
		if stored.RequestHash != hash {
			return model.Trip{}, false, model.ErrIdempotencyConflict
		}

		value, err := s.repository.Get(ctx, stored.TripID)

		return value, false, err
	}

	value, err := s.create(ctx, params)
	if err != nil {
		return model.Trip{}, false, err
	}

	if err := s.repository.SaveIdempotencyKey(ctx, key, hash, value.ID, value.StartedAt); err != nil {
		return model.Trip{}, false, err
	}

	return value, true, nil
}

func requestHash(params model.CreateParams) (string, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("hash create trip request: %w", err)
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}
