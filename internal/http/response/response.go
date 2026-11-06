package response

import (
	"encoding/json"
	"net/http"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/http/domainerror"
	"github.com/mrsobakin/itmo-avito-go/internal/http/middleware"
)

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func Problem(w http.ResponseWriter, r *http.Request, status int, body api.Problem) {
	body.Instance = &r.URL.Path

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func Error(w http.ResponseWriter, r *http.Request, err error) {
	status, body := domainerror.Problem(err)
	if status >= http.StatusInternalServerError {
		middleware.ReportError(r.Context(), err)
	}

	Problem(w, r, status, body)
}

func InvalidRequest(w http.ResponseWriter, r *http.Request, detail string) {
	status, body := domainerror.InvalidRequest(detail)
	Problem(w, r, status, body)
}
