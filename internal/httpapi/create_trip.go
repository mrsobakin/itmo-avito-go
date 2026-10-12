package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/trip/model"
)

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var request api.TripData
	if err := decodeJSON(r.Body, &request); err != nil {
		h.writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", "Invalid JSON body: "+err.Error())
		return
	}

	value, created, err := h.service.Create(r.Context(), model.CreateParams{
		UserID:   request.UserId,
		DriverID: request.DriverId,
		StartPoint: model.Coordinates{
			Latitude:  request.StartPoint.Latitude,
			Longitude: request.StartPoint.Longitude,
		},
		EndPoint: model.Coordinates{
			Latitude:  request.EndPoint.Latitude,
			Longitude: request.EndPoint.Longitude,
		},
		Price: request.Price,
	}, params.IdempotencyKey)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	if !created {
		h.writeJSON(w, http.StatusOK, toAPITrip(value))
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+value.ID.String())
	h.writeJSON(w, http.StatusCreated, toAPITrip(value))
}

var errTrailingData = errors.New("unexpected data after JSON value")

func decodeJSON(body io.Reader, destination any) error {
	decoder := json.NewDecoder(body)

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errTrailingData
	}

	return nil
}
