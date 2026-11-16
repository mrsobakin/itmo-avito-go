package positions

import (
	"context"
	"errors"
	"log/slog"
	"slices"
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
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

var (
	base       = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	errPush    = errors.New("push service unavailable")
	staleAfter = time.Minute
)

const batchSize = 100

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type fakeTicker struct {
	c chan time.Time
}

func (t fakeTicker) C() <-chan time.Time { return t.c }

func (fakeTicker) Stop() {}

type env struct {
	service *Service
	trips   *MocktripRepository
	push    *Mockpusher
	ticker  fakeTicker
	metrics *sdkmetric.ManualReader
	spans   *tracetest.SpanRecorder
}

func newEnv(t *testing.T, configure func(*Options)) env {
	t.Helper()

	ctrl := gomock.NewController(t)
	e := env{
		trips:   NewMocktripRepository(ctrl),
		push:    NewMockpusher(ctrl),
		ticker:  fakeTicker{c: make(chan time.Time)},
		metrics: sdkmetric.NewManualReader(),
		spans:   tracetest.NewSpanRecorder(),
	}

	opts := Options{
		CheckInterval:  30 * time.Second,
		StaleAfter:     staleAfter,
		Workers:        2,
		QueueSize:      10,
		ScanBatchSize:  batchSize,
		RequestTimeout: time.Second,
	}
	if configure != nil {
		configure(&opts)
	}

	service, err := New(opts, Deps{
		Trips:     e.trips,
		Push:      e.push,
		NewTicker: func(time.Duration) Ticker { return e.ticker },
		Meter:     sdkmetric.NewMeterProvider(sdkmetric.WithReader(e.metrics)).Meter("test"),
		Tracer:    sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(e.spans)).Tracer("test"),
		Logger:    slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)
	e.service = service

	return e
}

func (e env) count(t *testing.T, name, result string) int64 {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, e.metrics.Collect(t.Context(), &data))

	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if m.Name != name || !ok {
				continue
			}

			for _, point := range sum.DataPoints {
				if value, _ := point.Attributes.Value("result"); value.AsString() == result {
					return point.Value
				}
			}
		}
	}

	return 0
}

func (e env) expectScan(trips ...model.StaleTrip) {
	e.trips.EXPECT().ListStale(gomock.Any(), staleAfter, nil, uint64(batchSize)).Return(trips, nil)
}

func (e env) expectPages(t *testing.T, trips []model.StaleTrip) *gomock.Call {
	t.Helper()

	return e.trips.EXPECT().ListStale(gomock.Any(), staleAfter, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ time.Duration, after *model.StaleTrip, limit uint64) ([]model.StaleTrip, error) {
			assert.Equal(t, uint64(batchSize), limit)

			start := 0
			if after != nil {
				start = slices.IndexFunc(trips, func(v model.StaleTrip) bool { return v.ID == after.ID }) + 1
			}

			return trips[start:min(start+batchSize, len(trips))], nil
		},
	)
}

func staleTrips(n int) []model.StaleTrip {
	trips := make([]model.StaleTrip, n)
	for i := range trips {
		trips[i] = model.StaleTrip{ID: uuid.New(), DriverID: uuid.New(), SilentSince: base.Add(time.Duration(i) * time.Second)}
	}

	return trips
}

func TestTickTriggersScan(t *testing.T) {
	e := newEnv(t, nil)
	scanned := make(chan struct{})
	e.trips.EXPECT().ListStale(gomock.Any(), staleAfter, nil, uint64(batchSize)).
		DoAndReturn(func(context.Context, time.Duration, *model.StaleTrip, uint64) ([]model.StaleTrip, error) {
			close(scanned)
			return nil, nil
		})

	e.service.Start(t.Context())
	e.ticker.c <- base
	<-scanned

	require.NoError(t, e.service.Stop(t.Context()))
}

func TestTickedTripsReachIdleWorkers(t *testing.T) {
	e := newEnv(t, nil)
	trips := staleTrips(3)
	e.expectScan(trips...)

	requested := make(chan model.StaleTrip, len(trips))
	e.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any()).Times(len(trips)).
		DoAndReturn(func(_ context.Context, value model.StaleTrip) error {
			requested <- value
			return nil
		})

	e.service.Start(t.Context())
	e.ticker.c <- base

	got := make([]model.StaleTrip, 0, len(trips))
	for range trips {
		got = append(got, <-requested)
	}

	require.NoError(t, e.service.Stop(t.Context()))
	assert.ElementsMatch(t, trips, got)
}

