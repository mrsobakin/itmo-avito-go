package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/config"
	"github.com/mrsobakin/itmo-avito-go/internal/database"
	"github.com/mrsobakin/itmo-avito-go/internal/httpapi"
	"github.com/mrsobakin/itmo-avito-go/internal/transaction"
	"github.com/mrsobakin/itmo-avito-go/internal/trip"
)

const maxRequestBody = 1 << 20

func main() {
	if err := run(); err != nil {
		slog.Error("trip service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	appCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(appCtx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	repository := trip.NewRepository(pool, cfg.Database.QueryTimeout)
	txManager := transaction.NewManager(pool, cfg.Database.QueryTimeout)
	service := trip.NewService(repository, txManager, cfg.IdempotencyKeyTTL)
	handler := httpapi.NewHandler(service, pool, cfg.Database.QueryTimeout, logger)

	spec, err := api.GetSpec()
	if err != nil {
		return fmt.Errorf("load OpenAPI spec: %w", err)
	}
	openapi3.SchemaErrorDetailsDisabled = true
	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		DoNotValidateServers: true,
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttpmiddleware.ErrorHandlerOpts) {
			handler.InvalidRequest(w, r, err)
		},
	})

	router := chi.NewRouter()
	router.Use(handler.Recover)
	router.Use(func(next http.Handler) http.Handler {
		return http.MaxBytesHandler(next, maxRequestBody)
	})
	router.NotFound(handler.NotFound)
	router.MethodNotAllowed(handler.MethodNotAllowed)
	api.HandlerWithOptions(handler, api.ChiServerOptions{
		BaseRouter:       router,
		Middlewares:      []api.MiddlewareFunc{validator},
		ErrorHandlerFunc: handler.InvalidRequest,
	})

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           router,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server started", "address", cfg.HTTP.Addr)
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErrors <- err
	}()

	select {
	case err := <-serverErrors:
		return err
	case <-appCtx.Done():
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown timed out", "error", err)
		if closeErr := server.Close(); closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}
	return <-serverErrors
}
