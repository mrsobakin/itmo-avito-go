package domainerror

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
)

const typeBaseURL = "https://tripgo.example/problems/"

type mapping struct {
	err    error
	status int32
	code   string
	title  string
	detail string
}

var mappings = []mapping{
	{trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found", "Trip not found", "Trip was not found"},
	{trip.ErrTripCompleted, http.StatusConflict, "trip_completed", "Trip completed", "Trip is already completed"},
	{trip.ErrDriverBusy, http.StatusConflict, "driver_busy", "Driver busy", "Driver already has an active trip"},
	{
		trip.ErrIdempotencyConflict, http.StatusConflict, "idempotency_conflict", "Idempotency conflict",
		"Idempotency-Key was already used with a different request body",
	},
}

func Problem(err error) (status int, body api.Problem) {
	if validationErr, ok := errors.AsType[*trip.ValidationError](err); ok {
		return InvalidRequest(validationErr.Error())
	}

	for _, m := range mappings {
		if errors.Is(err, m.err) {
			return problem(m.status, m.code, m.title, m.detail)
		}
	}

	return Internal()
}

func Internal() (status int, body api.Problem) {
	return problem(http.StatusInternalServerError, "internal_error", "Internal Server Error", "Internal server error")
}

func InvalidRequest(detail string) (status int, body api.Problem) {
	return problem(http.StatusBadRequest, "invalid_request", "Invalid request", detail)
}

func RouteNotFound() (status int, body api.Problem) {
	return problem(http.StatusNotFound, "not_found", "Not found", "Route was not found")
}

func MethodNotAllowed() (status int, body api.Problem) {
	return problem(http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "Method is not allowed for this route")
}

func problem(status int32, code, title, detail string) (int, api.Problem) {
	return int(status), api.Problem{
		Type:   typeBaseURL + strings.ReplaceAll(code, "_", "-"),
		Title:  title,
		Status: status,
		Detail: &detail,
		Code:   code,
	}
}
