package get_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/http/trip/get"
)

func TestGet(t *testing.T) {
	id := uuid.New()

	tests := []struct {
		name       string
		value      trip.Trip
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "ok", value: trip.Trip{ID: id, Status: trip.StatusActive}, wantStatus: http.StatusOK},
		{name: "not found", err: fmt.Errorf("wrap: %w", trip.ErrTripNotFound), wantStatus: http.StatusNotFound, wantCode: "trip_not_found"},

		{name: "internal", err: errors.New("connection refused"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewMockusecase(gomock.NewController(t))
			uc.EXPECT().Get(gomock.Any(), id).Return(tt.value, tt.err)

			r := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String()+"", http.NoBody)
			w := httptest.NewRecorder()
			get.New(uc).Handle(w, r, id)

			require.Equal(t, tt.wantStatus, w.Code)
			if tt.wantCode == "" {
				var got api.Trip
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				assert.Equal(t, id, got.Id)
				assert.Equal(t, api.TripStatus(tt.value.Status), got.Status)
				return
			}

			var problem api.Problem
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
			assert.Equal(t, tt.wantCode, problem.Code)
			assert.Equal(t, r.URL.Path, *problem.Instance)
			assert.NotContains(t, w.Body.String(), "connection refused")
		})
	}
}
