package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/http/health"
	"github.com/mrsobakin/itmo-avito-go/internal/http/router"
	"github.com/mrsobakin/itmo-avito-go/internal/http/trip/create"
	"github.com/mrsobakin/itmo-avito-go/internal/http/trip/finish"
	"github.com/mrsobakin/itmo-avito-go/internal/http/trip/get"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/telemetry"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type fakeTrips struct {
	get    func(id uuid.UUID) (trip.Trip, error)
	finish func(id uuid.UUID) (trip.Trip, error)
}

func (f fakeTrips) Create(context.Context, model.CreateTripInput) (model.CreateTripResult, error) {
	return model.CreateTripResult{Trip: trip.Trip{ID: uuid.New(), Status: trip.StatusActive}, Created: true}, nil
}

func (f fakeTrips) Get(_ context.Context, id uuid.UUID) (trip.Trip, error) { return f.get(id) }

func (f fakeTrips) Finish(_ context.Context, id uuid.UUID) (trip.Trip, error) { return f.finish(id) }

type fakePinger struct{}

func (fakePinger) Ping(context.Context) error { return nil }

type env struct {
	handler http.Handler
	logs    *bytes.Buffer
	spans   *tracetest.SpanRecorder
	metrics *sdkmetric.ManualReader
}

func newEnv(t *testing.T, trips fakeTrips) env {
	t.Helper()

	e := env{logs: &bytes.Buffer{}, spans: tracetest.NewSpanRecorder(), metrics: sdkmetric.NewManualReader()}
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(e.spans))
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(e.metrics))

	handler, err := router.New(router.Handlers{
		Create: create.New(trips),
		Get:    get.New(trips),
		Finish: finish.New(trips),
		Health: health.New(fakePinger{}, time.Second),
	}, router.Telemetry{
		Logger:         telemetry.NewLogger(e.logs, slog.LevelInfo, "trip-service"),
		TracerProvider: tracerProvider,
		Meter:          meterProvider.Meter("test"),
	})
	require.NoError(t, err)
	e.handler = handler

	return e
}

func (e env) do(method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, r)

	return w
}

func (e env) logRecords(t *testing.T) []map[string]any {
	t.Helper()

	var records []map[string]any
	for line := range strings.Lines(e.logs.String()) {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}

	return records
}

func TestRequestLogUsesRouteTemplateAndTraceContext(t *testing.T) {
	id := uuid.New()
	e := newEnv(t, fakeTrips{get: func(uuid.UUID) (trip.Trip, error) { return trip.Trip{}, trip.ErrTripNotFound }})

	w := e.do(http.MethodGet, "/api/v1/trips/"+id.String(), "")

	require.Equal(t, http.StatusNotFound, w.Code)
	records := e.logRecords(t)
	require.Len(t, records, 1)
	record := records[0]
	assert.Equal(t, "request handled", record["msg"])
	assert.Equal(t, "INFO", record["level"])
	assert.Equal(t, "trip-service", record["service"])
	assert.Equal(t, "GET", record["method"])
	assert.Equal(t, "/api/v1/trips/{tripId}", record["route"])
	assert.InDelta(t, http.StatusNotFound, record["status"], 0)
	assert.Contains(t, record, "duration_ms")

	spans := e.spans.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "GET /api/v1/trips/{tripId}", spans[0].Name())
	assert.Equal(t, spans[0].SpanContext().TraceID().String(), record["trace_id"])
	assert.Equal(t, spans[0].SpanContext().SpanID().String(), record["span_id"])
	assert.Contains(t, spans[0].Attributes(), attribute.String("http.route", "/api/v1/trips/{tripId}"))
}

func TestServerErrorIsLoggedOnceAtErrorLevel(t *testing.T) {
	e := newEnv(t, fakeTrips{finish: func(uuid.UUID) (trip.Trip, error) {
		return trip.Trip{}, errors.New("connection refused")
	}})

	w := e.do(http.MethodPost, "/api/v1/trips/"+uuid.NewString()+"/finish", "")

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "connection refused")

	records := e.logRecords(t)
	require.Len(t, records, 1)
	assert.Equal(t, "ERROR", records[0]["level"])
	assert.Equal(t, "connection refused", records[0]["error"])

	spans := e.spans.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "Error", spans[0].Status().Code.String())
	assert.Contains(t, spans[0].Attributes(), attribute.String("error.type", "*errors.errorString"))
}

func TestRecoverReturnsInternalError(t *testing.T) {
	e := newEnv(t, fakeTrips{get: func(uuid.UUID) (trip.Trip, error) { panic("boom") }})

	w := e.do(http.MethodGet, "/api/v1/trips/"+uuid.NewString(), "")

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var problem api.Problem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, "internal_error", problem.Code)

	records := e.logRecords(t)
	require.NotEmpty(t, records)
	assert.Equal(t, "panic recovered", records[0]["msg"])
	assert.Equal(t, "ERROR", records[0]["level"])
	assert.Contains(t, records[0]["stack"], "goroutine")
}

func TestInvalidBodyIsRejected(t *testing.T) {
	valid := `{
		"user_id": "8a0c6f6e-2f0b-4b39-9b8e-6c1f7d3f1a11",
		"driver_id": "f1e2d3c4-b5a6-4789-8abc-def012345678",
		"start_point": {"latitude": 55.75, "longitude": 37.61},
		"end_point": {"latitude": 59.93, "longitude": 30.33},
		"price": 1500
	}`

	tests := []struct {
		name string
		body string
	}{
		{name: "schema violation", body: `{"user_id": "not-a-uuid"}`},
		{name: "malformed json", body: `{"user_id":`},
		{name: "trailing value", body: valid + `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, fakeTrips{})

			w := e.do(http.MethodPost, "/api/v1/trips", tt.body)

			require.Equal(t, http.StatusBadRequest, w.Code)

			var problem api.Problem
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
			assert.Equal(t, "invalid_request", problem.Code)
		})
	}
}

func TestMetricsUseRouteTemplate(t *testing.T) {
	e := newEnv(t, fakeTrips{get: func(id uuid.UUID) (trip.Trip, error) { return trip.Trip{ID: id}, nil }})

	e.do(http.MethodGet, "/api/v1/trips/"+uuid.NewString(), "")
	e.do(http.MethodGet, "/api/v1/trips/"+uuid.NewString(), "")

	var data metricdata.ResourceMetrics
	require.NoError(t, e.metrics.Collect(context.Background(), &data))

	metrics := map[string]metricdata.Aggregation{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			metrics[m.Name] = m.Data
		}
	}

	duration, ok := metrics["http.server.request.duration"].(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, duration.DataPoints, 1)
	point := duration.DataPoints[0]
	assert.Equal(t, uint64(2), point.Count)
	route, _ := point.Attributes.Value("http.route")
	assert.Equal(t, "/api/v1/trips/{tripId}", route.AsString())
	status, _ := point.Attributes.Value("http.response.status_code")
	assert.Equal(t, int64(http.StatusOK), status.AsInt64())

	active, ok := metrics["http.server.active_requests"].(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, active.DataPoints, 1)
	assert.Equal(t, int64(0), active.DataPoints[0].Value)
}
