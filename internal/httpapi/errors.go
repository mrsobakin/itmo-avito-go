package httpapi

import (
	"net/http"

	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

var errorKindStatuses = map[model.ErrorKind]int{
	model.KindInvalidArgument: http.StatusBadRequest,
	model.KindNotFound:        http.StatusNotFound,
	model.KindConflict:        http.StatusConflict,
}

func (h *Handler) InvalidRequest(w http.ResponseWriter, r *http.Request, err error) {
	h.writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
}

func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	h.writeProblem(w, r, http.StatusNotFound, "not_found", "Not found", "Route was not found")
}

func (h *Handler) MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	h.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "Method is not allowed for this route")
}

func (h *Handler) Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}

				h.logger.Error("panic while handling request", "error", recovered)
				h.writeInternalError(w, r)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	if domainErr, ok := model.AsError(err); ok {
		if status, ok := errorKindStatuses[domainErr.Kind]; ok {
			h.writeProblem(w, r, status, domainErr.Code, domainErr.Title, domainErr.Message)
			return
		}
	}

	h.logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	h.writeInternalError(w, r)
}

func (h *Handler) writeInternalError(w http.ResponseWriter, r *http.Request) {
	h.writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal Server Error", "Internal server error")
}
