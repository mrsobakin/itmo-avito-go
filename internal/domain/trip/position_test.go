package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
)

func TestNewPosition(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	tripID := uuid.New()
	valid := trip.Coordinates{Latitude: 59.93, Longitude: 30.33}

	tests := []struct {
		name        string
		coordinates trip.Coordinates
		recordedAt  time.Time
		wantField   string
	}{
		{name: "past", coordinates: valid, recordedAt: now.Add(-time.Hour)},
		{name: "within clock skew", coordinates: valid, recordedAt: now.Add(trip.MaxPositionClockSkew)},
		{name: "too far in future", coordinates: valid, recordedAt: now.Add(trip.MaxPositionClockSkew + time.Second), wantField: "recorded_at"},
		{name: "latitude", coordinates: trip.Coordinates{Latitude: 91}, recordedAt: now, wantField: "coordinates.latitude"},
		{name: "longitude", coordinates: trip.Coordinates{Longitude: -181}, recordedAt: now, wantField: "coordinates.longitude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := trip.NewPosition(tripID, tt.coordinates, tt.recordedAt, now)

			if tt.wantField == "" {
				require.NoError(t, err)
				assert.Equal(t, trip.Position{TripID: tripID, Coordinates: tt.coordinates, RecordedAt: tt.recordedAt}, got)

				return
			}

			validationErr, ok := errors.AsType[*trip.ValidationError](err)
			require.True(t, ok)
			assert.Equal(t, tt.wantField, validationErr.Field)
		})
	}
}

func TestCanRecordPosition(t *testing.T) {
	active := trip.Trip{Status: trip.StatusActive}
	completed := trip.Trip{Status: trip.StatusCompleted}

	require.NoError(t, active.CanRecordPosition())
	require.ErrorIs(t, completed.CanRecordPosition(), trip.ErrTripCompleted)
}
