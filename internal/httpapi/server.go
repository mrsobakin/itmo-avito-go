package httpapi

import (
	"log/slog"
	"time"

	"github.com/mrsobakin/itmo-avito-go/api"
)

type Handler struct {
	service      tripService
	database     pinger
	queryTimeout time.Duration
	logger       *slog.Logger
}

func NewHandler(
	service tripService,
	database pinger,
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

var _ api.ServerInterface = (*Handler)(nil)
