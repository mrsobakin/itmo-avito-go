package create_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/http/trip/create"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

const invalidRequest = "invalid_request"

const body = `{
	"user_id": "8a0c6f6e-2f0b-4b39-9b8e-6c1f7d3f1a11",
	"driver_id": "f1e2d3c4-b5a6-4789-8abc-def012345678",
	"start_point": {"latitude": 55.75, "longitude": 37.61},
	"end_point": {"latitude": 59.93, "longitude": 30.33},
	"price": 1500
}`

func created() trip.Trip {
	return trip.Trip{
		ID:         uuid.New(),
		UserID:     uuid.MustParse("8a0c6f6e-2f0b-4b39-9b8e-6c1f7d3f1a11"),
		DriverID:   uuid.MustParse("f1e2d3c4-b5a6-4789-8abc-def012345678"),
		StartPoint: trip.Coordinates{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   trip.Coordinates{Latitude: 59.93, Longitude: 30.33},
		Price:      1500,
		Status:     trip.StatusActive,
		StartedAt:  time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
	}
}

func serve(t *testing.T, uc *Mockusecase, requestBody string, params api.CreateTripParams) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(requestBody))
	w := httptest.NewRecorder()
	create.New(uc).Handle(w, r, params)

	return w
}

func decodeProblem(t *testing.T, w *httptest.ResponseRecorder) api.Problem {
	t.Helper()

	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var problem api.Problem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))

	return problem
}

func TestCreate(t *testing.T) {
	uc := NewMockusecase(gomock.NewController(t))
	value := created()
	key := uuid.New()

	uc.EXPECT().Create(gomock.Any(), model.CreateTripInput{
		Params: trip.Params{
			UserID:     value.UserID,
			DriverID:   value.DriverID,
			StartPoint: value.StartPoint,
			EndPoint:   value.EndPoint,
			Price:      value.Price,
		},
		IdempotencyKey: &key,
	}).Return(model.CreateTripResult{Trip: value, Created: true}, nil)

	w := serve(t, uc, body, api.CreateTripParams{IdempotencyKey: &key})

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "/api/v1/trips/"+value.ID.String(), w.Header().Get("Location"))

	var got api.Trip
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, value.ID, got.Id)
	assert.Equal(t, api.Active, got.Status)
	assert.Equal(t, value.StartedAt, got.StartedAt)
}

func TestCreateReplay(t *testing.T) {
	uc := NewMockusecase(gomock.NewController(t))
	uc.EXPECT().Create(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{Trip: created()}, nil)

	w := serve(t, uc, body, api.CreateTripParams{})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Location"))
}

func TestCreateErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		usecaseErr error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid json", body: `{"user_id":`, wantStatus: http.StatusBadRequest, wantCode: invalidRequest},
		{name: "trailing data", body: body + `{}`, wantStatus: http.StatusBadRequest, wantCode: invalidRequest},
		{
			name: "validation", body: body,
			usecaseErr: &trip.ValidationError{Field: "price", Reason: "must be non-negative"},
			wantStatus: http.StatusBadRequest, wantCode: invalidRequest,
		},
		{
			name: "driver busy", body: body, usecaseErr: fmt.Errorf("create trip: %w", trip.ErrDriverBusy),
			wantStatus: http.StatusConflict, wantCode: "driver_busy",
		},
		{
			name: "idempotency conflict", body: body, usecaseErr: trip.ErrIdempotencyConflict,
			wantStatus: http.StatusConflict, wantCode: "idempotency_conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewMockusecase(gomock.NewController(t))
			if tt.usecaseErr != nil {
				uc.EXPECT().Create(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{}, tt.usecaseErr)
			}

			w := serve(t, uc, tt.body, api.CreateTripParams{})

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeProblem(t, w).Code)
		})
	}
}

func TestCreateInternalErrorHidesDetails(t *testing.T) {
	uc := NewMockusecase(gomock.NewController(t))
	uc.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(model.CreateTripResult{}, errors.New(`ERROR: relation "trips" does not exist (SQLSTATE 42P01)`))

	w := serve(t, uc, body, api.CreateTripParams{})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "internal_error", decodeProblem(t, w).Code)
	assert.NotContains(t, w.Body.String(), "relation")
	assert.NotContains(t, w.Body.String(), "SQLSTATE")
}
