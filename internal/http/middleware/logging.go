package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

type errorSlotKey struct{}

type errorSlot struct {
	err error
}

func ReportError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetAttributes(semconv.ErrorTypeKey.String(errorType(err)))

	if slot, ok := ctx.Value(errorSlotKey{}).(*errorSlot); ok {
		slot.err = err
	}
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			slot := &errorSlot{}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r.WithContext(context.WithValue(r.Context(), errorSlotKey{}, slot)))

			status := statusOf(ww)

			level := slog.LevelInfo
			if status >= http.StatusInternalServerError {
				level = slog.LevelError
			}

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("route", r.Pattern),
				slog.Int("status", status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			}

			if slot.err != nil {
				attrs = append(attrs, slog.String("error", slot.err.Error()))
			}

			logger.LogAttrs(r.Context(), level, "request handled", attrs...)
		})
	}
}

func statusOf(w middleware.WrapResponseWriter) int {
	if w.Status() == 0 {
		return http.StatusOK
	}

	return w.Status()
}

func errorType(err error) string {
	for {
		inner := errors.Unwrap(err)
		if inner == nil {
			return fmt.Sprintf("%T", err)
		}

		err = inner
	}
}
