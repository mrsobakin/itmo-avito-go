package domainerror_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{name: "not found", err: trip.ErrTripNotFound, wantStatus: http.StatusNotFound, wantCode: "trip_not_found"},
		{name: "completed", err: trip.ErrTripCompleted, wantStatus: http.StatusConflict, wantCode: "trip_completed"},
		{name: "driver busy", err: trip.ErrDriverBusy, wantStatus: http.StatusConflict, wantCode: "driver_busy"},
		{
			name: "idempotency conflict", err: trip.ErrIdempotencyConflict,
			wantStatus: http.StatusConflict, wantCode: "idempotency_conflict",
		},
		{
			name: "wrapped domain error", err: fmt.Errorf("finish trip: %w", trip.ErrTripCompleted),
			wantStatus: http.StatusConflict, wantCode: "trip_completed",
		},
		{
			name: "validation", err: fmt.Errorf("create trip: %w", &trip.ValidationError{Field: "price", Reason: "must be non-negative"}),
			wantStatus: http.StatusBadRequest, wantCode: "invalid_request", wantDetail: "price must be non-negative",
		},
		{
			name: "internal", err: fmt.Errorf("save trip: %w", errors.New(`relation "trips" does not exist`)),
			wantStatus: http.StatusInternalServerError, wantCode: "internal_error", wantDetail: "Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := domainerror.Problem(tt.err)

			assert.Equal(t, tt.wantStatus, status)
			assert.EqualValues(t, tt.wantStatus, body.Status)
			assert.Equal(t, tt.wantCode, body.Code)
			assert.NotEmpty(t, body.Title)
			assert.NotEmpty(t, body.Type)
			if tt.wantDetail != "" {
				assert.Equal(t, tt.wantDetail, *body.Detail)
			}
		})
	}
}

func TestProblemDoesNotLeakInternalDetails(t *testing.T) {
	_, body := domainerror.Problem(errors.New("pq: password authentication failed for user tripgo"))

	assert.NotContains(t, *body.Detail, "password")
	assert.NotContains(t, body.Title, "password")
}
