package trip_test

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
)

const (
	startLatitude  = "start_point.latitude"
	startLongitude = "start_point.longitude"
)

func validParams() trip.Params {
	return trip.Params{
		UserID:     uuid.New(),
		DriverID:   uuid.New(),
		StartPoint: trip.Coordinates{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   trip.Coordinates{Latitude: 59.93, Longitude: 30.33},
		Price:      1500,
	}
}

func TestNew(t *testing.T) {
	id := uuid.New()
	params := validParams()
	startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	value, err := trip.New(id, params, startedAt)

	require.NoError(t, err)
	assert.Equal(t, trip.Trip{
		ID:         id,
		UserID:     params.UserID,
		DriverID:   params.DriverID,
		StartPoint: params.StartPoint,
		EndPoint:   params.EndPoint,
		Price:      params.Price,
		Status:     trip.StatusActive,
		StartedAt:  startedAt,
	}, value)
}

func TestNewRejectsInvalidParams(t *testing.T) {
	params := validParams()
	params.Price = -1

	_, err := trip.New(uuid.New(), params, time.Now())

	var validationErr *trip.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "price", validationErr.Field)
}

func TestFinish(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name       string
		status     trip.Status
		wantErr    error
		wantStatus trip.Status
		wantAt     *time.Time
	}{
		{name: "active trip", status: trip.StatusActive, wantStatus: trip.StatusCompleted, wantAt: &at},
		{name: "completed trip", status: trip.StatusCompleted, wantErr: trip.ErrTripCompleted, wantStatus: trip.StatusCompleted},
		{name: "unknown status", status: "cancelled", wantErr: trip.ErrTripCompleted, wantStatus: "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := trip.Trip{Status: tt.status}

			err := value.Finish(at)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantStatus, value.Status)
			assert.Equal(t, tt.wantAt, value.FinishedAt)
		})
	}
}

func TestParamsValidate(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(p *trip.Params)
		wantField string
	}{
		{name: "valid", modify: func(*trip.Params) {}},
		{name: "zero price", modify: func(p *trip.Params) { p.Price = 0 }},
		{name: "boundary coordinates", modify: func(p *trip.Params) {
			p.StartPoint = trip.Coordinates{Latitude: -90, Longitude: -180}
			p.EndPoint = trip.Coordinates{Latitude: 90, Longitude: 180}
		}},
		{name: "nil user", modify: func(p *trip.Params) { p.UserID = uuid.Nil }, wantField: "user_id"},
		{name: "nil driver", modify: func(p *trip.Params) { p.DriverID = uuid.Nil }, wantField: "driver_id"},
		{name: "start latitude too low", modify: func(p *trip.Params) { p.StartPoint.Latitude = -90.01 }, wantField: startLatitude},
		{name: "start latitude too high", modify: func(p *trip.Params) { p.StartPoint.Latitude = 90.01 }, wantField: startLatitude},
		{name: "start longitude too low", modify: func(p *trip.Params) { p.StartPoint.Longitude = -180.01 }, wantField: startLongitude},
		{name: "start longitude too high", modify: func(p *trip.Params) { p.StartPoint.Longitude = 180.01 }, wantField: startLongitude},
		{name: "start latitude NaN", modify: func(p *trip.Params) { p.StartPoint.Latitude = math.NaN() }, wantField: startLatitude},
		{name: "start longitude NaN", modify: func(p *trip.Params) { p.StartPoint.Longitude = math.NaN() }, wantField: startLongitude},
		{name: "end latitude +Inf", modify: func(p *trip.Params) { p.EndPoint.Latitude = math.Inf(1) }, wantField: "end_point.latitude"},
		{name: "end latitude", modify: func(p *trip.Params) { p.EndPoint.Latitude = 91 }, wantField: "end_point.latitude"},
		{name: "end longitude", modify: func(p *trip.Params) { p.EndPoint.Longitude = -181 }, wantField: "end_point.longitude"},
		{name: "negative price", modify: func(p *trip.Params) { p.Price = -1 }, wantField: "price"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := validParams()
			tt.modify(&params)

			err := params.Validate()

			if tt.wantField == "" {
				require.NoError(t, err)
				return
			}

			var validationErr *trip.ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Equal(t, tt.wantField, validationErr.Field)
			assert.Contains(t, err.Error(), tt.wantField)
		})
	}
}
