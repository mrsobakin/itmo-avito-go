//go:build integration

package trip_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres/pgtest"
	"github.com/mrsobakin/itmo-avito-go/internal/infra/postgres/transaction"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
	triprepo "github.com/mrsobakin/itmo-avito-go/internal/repository/trip"
)

const historyCount = "SELECT count(*) FROM trip_status_history WHERE trip_id = $1"

var errAbort = errors.New("abort")

func setup(t *testing.T) (*pgxpool.Pool, *triprepo.Repository, *transaction.Manager) {
	t.Helper()

	pool := pgtest.Pool(t)

	return pool, triprepo.New(postgres.NewDB(pool, pgtest.Timeout)), transaction.NewManager(pool, pgtest.Timeout)
}

func newTrip(t *testing.T, pool *pgxpool.Pool) trip.Trip {
	t.Helper()

	value, err := trip.New(uuid.New(), trip.Params{
		UserID:     uuid.New(),
		DriverID:   uuid.New(),
		StartPoint: trip.Coordinates{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   trip.Coordinates{Latitude: 59.93, Longitude: 30.33},
		Price:      1500,
	}, time.Now().UTC().Truncate(time.Microsecond))
	require.NoError(t, err)
	pgtest.DeleteTrips(t, pool, value.ID)

	return value
}

func TestSaveAndGet(t *testing.T) {
	pool, repo, _ := setup(t)
	value := newTrip(t, pool)

	require.NoError(t, repo.Save(t.Context(), value))

	got, err := repo.Get(t.Context(), value.ID)
	require.NoError(t, err)
	assert.Equal(t, value, got)
}

func TestGetNotFound(t *testing.T) {
	_, repo, _ := setup(t)

	_, err := repo.Get(t.Context(), uuid.New())

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestSaveDriverBusy(t *testing.T) {
	pool, repo, _ := setup(t)
	first := newTrip(t, pool)
	second := newTrip(t, pool)
	second.DriverID = first.DriverID

	require.NoError(t, repo.Save(t.Context(), first))
	err := repo.Save(t.Context(), second)

	require.ErrorIs(t, err, trip.ErrDriverBusy)
}

func TestDriverIsFreeAfterFinish(t *testing.T) {
	pool, repo, _ := setup(t)
	first := newTrip(t, pool)
	second := newTrip(t, pool)
	second.DriverID = first.DriverID

	require.NoError(t, repo.Save(t.Context(), first))
	require.NoError(t, first.Finish(time.Now().UTC()))
	require.NoError(t, repo.Update(t.Context(), first))

	require.NoError(t, repo.Save(t.Context(), second))
}

func TestUpdate(t *testing.T) {
	pool, repo, tx := setup(t)
	value := newTrip(t, pool)
	require.NoError(t, repo.Save(t.Context(), value))

	var finished trip.Trip
	err := tx.Do(t.Context(), func(ctx context.Context) error {
		var err error
		finished, err = repo.GetForUpdate(ctx, value.ID)
		require.NoError(t, err)
		require.NoError(t, finished.Finish(time.Now().UTC().Truncate(time.Microsecond)))

		return repo.Update(ctx, finished)
	})
	require.NoError(t, err)

	got, err := repo.Get(t.Context(), value.ID)
	require.NoError(t, err)
	assert.Equal(t, finished, got)
	assert.Equal(t, trip.StatusCompleted, got.Status)
}

func TestGetForUpdateNotFound(t *testing.T) {
	_, repo, tx := setup(t)

	err := tx.Do(t.Context(), func(ctx context.Context) error {
		_, err := repo.GetForUpdate(ctx, uuid.New())
		return err
	})

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestAddStatusHistory(t *testing.T) {
	pool, repo, _ := setup(t)
	value := newTrip(t, pool)
	require.NoError(t, repo.Save(t.Context(), value))

	from := trip.StatusActive
	require.NoError(t, repo.AddStatusHistory(t.Context(), model.StatusChange{TripID: value.ID, To: trip.StatusActive}))
	require.NoError(t, repo.AddStatusHistory(t.Context(), model.StatusChange{
		TripID: value.ID, From: &from, To: trip.StatusCompleted, Reason: "trip finished",
	}))

	assert.Equal(t, 2, pgtest.Count(t, pool, historyCount, value.ID))
}

func TestTransactionRollsBackBothTables(t *testing.T) {
	tests := []struct {
		name   string
		change func(id uuid.UUID) model.StatusChange
		after  error
	}{
		{
			name:   "error after both inserts",
			change: func(id uuid.UUID) model.StatusChange { return model.StatusChange{TripID: id, To: trip.StatusActive} },
			after:  errAbort,
		},
		{
			name:   "second insert fails",
			change: func(id uuid.UUID) model.StatusChange { return model.StatusChange{TripID: id, To: "bogus"} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, repo, tx := setup(t)
			value := newTrip(t, pool)

			err := tx.Do(t.Context(), func(ctx context.Context) error {
				if err := repo.Save(ctx, value); err != nil {
					return err
				}

				if err := repo.AddStatusHistory(ctx, tt.change(value.ID)); err != nil {
					return err
				}

				return tt.after
			})
			require.Error(t, err)

			_, err = repo.Get(t.Context(), value.ID)
			require.ErrorIs(t, err, trip.ErrTripNotFound)
			assert.Zero(t, pgtest.Count(t, pool, historyCount, value.ID))
		})
	}
}

func TestTransactionCommitsBothTables(t *testing.T) {
	pool, repo, tx := setup(t)
	value := newTrip(t, pool)

	err := tx.Do(t.Context(), func(ctx context.Context) error {
		if err := repo.Save(ctx, value); err != nil {
			return err
		}

		return repo.AddStatusHistory(ctx, model.StatusChange{TripID: value.ID, To: trip.StatusActive})
	})
	require.NoError(t, err)

	_, err = repo.Get(t.Context(), value.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, pgtest.Count(t, pool, historyCount, value.ID))
}
