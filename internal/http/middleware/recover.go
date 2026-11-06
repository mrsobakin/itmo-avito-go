package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
)

func Recover(logger *slog.Logger, onPanic http.HandlerFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}

				if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(recovered)
				}

				logger.ErrorContext(r.Context(), "panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", recovered,
					"stack", string(debug.Stack()),
				)

				onPanic(w, r)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
