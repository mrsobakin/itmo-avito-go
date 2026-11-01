package httpapi

import (
	"context"
	"net/http"

	"github.com/mrsobakin/itmo-avito-go/api"
)

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	h.writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	if err := h.database.Ping(ctx); err != nil {
		h.writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}

	h.writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
