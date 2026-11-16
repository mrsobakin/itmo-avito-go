package list_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/http/position/list"
)

func TestList(t *testing.T) {
	id := uuid.New()
	recordedAt := time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)

	tests := []struct {
		name       string
		positions  []trip.Position
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "empty", positions: []trip.Position{}, wantStatus: http.StatusOK, wantBody: `{"positions":[]}`},
		{
			name: "positions",
			positions: []trip.Position{
				{ID: 1, TripID: id, Coordinates: trip.Coordinates{Latitude: 1, Longitude: 2}, RecordedAt: recordedAt},
			},
			wantStatus: http.StatusOK,
			wantBody: `{"positions":[{"coordinates":{"latitude":1,"longitude":2},"id":1,` +
				`"recorded_at":"2026-08-24T12:00:05Z","trip_id":"` + id.String() + `"}]}`,
		},
		{name: "not found", err: fmt.Errorf("wrap: %w", trip.ErrTripNotFound), wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewMockusecase(gomock.NewController(t))
			uc.EXPECT().List(gomock.Any(), id).Return(tt.positions, tt.err)

			r := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String()+"/positions", http.NoBody)
			w := httptest.NewRecorder()
			list.New(uc).Handle(w, r, id)

			require.Equal(t, tt.wantStatus, w.Code)
			if tt.wantBody != "" {
				assert.JSONEq(t, tt.wantBody, w.Body.String())
			}
		})
	}
}
