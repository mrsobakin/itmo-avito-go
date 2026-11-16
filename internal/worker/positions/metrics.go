package positions

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	checkQueued    = "queued"
	checkDuplicate = "duplicate"
	checkDropped   = "dropped"

	requestSuccess   = "success"
	requestError     = "error"
	requestCancelled = "cancelled"
)

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type metrics struct {
	staleChecks metric.Int64Counter
	requests    metric.Int64Counter
	duration    metric.Float64Histogram
	rowsRead    metric.Int64Histogram
}

func newMetrics(meter metric.Meter, queueSize, activeWorkers func() int64) (*metrics, error) {
	staleChecks, checksErr := meter.Int64Counter("tripgo.stale_checks",
		metric.WithUnit("{trip}"), metric.WithDescription("Outcome of enqueuing a stale trip."))
	requests, requestsErr := meter.Int64Counter("tripgo.position_requests",
		metric.WithUnit("{request}"), metric.WithDescription("Outcome of position requests."))
	duration, durationErr := meter.Float64Histogram("tripgo.position_requests.duration",
		metric.WithUnit("s"), metric.WithDescription("Duration of Push Service calls."),
		metric.WithExplicitBucketBoundaries(durationBuckets...))
	rowsRead, rowsErr := meter.Int64Histogram("tripgo.stale_checks.rows_read",
		metric.WithUnit("{trip}"), metric.WithDescription("Stale trips read from the database per scan."))
	size, sizeErr := meter.Int64ObservableGauge("tripgo.position_requests.queue.size",
		metric.WithUnit("{request}"), metric.WithDescription("Position requests waiting in the queue."))
	active, activeErr := meter.Int64ObservableGauge("tripgo.position_requests.workers.active",
		metric.WithUnit("{worker}"), metric.WithDescription("Workers busy with a position request."))

	if err := errors.Join(checksErr, requestsErr, durationErr, rowsErr, sizeErr, activeErr); err != nil {
		return nil, fmt.Errorf("create position metrics: %w", err)
	}

	_, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(size, queueSize())
		o.ObserveInt64(active, activeWorkers())

		return nil
	}, size, active)
	if err != nil {
		return nil, fmt.Errorf("register position gauges: %w", err)
	}

	return &metrics{
		staleChecks: staleChecks,
		requests:    requests,
		duration:    duration,
		rowsRead:    rowsRead,
	}, nil
}

func resultAttr(result string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("result", result))
}
