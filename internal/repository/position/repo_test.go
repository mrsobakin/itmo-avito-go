//go:build integration

package position_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres/pgtest"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres/transaction"
	positionrepo "github.com/mrsobakin/itmo-avito-go/internal/repository/position"
	triprepo "github.com/mrsobakin/itmo-avito-go/internal/repository/trip"
)

const positionCount = "SELECT count(*) FROM trip_positions WHERE trip_id = $1"

var errAbort = errors.New("abort")

type env struct {
	trips     *triprepo.Repository
	positions *positionrepo.Repository
	tx        *transaction.Manager
	value     trip.Trip
}

func setup(t *testing.T) (env, func() int) {
	t.Helper()

	pool := pgtest.Pool(t)
	db := postgres.NewDB(pool, pgtest.Timeout)
	e := env{
		trips:     triprepo.New(db),
		positions: positionrepo.New(db),
		tx:        transaction.NewManager(pool, pgtest.Timeout),
	}

	value, err := trip.New(uuid.New(), trip.Params{
		UserID:     uuid.New(),
		DriverID:   uuid.New(),
		StartPoint: trip.Coordinates{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   trip.Coordinates{Latitude: 59.93, Longitude: 30.33},
		Price:      1500,
	}, time.Now().UTC().Truncate(time.Microsecond))
	require.NoError(t, err)
	pgtest.DeleteTrips(t, pool, value.ID)
	require.NoError(t, e.trips.Save(t.Context(), value))
	e.value = value

	return e, func() int { return pgtest.Count(t, pool, positionCount, value.ID) }
}

func (e env) position(offset time.Duration) trip.Position {
	return trip.Position{
		TripID:      e.value.ID,
		Coordinates: trip.Coordinates{Latitude: 59.93, Longitude: 30.33},
		RecordedAt:  e.value.StartedAt.Add(offset),
	}
}

func (e env) save(ctx context.Context, value trip.Position, after error) error {
	return e.tx.Do(ctx, func(ctx context.Context) error {
		if _, err := e.positions.Save(ctx, value); err != nil {
			return err
		}

		if err := e.trips.UpdateLastPosition(ctx, value.TripID, value.RecordedAt); err != nil {
			return err
		}

		return after
	})
}

func TestListOrdersByRecordedAtThenID(t *testing.T) {
	e, _ := setup(t)

	late, err := e.positions.Save(t.Context(), e.position(2*time.Second))
	require.NoError(t, err)
	first, err := e.positions.Save(t.Context(), e.position(time.Second))
	require.NoError(t, err)
	second, err := e.positions.Save(t.Context(), e.position(time.Second))
	require.NoError(t, err)

	got, err := e.positions.List(t.Context(), e.value.ID)

	require.NoError(t, err)
	assert.Equal(t, []trip.Position{first, second, late}, got)
}

func TestListEmpty(t *testing.T) {
	e, _ := setup(t)

	got, err := e.positions.List(t.Context(), e.value.ID)

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NotNil(t, got)
}

func TestSavePositionUpdatesLastPositionAt(t *testing.T) {
	e, count := setup(t)
	latest := e.position(time.Minute)

	require.NoError(t, e.save(t.Context(), latest, nil))
	require.NoError(t, e.save(t.Context(), e.position(time.Second), nil))

	got, err := e.trips.Get(t.Context(), e.value.ID)
	require.NoError(t, err)
	assert.Equal(t, &latest.RecordedAt, got.LastPositionAt)
	assert.Equal(t, 2, count())
}

func TestSavePositionRollsBackAtomically(t *testing.T) {
	e, count := setup(t)

	require.ErrorIs(t, e.save(t.Context(), e.position(time.Minute), errAbort), errAbort)

	got, err := e.trips.Get(t.Context(), e.value.ID)
	require.NoError(t, err)
	assert.Nil(t, got.LastPositionAt)
	assert.Zero(t, count())
}
