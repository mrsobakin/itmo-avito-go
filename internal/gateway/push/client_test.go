package push_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/mrsobakin/itmo-avito-go/internal/gateway/push"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

func newClient(t *testing.T, handler http.HandlerFunc) *push.Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := push.New(server.URL, time.Second, noop.NewTracerProvider())
	t.Cleanup(client.Close)

	return client
}

func TestRequestPosition(t *testing.T) {
	value := model.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}

	var got map[string]any

	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/push", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message_id":"1","accepted_at":"2026-08-24T12:00:00Z"}`))
	})

	require.NoError(t, client.RequestPosition(t.Context(), value))

	assert.Equal(t, value.DriverID.String(), got["recipient_id"])
	assert.Equal(t, "REQUEST_POSITION", got["kind"])
	assert.Equal(t, map[string]any{"trip_id": value.ID.String()}, got["data"])
	assert.NotEmpty(t, got["request_id"])
}

func TestRequestPositionStatusErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := newClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })

			err := client.RequestPosition(t.Context(), model.StaleTrip{})

			statusErr, ok := errors.AsType[*push.StatusError](err)
			require.True(t, ok)
			assert.Equal(t, status, statusErr.StatusCode)
		})
	}
}

func TestRequestPositionTransportError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	client := push.New(server.URL, time.Second, noop.NewTracerProvider())

	err := client.RequestPosition(t.Context(), model.StaleTrip{})

	require.Error(t, err)
	_, ok := errors.AsType[*push.StatusError](err)
	assert.False(t, ok)
}

func TestRequestPositionTimeout(t *testing.T) {
	release := make(chan struct{})
	client := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusAccepted)
	})
	defer close(release)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	err := client.RequestPosition(ctx, model.StaleTrip{})

	require.ErrorIs(t, err, context.DeadlineExceeded)
}
