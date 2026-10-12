package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

func (s *Service) Finish(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	var value model.Trip

	err := s.txManager.Do(ctx, func(ctx context.Context) error {
		finished, err := s.repository.Finish(ctx, id, s.now())
		if err != nil {
			return err
		}

		from := model.StatusActive
		if err := s.repository.AddStatusHistory(ctx, id, &from, model.StatusCompleted, "trip finished"); err != nil {
			return err
		}

		value = finished

		return nil
	})
	if err != nil {
		return model.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	return value, nil
}