func TestContextCancelWakesIdleWorkers(t *testing.T) {
	e := newEnv(t, nil)
	ctx, cancel := context.WithCancel(t.Context())

	e.service.Start(ctx)
	cancel()
	e.service.workers.Wait()
	e.service.scanner.Wait()

	require.NoError(t, e.service.Stop(t.Context()))
}

func TestRepeatedScanDoesNotDuplicateQueuedTrips(t *testing.T) {
	e := newEnv(t, nil)
	trips := staleTrips(3)
	e.expectScan(trips...)
	e.expectScan(trips...)

	e.service.scan(t.Context())
	e.service.scan(t.Context())

	assert.Equal(t, 3, e.service.queue.len())
	assert.Equal(t, int64(3), e.count(t, "tripgo.stale_checks", checkQueued))
	assert.Equal(t, int64(3), e.count(t, "tripgo.stale_checks", checkDuplicate))
}

func TestPositionCancelsQueuedRequest(t *testing.T) {
	e := newEnv(t, nil)
	trips := staleTrips(2)
	e.expectScan(trips...)
	e.service.scan(t.Context())

	e.service.Cancel(t.Context(), trips[0].ID)
	e.service.Cancel(t.Context(), uuid.New())
	e.push.EXPECT().RequestPosition(gomock.Any(), trips[1]).Return(nil)

	e.service.Start(t.Context())
	require.NoError(t, e.service.Stop(t.Context()))

	assert.Equal(t, int64(1), e.count(t, "tripgo.position_requests", requestCancelled))
	assert.Equal(t, int64(1), e.count(t, "tripgo.position_requests", requestSuccess))
}

func TestFullQueueDropsInsteadOfBlocking(t *testing.T) {
	e := newEnv(t, func(o *Options) { o.QueueSize = 2 })
	e.expectScan(staleTrips(5)...)

	e.service.scan(t.Context())

	assert.Equal(t, 2, e.service.queue.len())
	assert.Equal(t, int64(2), e.count(t, "tripgo.stale_checks", checkQueued))
	assert.Equal(t, int64(1), e.count(t, "tripgo.stale_checks", checkDropped))
}

func TestFullQueueEvictsFreshestTrip(t *testing.T) {
	e := newEnv(t, func(o *Options) {
		o.QueueSize = 2
		o.Workers = 1
	})
	trips := staleTrips(3)
	e.expectScan(trips[1], trips[2])
	e.expectScan(trips...)

	e.service.scan(t.Context())
	e.service.scan(t.Context())

	assert.Equal(t, int64(3), e.count(t, "tripgo.stale_checks", checkQueued))
	assert.Equal(t, int64(1), e.count(t, "tripgo.stale_checks", checkDuplicate))
	assert.Equal(t, int64(2), e.count(t, "tripgo.stale_checks", checkDropped))

	gomock.InOrder(
		e.push.EXPECT().RequestPosition(gomock.Any(), trips[0]).Return(nil),
		e.push.EXPECT().RequestPosition(gomock.Any(), trips[1]).Return(nil),
	)

	e.service.Start(t.Context())
	require.NoError(t, e.service.Stop(t.Context()))
}

func TestScanReadsBatchesUntilExhausted(t *testing.T) {
	e := newEnv(t, func(o *Options) { o.QueueSize = 300 })
	e.expectPages(t, staleTrips(250)).Times(3)

	e.service.scan(t.Context())

	assert.Equal(t, 250, e.service.queue.len())
	assert.Equal(t, int64(250), e.count(t, "tripgo.stale_checks", checkQueued))
}

func TestScanStopsReadingAfterDrop(t *testing.T) {
	e := newEnv(t, func(o *Options) { o.QueueSize = 50 })
	e.expectPages(t, staleTrips(250)).Times(1)

	e.service.scan(t.Context())

	assert.Equal(t, 50, e.service.queue.len())
	assert.Equal(t, int64(1), e.count(t, "tripgo.stale_checks", checkDropped))
}

