//go:build integration

package idempotency_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres/pgtest"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres/transaction"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
	"github.com/mrsobakin/itmo-avito-go/internal/repository/idempotency"
	triprepo "github.com/mrsobakin/itmo-avito-go/internal/repository/trip"
)

func TestSaveAndFind(t *testing.T) {
	pool := pgtest.Pool(t)
	db := postgres.NewDB(pool, pgtest.Timeout)
	trips := triprepo.New(db)
	keys := idempotency.New(db)
	tx := transaction.NewManager(pool, pgtest.Timeout)

	now := time.Now().UTC().Truncate(time.Microsecond)
	value, err := trip.New(uuid.New(), trip.Params{UserID: uuid.New(), DriverID: uuid.New()}, now)
	require.NoError(t, err)
	pgtest.DeleteTrips(t, pool, value.ID)
	require.NoError(t, trips.Save(t.Context(), value))

	key := uuid.New()
	record := model.IdempotencyRecord{RequestHash: "hash", TripID: value.ID}

	err = tx.Do(t.Context(), func(ctx context.Context) error {
		require.NoError(t, keys.Lock(ctx, key))

		_, found, err := keys.Find(ctx, key, now.Add(-time.Hour))
		require.NoError(t, err)
		assert.False(t, found)

		return keys.Save(ctx, key, record, now)
	})
	require.NoError(t, err)

	got, found, err := keys.Find(t.Context(), key, now.Add(-time.Hour))
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, record, got)

	_, found, err = keys.Find(t.Context(), key, now)
	require.NoError(t, err)
	assert.False(t, found, "expired key must not be found")

	updated := model.IdempotencyRecord{RequestHash: "other", TripID: value.ID}
	require.NoError(t, keys.Save(t.Context(), key, updated, now.Add(time.Minute)))

	got, found, err = keys.Find(t.Context(), key, now)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, updated, got)
}
