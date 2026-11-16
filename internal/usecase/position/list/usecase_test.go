package list_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/usecase/position/list"
)

func newUsecase(t *testing.T) (*MocktripRepository, *MockpositionRepository, *list.Usecase) {
	ctrl := gomock.NewController(t)
	trips, positions := NewMocktripRepository(ctrl), NewMockpositionRepository(ctrl)

	return trips, positions, list.New(trips, positions)
}

func TestList(t *testing.T) {
	trips, positions, usecase := newUsecase(t)
	id := uuid.New()
	want := []trip.Position{{ID: 1, TripID: id}, {ID: 2, TripID: id}}
	trips.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{ID: id}, nil)
	positions.EXPECT().List(gomock.Any(), id).Return(want, nil)

	got, err := usecase.List(context.Background(), id)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListTripNotFound(t *testing.T) {
	trips, _, usecase := newUsecase(t)
	id := uuid.New()
	trips.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)

	_, err := usecase.List(context.Background(), id)

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestListStorageError(t *testing.T) {
	trips, positions, usecase := newUsecase(t)
	id := uuid.New()
	errDB := errors.New("connection reset")
	trips.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{ID: id}, nil)
	positions.EXPECT().List(gomock.Any(), id).Return(nil, errDB)

	_, err := usecase.List(context.Background(), id)

	require.ErrorIs(t, err, errDB)
}
