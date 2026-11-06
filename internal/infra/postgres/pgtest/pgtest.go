//go:build integration

package pgtest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/mrsobakin/itmo-avito-go/internal/config"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
)

const Timeout = 3 * time.Second

func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}

	pool, err := postgres.Open(t.Context(), config.Database{
		URL:             url,
		MaxConns:        4,
		MaxConnLifetime: time.Minute,
		ConnectTimeout:  Timeout,
	}, tracenoop.NewTracerProvider(), metricnoop.NewMeterProvider())
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}

func DeleteTrips(t *testing.T, pool *pgxpool.Pool, ids ...uuid.UUID) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()

		_, err := pool.Exec(ctx, "DELETE FROM trips WHERE id = ANY($1)", ids)
		require.NoError(t, err)
	})
}

func Count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()

	var count int
	require.NoError(t, pool.QueryRow(t.Context(), query, args...).Scan(&count))

	return count
}
