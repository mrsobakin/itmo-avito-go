package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
}

func New(ctx context.Context, logger *slog.Logger) (*Telemetry, error) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn("opentelemetry export failed", "error", err)
	}))

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(semconv.ServiceVersion(buildVersion())))
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("create span exporter: %w", err)
	}

	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create metric reader: %w", err), spanExporter.Shutdown(ctx))
	}

	return &Telemetry{
		TracerProvider: sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(spanExporter),
			sdktrace.WithResource(res),
		),
		MeterProvider: sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(metricReader),
			sdkmetric.WithResource(res),
		),
	}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	return errors.Join(t.TracerProvider.Shutdown(ctx), t.MeterProvider.Shutdown(ctx))
}

type Counter struct {
	counter metric.Int64Counter
}

func NewCounter(meter metric.Meter, name, description string) (*Counter, error) {
	counter, err := meter.Int64Counter(name, metric.WithUnit("{trip}"), metric.WithDescription(description))
	if err != nil {
		return nil, fmt.Errorf("create counter %s: %w", name, err)
	}

	return &Counter{counter: counter}, nil
}

func (c *Counter) Inc(ctx context.Context) {
	c.counter.Add(ctx, 1)
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}

	return info.Main.Version
}
