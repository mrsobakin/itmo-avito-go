package create

import (
	"net/http"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/http/response"
)

type Handler struct {
	usecase usecase
}

func New(usecase usecase) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var body api.TripData
	if err := decodeJSON(r.Body, &body); err != nil {
		response.InvalidRequest(w, r, "Invalid JSON body: "+err.Error())
		return
	}

	result, err := h.usecase.Create(r.Context(), toInput(body, params))
	if err != nil {
		response.Error(w, r, err)
		return
	}

	if !result.Created {
		response.JSON(w, http.StatusOK, response.Trip(result.Trip))
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+result.Trip.ID.String())
	response.JSON(w, http.StatusCreated, response.Trip(result.Trip))
}
