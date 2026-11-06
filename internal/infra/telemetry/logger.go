package telemetry

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

func NewLogger(w io.Writer, level slog.Level, service string) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})

	return slog.New(traceHandler{handler}).With("service", service)
}

type traceHandler struct {
	slog.Handler
}

func (h traceHandler) Handle(ctx context.Context, record slog.Record) error {
	if spanContext := trace.SpanContextFromContext(ctx); spanContext.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
		)
	}

	return h.Handler.Handle(ctx, record)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}
