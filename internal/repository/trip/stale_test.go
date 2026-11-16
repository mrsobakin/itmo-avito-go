//go:build integration

package trip_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
	triprepo "github.com/mrsobakin/itmo-avito-go/internal/repository/trip"
)

const staleAfter = time.Minute

func saveTrip(t *testing.T, repo *triprepo.Repository, value trip.Trip) trip.Trip {
	t.Helper()
	require.NoError(t, repo.Save(t.Context(), value))

	return value
}

func staleIDs(t *testing.T, repo *triprepo.Repository) []uuid.UUID {
	t.Helper()

	var (
		ids   []uuid.UUID
		after *model.StaleTrip
	)

	for {
		batch, err := repo.ListStale(t.Context(), staleAfter, after, 1000)
		require.NoError(t, err)

		for _, value := range batch {
			ids = append(ids, value.ID)
		}

		if len(batch) < 1000 {
			return ids
		}

		after = &batch[len(batch)-1]
	}
}

func TestListStale(t *testing.T) {
	pool, repo, _ := setup(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	noPositions := newTrip(t, pool)
	noPositions.StartedAt = now.Add(-time.Hour)
	saveTrip(t, repo, noPositions)

	fresh := newTrip(t, pool)
	saveTrip(t, repo, fresh)

	freshPosition := newTrip(t, pool)
	freshPosition.StartedAt = now.Add(-time.Hour)
	saveTrip(t, repo, freshPosition)
	require.NoError(t, repo.UpdateLastPosition(t.Context(), freshPosition.ID, now))

	stalePosition := newTrip(t, pool)
	stalePosition.StartedAt = now.Add(-time.Hour)
	saveTrip(t, repo, stalePosition)
	require.NoError(t, repo.UpdateLastPosition(t.Context(), stalePosition.ID, now.Add(-10*time.Minute)))

	positionBeforeStart := newTrip(t, pool)
	saveTrip(t, repo, positionBeforeStart)
	require.NoError(t, repo.UpdateLastPosition(t.Context(), positionBeforeStart.ID, now.Add(-10*time.Minute)))

	completed := newTrip(t, pool)
	completed.StartedAt = now.Add(-time.Hour)
	require.NoError(t, completed.Finish(now))
	saveTrip(t, repo, completed)

	ids := staleIDs(t, repo)

	assert.Contains(t, ids, noPositions.ID)
	assert.Contains(t, ids, stalePosition.ID)
	assert.NotContains(t, ids, fresh.ID)
	assert.NotContains(t, ids, freshPosition.ID)
	assert.NotContains(t, ids, positionBeforeStart.ID)
	assert.NotContains(t, ids, completed.ID)
	assert.Less(t, slices.Index(ids, noPositions.ID), slices.Index(ids, stalePosition.ID))
}

func TestListStalePagesThroughTies(t *testing.T) {
	pool, repo, _ := setup(t)
	silentSince := time.Now().AddDate(-30, 0, 0).UTC().Truncate(time.Microsecond)

	want := make([]uuid.UUID, 5)
	for i := range want {
		value := newTrip(t, pool)
		value.StartedAt = silentSince
		want[i] = saveTrip(t, repo, value).ID
	}

	var got []uuid.UUID

	after := &model.StaleTrip{SilentSince: silentSince, ID: uuid.Nil}
	for len(got) < len(want) {
		batch, err := repo.ListStale(t.Context(), staleAfter, after, 2)
		require.NoError(t, err)
		require.NotEmpty(t, batch)

		for _, value := range batch {
			if value.SilentSince.Equal(silentSince) {
				got = append(got, value.ID)
			}
		}

		after = &batch[len(batch)-1]
	}

	assert.ElementsMatch(t, want, got)
	assert.True(t, slices.IsSortedFunc(got, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) }))
}

func TestUpdateLastPositionIgnoresOlderPositions(t *testing.T) {
	pool, repo, _ := setup(t)
	value := saveTrip(t, repo, newTrip(t, pool))
	latest := value.StartedAt.Add(time.Minute)

	require.NoError(t, repo.UpdateLastPosition(t.Context(), value.ID, latest))
	require.NoError(t, repo.UpdateLastPosition(t.Context(), value.ID, latest.Add(-30*time.Second)))

	got, err := repo.Get(t.Context(), value.ID)
	require.NoError(t, err)
	assert.Equal(t, &latest, got.LastPositionAt)
}
