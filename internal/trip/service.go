package trip

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/transaction"
)

type Service struct {
	repository        *Repository
	txManager         transaction.TxManager
	idempotencyKeyTTL time.Duration
	now               func() time.Time
}

func NewService(repository *Repository, txManager transaction.TxManager, idempotencyKeyTTL time.Duration) *Service {
	return &Service{
		repository:        repository,
		txManager:         txManager,
		idempotencyKeyTTL: idempotencyKeyTTL,
		now:               func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	}
}

func (s *Service) Create(ctx context.Context, params CreateParams, idempotencyKey *uuid.UUID) (Trip, bool, error) {
	var value Trip
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
		return Trip{}, false, fmt.Errorf("create trip: %w", err)
	}
	return value, created, nil
}

func (s *Service) create(ctx context.Context, params CreateParams) (Trip, error) {
	value := Trip{
		ID:         uuid.New(),
		UserID:     params.UserID,
		DriverID:   params.DriverID,
		StartPoint: params.StartPoint,
		EndPoint:   params.EndPoint,
		Price:      params.Price,
		Status:     StatusActive,
		StartedAt:  s.now(),
	}
	if err := s.repository.Create(ctx, value); err != nil {
		return Trip{}, err
	}
	if err := s.repository.AddStatusHistory(ctx, value.ID, nil, StatusActive, "trip created"); err != nil {
		return Trip{}, err
	}
	return value, nil
}

func (s *Service) createIdempotent(ctx context.Context, params CreateParams, key uuid.UUID) (Trip, bool, error) {
	hash, err := requestHash(params)
	if err != nil {
		return Trip{}, false, err
	}
	if err := s.repository.LockIdempotencyKey(ctx, key); err != nil {
		return Trip{}, false, err
	}

	stored, found, err := s.repository.FindIdempotencyKey(ctx, key, s.now().Add(-s.idempotencyKeyTTL))
	if err != nil {
		return Trip{}, false, err
	}
	if found {
		if stored.RequestHash != hash {
			return Trip{}, false, ErrIdempotencyConflict
		}
		value, err := s.repository.Get(ctx, stored.TripID)
		return value, false, err
	}

	value, err := s.create(ctx, params)
	if err != nil {
		return Trip{}, false, err
	}
	if err := s.repository.SaveIdempotencyKey(ctx, key, hash, value.ID, value.StartedAt); err != nil {
		return Trip{}, false, err
	}
	return value, true, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Trip, error) {
	return s.repository.Get(ctx, id)
}

func (s *Service) Finish(ctx context.Context, id uuid.UUID) (Trip, error) {
	var value Trip
	err := s.txManager.Do(ctx, func(ctx context.Context) error {
		finished, err := s.repository.Finish(ctx, id, s.now())
		if err != nil {
			return err
		}
		from := StatusActive
		if err := s.repository.AddStatusHistory(ctx, id, &from, StatusCompleted, "trip finished"); err != nil {
			return err
		}
		value = finished
		return nil
	})
	if err != nil {
		return Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	return value, nil
}

func requestHash(params CreateParams) (string, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("hash create trip request: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
