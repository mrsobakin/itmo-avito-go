package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

func (s *Service) Get(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	return s.repository.Get(ctx, id)
}
