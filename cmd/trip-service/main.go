package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mrsobakin/itmo-avito-go/internal/config"
	"github.com/mrsobakin/itmo-avito-go/internal/container"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("trip service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	appCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := container.New(appCtx, cfg)
	if err != nil {
		return err
	}

	logger := c.Logger

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server started", "address", cfg.HTTP.Addr)
		err := c.Server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}

		serverErrors <- err
	}()

	select {
	case err = <-serverErrors:
	case <-appCtx.Done():
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if shutdownErr := c.Server.Shutdown(shutdownCtx); shutdownErr != nil {
		logger.Error("graceful shutdown timed out, forcing exit", "error", shutdownErr)
		err = errors.Join(err, shutdownErr, c.Server.Close())
	} else if err == nil {
		err = <-serverErrors
	}

	err = errors.Join(err, c.Close(shutdownCtx))
	if err == nil {
		logger.Info("trip service stopped")
	}

	return err
}
