package httpapi

import (
	"net/http"

	"github.com/mrsobakin/itmo-avito-go/api"
)

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	value, err := h.service.Finish(r.Context(), tripID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, toAPITrip(value))
}
