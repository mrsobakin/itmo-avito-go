package save

import (
	"net/http"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/http/request"
	"github.com/mrsobakin/itmo-avito-go/internal/http/response"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type Handler struct {
	usecase usecase
}

func New(usecase usecase) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	var body api.PositionData
	if err := request.DecodeJSON(r.Body, &body); err != nil {
		response.InvalidRequest(w, r, "Invalid JSON body: "+err.Error())
		return
	}

	value, err := h.usecase.Save(r.Context(), model.SavePositionInput{
		TripID: tripID,
		Coordinates: trip.Coordinates{
			Latitude:  body.Coordinates.Latitude,
			Longitude: body.Coordinates.Longitude,
		},
		RecordedAt: body.RecordedAt,
	})
	if err != nil {
		response.Error(w, r, err)
		return
	}

	response.JSON(w, http.StatusCreated, response.Position(value))
}
