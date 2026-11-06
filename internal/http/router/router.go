package router

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/http/domainerror"
	"github.com/mrsobakin/itmo-avito-go/internal/http/health"
	"github.com/mrsobakin/itmo-avito-go/internal/http/middleware"
	"github.com/mrsobakin/itmo-avito-go/internal/http/response"
	"github.com/mrsobakin/itmo-avito-go/internal/http/trip/create"
	"github.com/mrsobakin/itmo-avito-go/internal/http/trip/finish"
	"github.com/mrsobakin/itmo-avito-go/internal/http/trip/get"
)

const maxRequestBody = 1 << 20

type Handlers struct {
	Create *create.Handler
	Get    *get.Handler
	Finish *finish.Handler
	Health *health.Handler
}

type Telemetry struct {
	Logger         *slog.Logger
	TracerProvider trace.TracerProvider
	Meter          metric.Meter
}

func New(handlers Handlers, telemetry Telemetry) (http.Handler, error) {
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI spec: %w", err)
	}

	openapi3.SchemaErrorDetailsDisabled = true
	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		DoNotValidateServers: true,
		ErrorHandlerWithOpts: func(
			_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttpmiddleware.ErrorHandlerOpts,
		) {
			invalidRequest(w, r, err)
		},
	})

	metrics, err := middleware.Metrics(telemetry.Meter)
	if err != nil {
		return nil, err
	}

	router := chi.NewRouter()
	route := func(r *http.Request) string {
		return router.Find(chi.NewRouteContext(), r.Method, r.URL.Path)
	}

	router.Use(
		middleware.Recover(telemetry.Logger, problem(domainerror.Internal)),
		middleware.Tracing(telemetry.TracerProvider, route),
		middleware.Logging(telemetry.Logger),
		metrics,
	)
	router.NotFound(problem(domainerror.RouteNotFound))
	router.MethodNotAllowed(problem(domainerror.MethodNotAllowed))

	api.HandlerWithOptions(server{h: handlers}, api.ChiServerOptions{
		BaseRouter: router,
		Middlewares: []api.MiddlewareFunc{validator, func(next http.Handler) http.Handler {
			return http.MaxBytesHandler(next, maxRequestBody)
		}},
		ErrorHandlerFunc: invalidRequest,
	})

	return router, nil
}

func invalidRequest(w http.ResponseWriter, r *http.Request, err error) {
	response.InvalidRequest(w, r, err.Error())
}

func problem(build func() (int, api.Problem)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, body := build()
		response.Problem(w, r, status, body)
	}
}

type server struct {
	h Handlers
}

var _ api.ServerInterface = server{}

func (s server) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	s.h.Create.Handle(w, r, params)
}

func (s server) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.h.Get.Handle(w, r, tripID)
}

func (s server) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.h.Finish.Handle(w, r, tripID)
}

func (s server) Health(w http.ResponseWriter, r *http.Request) {
	s.h.Health.Health(w, r)
}

func (s server) Ready(w http.ResponseWriter, r *http.Request) {
	s.h.Health.Ready(w, r)
}
