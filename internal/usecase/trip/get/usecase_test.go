package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/usecase/trip/get"
)

func TestGet(t *testing.T) {
	value := trip.Trip{ID: uuid.New(), Status: trip.StatusActive}
	errDB := errors.New("connection reset")

	tests := []struct {
		name    string
		repoErr error
		want    trip.Trip
		wantErr error
	}{
		{name: "found", want: value},
		{name: "not found", repoErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "storage error", repoErr: errDB, wantErr: errDB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMocktripRepository(gomock.NewController(t))
			repo.EXPECT().Get(gomock.Any(), value.ID).Return(tt.want, tt.repoErr)

			got, err := get.New(repo).Get(context.Background(), value.ID)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}
