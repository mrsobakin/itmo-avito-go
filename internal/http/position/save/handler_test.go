package save_test

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
	"github.com/mrsobakin/itmo-avito-go/internal/http/position/save"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

const body = `{"coordinates": {"latitude": 59.935, "longitude": 30.335}, "recorded_at": "2026-08-24T12:00:05Z"}`

func TestSave(t *testing.T) {
	id := uuid.New()
	recordedAt := time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)
	input := model.SavePositionInput{
		TripID:      id,
		Coordinates: trip.Coordinates{Latitude: 59.935, Longitude: 30.335},
		RecordedAt:  recordedAt,
	}
	saved := trip.Position{ID: 42, TripID: id, Coordinates: input.Coordinates, RecordedAt: recordedAt}

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "ok", wantStatus: http.StatusCreated},
		{name: "validation", err: &trip.ValidationError{Field: "recorded_at", Reason: "must not be in the future"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "not found", err: fmt.Errorf("wrap: %w", trip.ErrTripNotFound), wantStatus: http.StatusNotFound, wantCode: "trip_not_found"},
		{name: "completed", err: fmt.Errorf("wrap: %w", trip.ErrTripCompleted), wantStatus: http.StatusConflict, wantCode: "trip_completed"},
		{name: "internal", err: errors.New("connection refused"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewMockusecase(gomock.NewController(t))
			uc.EXPECT().Save(gomock.Any(), input).Return(saved, tt.err)

			r := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/positions", strings.NewReader(body))
			w := httptest.NewRecorder()
			save.New(uc).Handle(w, r, id)

			require.Equal(t, tt.wantStatus, w.Code)
			if tt.wantCode == "" {
				var got api.TripPosition
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				assert.Equal(t, api.TripPosition{
					Id:          42,
					TripId:      id,
					Coordinates: api.Coordinates{Latitude: 59.935, Longitude: 30.335},
					RecordedAt:  recordedAt,
				}, got)

				return
			}

			var problem api.Problem
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
			assert.Equal(t, tt.wantCode, problem.Code)
			assert.NotContains(t, w.Body.String(), "connection refused")
		})
	}
}

func TestSaveRejectsMalformedBody(t *testing.T) {
	uc := NewMockusecase(gomock.NewController(t))
	id := uuid.New()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/positions", strings.NewReader(body+"{}"))
	w := httptest.NewRecorder()
	save.New(uc).Handle(w, r, id)

	require.Equal(t, http.StatusBadRequest, w.Code)
}
