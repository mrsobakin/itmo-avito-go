package positions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueuePopsStalestFirst(t *testing.T) {
	q := newQueue(3)
	trips := staleTrips(3)

	for _, i := range []int{1, 2, 0} {
		queued, dropped := q.push(&task{trip: trips[i]})
		require.True(t, queued)
		require.Nil(t, dropped)
	}

	q.close()

	for _, want := range trips {
		got, ok := q.pop(t.Context())
		require.True(t, ok)
		assert.Equal(t, want, got.trip)
	}

	_, ok := q.pop(t.Context())
	assert.False(t, ok)
}

func TestQueueDropsFresherTripWhenFull(t *testing.T) {
	q := newQueue(2)
	trips := staleTrips(3)
	q.push(&task{trip: trips[0]})
	q.push(&task{trip: trips[1]})

	fresh := &task{trip: trips[2]}
	queued, dropped := q.push(fresh)

	assert.False(t, queued)
	assert.Same(t, fresh, dropped)
	assert.Equal(t, 2, q.len())
}

func TestQueueKeepsInflightTripTracked(t *testing.T) {
	q := newQueue(2)
	value := staleTrips(1)[0]
	q.push(&task{trip: value})

	inflight, ok := q.pop(t.Context())
	require.True(t, ok)

	assert.False(t, q.cancel(value.ID))
	queued, dropped := q.push(&task{trip: value})
	assert.False(t, queued)
	assert.Nil(t, dropped)

	q.done(inflight)
	queued, _ = q.push(&task{trip: value})
	assert.True(t, queued)
}

func TestQueueEvictsFreshestTrip(t *testing.T) {
	q := newQueue(6)
	trips := staleTrips(8)

	for _, i := range []int{4, 1, 6, 3, 5, 2} {
		q.push(&task{trip: trips[i]})
	}

	queued, dropped := q.push(&task{trip: trips[0]})
	require.True(t, queued)
	assert.Equal(t, trips[6], dropped.trip)

	queued, dropped = q.push(&task{trip: trips[7]})
	require.False(t, queued)
	assert.Equal(t, trips[7], dropped.trip)

	q.close()

	for _, want := range trips[:6] {
		got, ok := q.pop(t.Context())
		require.True(t, ok)
		assert.Equal(t, want, got.trip)
	}
}
