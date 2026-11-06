package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

var dbDurationBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10}

const meterName = "github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"

var collectionPattern = regexp.MustCompile(`(?i)\b(?:from|into|update)\s+([a-z_][a-z0-9_]*)`)

type queryTracer struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
}

type queryStartKey struct{}

type queryStart struct {
	at    time.Time
	attrs []attribute.KeyValue
}

func newQueryTracer(tracerProvider trace.TracerProvider, meterProvider metric.MeterProvider) (*queryTracer, error) {
	duration, err := meterProvider.Meter(meterName).Float64Histogram(
		"db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of database client operations."),
		metric.WithExplicitBucketBoundaries(dbDurationBuckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("create db duration histogram: %w", err)
	}

	return &queryTracer{
		Tracer: otelpgx.NewTracer(
			otelpgx.WithTracerProvider(tracerProvider),
			otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		),
		duration: duration,
	}, nil
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)

	operation := otelpgx.SQLOperationName(ctx, data.SQL)
	attrs := []attribute.KeyValue{semconv.DBSystemNamePostgreSQL, semconv.DBOperationName(operation)}

	if collection := annotateSpan(ctx, data.SQL, operation, ""); collection != "" {
		attrs = append(attrs, semconv.DBCollectionName(collection))
	}

	return context.WithValue(ctx, queryStartKey{}, queryStart{at: time.Now(), attrs: attrs})
}

func (t *queryTracer) TracePrepareStart(ctx context.Context, conn *pgx.Conn, data pgx.TracePrepareStartData) context.Context {
	ctx = t.Tracer.TracePrepareStart(ctx, conn, data)
	annotateSpan(ctx, data.SQL, otelpgx.SQLOperationName(ctx, data.SQL), "prepare ")

	return ctx
}

func annotateSpan(ctx context.Context, sql, operation, prefix string) string {
	match := collectionPattern.FindStringSubmatch(sql)
	if match == nil {
		return ""
	}

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(semconv.DBCollectionName(match[1]))
	span.SetName(prefix + operation + " " + match[1])

	return match[1]
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	start, _ := ctx.Value(queryStartKey{}).(queryStart)
	attrs := start.attrs

	if data.Err != nil {
		errType := errorType(data.Err)
		attrs = append(attrs, errType)
		trace.SpanFromContext(ctx).SetAttributes(errType)
	}

	t.Tracer.TraceQueryEnd(ctx, conn, data)

	if !start.at.IsZero() {
		t.duration.Record(ctx, time.Since(start.at).Seconds(), metric.WithAttributes(attrs...))
	}
}

func errorType(err error) attribute.KeyValue {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return semconv.ErrorTypeKey.String(pgErr.Code)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return semconv.ErrorTypeKey.String("timeout")
	}

	return semconv.ErrorTypeOther
}