func TestScanErrorIsNotFatal(t *testing.T) {
	e := newEnv(t, nil)
	e.trips.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("db down"))
	e.expectScan(staleTrips(1)...)

	e.service.scan(t.Context())
	e.service.scan(t.Context())

	assert.Equal(t, 1, e.service.queue.len())
}

func TestWorkersProcessTasksConcurrently(t *testing.T) {
	const workers, tasks = 3, 9

	e := newEnv(t, func(o *Options) { o.Workers = workers })
	e.expectScan(staleTrips(tasks)...)
	e.service.scan(t.Context())

	started := make(chan struct{}, tasks)
	release := make(chan struct{})
	e.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any()).Times(tasks).
		DoAndReturn(func(context.Context, model.StaleTrip) error {
			started <- struct{}{}
			<-release

			return nil
		})

	e.service.Start(t.Context())
	for range workers {
		<-started
	}

	assert.Equal(t, int64(workers), e.service.active.Load())
	assert.Equal(t, tasks-workers, e.service.queue.len())

	close(release)
	require.NoError(t, e.service.Stop(t.Context()))

	assert.Equal(t, int64(tasks), e.count(t, "tripgo.position_requests", requestSuccess))
	assert.Zero(t, e.service.active.Load())
}

func TestPushErrorDoesNotStopWorker(t *testing.T) {
	e := newEnv(t, func(o *Options) { o.Workers = 1 })
	trips := staleTrips(2)
	e.expectScan(trips...)
	e.service.scan(t.Context())

	gomock.InOrder(
		e.push.EXPECT().RequestPosition(gomock.Any(), trips[0]).Return(errPush),
		e.push.EXPECT().RequestPosition(gomock.Any(), trips[1]).Return(nil),
	)

	e.service.Start(t.Context())
	require.NoError(t, e.service.Stop(t.Context()))

	assert.Equal(t, int64(1), e.count(t, "tripgo.position_requests", requestError))
	assert.Equal(t, int64(1), e.count(t, "tripgo.position_requests", requestSuccess))
}

func TestStopDrainsQueue(t *testing.T) {
	e := newEnv(t, nil)
	e.expectScan(staleTrips(5)...)
	e.service.scan(t.Context())
	e.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any()).Times(5).Return(nil)

	e.service.Start(t.Context())
	require.NoError(t, e.service.Stop(t.Context()))

	assert.Zero(t, e.service.queue.len())
}

func TestWorkerSpanLinksToScan(t *testing.T) {
	e := newEnv(t, nil)
	trips := staleTrips(1)
	e.expectScan(trips...)
	e.service.scan(t.Context())
	e.push.EXPECT().RequestPosition(gomock.Any(), trips[0]).Return(nil)

	e.service.Start(t.Context())
	require.NoError(t, e.service.Stop(t.Context()))

	spans := e.spans.Ended()
	require.Len(t, spans, 2)
	scan, request := spans[0], spans[1]
	assert.Equal(t, "positions.scan", scan.Name())
	assert.Equal(t, "positions.request", request.Name())
	assert.NotEqual(t, scan.SpanContext().TraceID(), request.SpanContext().TraceID())
	require.Len(t, request.Links(), 1)
	assert.Equal(t, scan.SpanContext(), request.Links()[0].SpanContext)
	assert.Contains(t, request.Attributes(), attribute.String("trip.id", trips[0].ID.String()))
}

func TestContextCancelStopsScannerAndWorkers(t *testing.T) {
	e := newEnv(t, func(o *Options) { o.Workers = 1 })
	e.expectScan(staleTrips(3)...)
	e.service.scan(t.Context())

	started := make(chan struct{})
	e.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ model.StaleTrip) error {
			close(started)
			<-ctx.Done()

			return ctx.Err()
		})

	ctx, cancel := context.WithCancel(t.Context())
	e.service.Start(ctx)
	<-started
	cancel()

	require.NoError(t, e.service.Stop(t.Context()))
	assert.Equal(t, 2, e.service.queue.len())
}

func TestStopTimeoutCancelsInflightRequests(t *testing.T) {
	e := newEnv(t, nil)
	e.expectScan(staleTrips(1)...)
	e.service.scan(t.Context())

	started := make(chan struct{})
	e.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ model.StaleTrip) error {
			close(started)
			<-ctx.Done()

			return ctx.Err()
		})

	e.service.Start(t.Context())
	<-started

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorIs(t, e.service.Stop(ctx), context.Canceled)
}
