package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrsobakin/itmo-avito-go/internal/config"
	"github.com/mrsobakin/itmo-avito-go/internal/http/health"
	"github.com/mrsobakin/itmo-avito-go/internal/http/router"
	createhttp "github.com/mrsobakin/itmo-avito-go/internal/http/trip/create"
	finishhttp "github.com/mrsobakin/itmo-avito-go/internal/http/trip/finish"
	gethttp "github.com/mrsobakin/itmo-avito-go/internal/http/trip/get"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres/transaction"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/telemetry"
	idempotencyrepo "github.com/mrsobakin/itmo-avito-go/internal/repository/idempotency"
	triprepo "github.com/mrsobakin/itmo-avito-go/internal/repository/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/usecase/trip/create"
	"github.com/mrsobakin/itmo-avito-go/internal/usecase/trip/finish"
	"github.com/mrsobakin/itmo-avito-go/internal/usecase/trip/get"
)

const meterName = "github.com/mrsobakin/itmo-avito-go"

type Container struct {
	Logger    *slog.Logger
	Server    *http.Server
	pool      *pgxpool.Pool
	telemetry *telemetry.Telemetry
}

func New(ctx context.Context, cfg config.Config) (_ *Container, err error) {
	c := &Container{Logger: telemetry.NewLogger(os.Stdout, cfg.LogLevel, cfg.ServiceName)}
	defer func() {
		if err != nil {
			err = errors.Join(err, c.Close(ctx))
		}
	}()

	c.telemetry, err = telemetry.New(ctx, c.Logger)
	if err != nil {
		return nil, err
	}

	meter := c.telemetry.MeterProvider.Meter(meterName)

	c.pool, err = postgres.Open(ctx, cfg.Database, c.telemetry.TracerProvider, c.telemetry.MeterProvider)
	if err != nil {
		return nil, err
	}

	tripsCreated, err := telemetry.NewCounter(meter, "tripgo.trips.created", "Number of created trips.")
	if err != nil {
		return nil, err
	}

	tripsCompleted, err := telemetry.NewCounter(meter, "tripgo.trips.completed", "Number of completed trips.")
	if err != nil {
		return nil, err
	}

	db := postgres.NewDB(c.pool, cfg.Database.QueryTimeout)
	txManager := transaction.NewManager(c.pool, cfg.Database.QueryTimeout)
	trips := triprepo.New(db)
	keys := idempotencyrepo.New(db)

	handler, err := router.New(router.Handlers{
		Create: createhttp.New(create.New(trips, keys, txManager, tripsCreated, c.Logger, cfg.IdempotencyKeyTTL, now)),
		Get:    gethttp.New(get.New(trips)),
		Finish: finishhttp.New(finish.New(trips, txManager, tripsCompleted, c.Logger, now)),
		Health: health.New(c.pool, cfg.Database.QueryTimeout),
	}, router.Telemetry{
		Logger:         c.Logger,
		TracerProvider: c.telemetry.TracerProvider,
		Meter:          meter,
	})
	if err != nil {
		return nil, err
	}

	c.Server = &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(c.Logger.Handler(), slog.LevelWarn),
	}

	return c, nil
}

func (c *Container) Close(ctx context.Context) error {
	if c.pool != nil {
		c.pool.Close()
	}

	if c.telemetry != nil {
		if err := c.telemetry.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown telemetry: %w", err)
		}
	}

	return nil
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}
