package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func Tracing(tracerProvider trace.TracerProvider, route func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		traced := otelhttp.NewHandler(next, "",
			otelhttp.WithTracerProvider(tracerProvider),
			otelhttp.WithMeterProvider(noop.NewMeterProvider()),
			otelhttp.WithPropagators(propagation.TraceContext{}),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				if r.Pattern == "" {
					return r.Method
				}

				return r.Method + " " + r.Pattern
			}),
		)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Pattern = route(r)
			traced.ServeHTTP(w, r)
		})
	}
}
