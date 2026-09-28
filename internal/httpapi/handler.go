package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/trip"
)

type Pinger interface {
	Ping(context.Context) error
}

type Handler struct {
	service      *trip.Service
	database     Pinger
	queryTimeout time.Duration
	logger       *slog.Logger
}

func NewHandler(
	service *trip.Service,
	database Pinger,
	queryTimeout time.Duration,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		service:      service,
		database:     database,
		queryTimeout: queryTimeout,
		logger:       logger,
	}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var request api.TripData
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", "Invalid JSON body: "+err.Error())
		return
	}
	if err := validateTripData(request); err != nil {
		h.writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
		return
	}

	value, created, err := h.service.Create(r.Context(), trip.CreateParams{
		UserID:   request.UserId,
		DriverID: request.DriverId,
		StartPoint: trip.Coordinates{
			Latitude:  request.StartPoint.Latitude,
			Longitude: request.StartPoint.Longitude,
		},
		EndPoint: trip.Coordinates{
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

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	value, err := h.service.Get(r.Context(), tripID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, toAPITrip(value))
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	value, err := h.service.Finish(r.Context(), tripID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, toAPITrip(value))
}

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
				h.writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal Server Error", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, trip.ErrNotFound):
		h.writeProblem(w, r, http.StatusNotFound, "trip_not_found", "Trip not found", "Trip was not found")
	case errors.Is(err, trip.ErrCompleted):
		h.writeProblem(w, r, http.StatusConflict, "trip_completed", "Trip completed", "Trip is already completed")
	case errors.Is(err, trip.ErrDriverBusy):
		h.writeProblem(w, r, http.StatusConflict, "driver_busy", "Driver busy", "Driver already has an active trip")
	case errors.Is(err, trip.ErrIdempotencyConflict):
		h.writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", "Idempotency-Key was already used with a different request body")
	default:
		h.logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		h.writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal Server Error", "Internal server error")
	}
}

func (h *Handler) writeProblem(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code string,
	title string,
	detail string,
) {
	instance := r.URL.Path
	problemType := "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-")
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(api.Problem{
		Type:     problemType,
		Title:    title,
		Status:   int32(status),
		Detail:   &detail,
		Instance: &instance,
		Code:     code,
	}); err != nil {
		h.logger.Error("write problem response", "error", err)
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.Error("write JSON response", "error", err)
	}
}

func validateTripData(value api.TripData) error {
	if value.UserId == uuid.Nil {
		return errors.New("user_id must be a non-empty UUID")
	}
	if value.DriverId == uuid.Nil {
		return errors.New("driver_id must be a non-empty UUID")
	}
	return nil
}

func toAPITrip(value trip.Trip) api.Trip {
	return api.Trip{
		Id:       value.ID,
		UserId:   value.UserID,
		DriverId: value.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  value.StartPoint.Latitude,
			Longitude: value.StartPoint.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  value.EndPoint.Latitude,
			Longitude: value.EndPoint.Longitude,
		},
		Price:          value.Price,
		Status:         api.TripStatus(value.Status),
		StartedAt:      value.StartedAt,
		FinishedAt:     value.FinishedAt,
		LastPositionAt: value.LastPositionAt,
	}
}

var _ api.ServerInterface = (*Handler)(nil)
