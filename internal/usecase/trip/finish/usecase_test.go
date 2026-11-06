package finish_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
	"github.com/mrsobakin/itmo-avito-go/internal/usecase/trip/finish"
)

var (
	now   = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	errDB = errors.New("connection reset")
)

type deps struct {
	trips     *MocktripRepository
	completed *Mockcounter
	usecase   *finish.Usecase
}

func newDeps(t *testing.T) deps {
	ctrl := gomock.NewController(t)
	tx := NewMocktxManager(ctrl)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })

	d := deps{trips: NewMocktripRepository(ctrl), completed: NewMockcounter(ctrl)}
	d.usecase = finish.New(d.trips, tx, d.completed, slog.New(slog.DiscardHandler), func() time.Time { return now })

	return d
}

func activeTrip() trip.Trip {
	return trip.Trip{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		DriverID:  uuid.New(),
		Price:     1500,
		Status:    trip.StatusActive,
		StartedAt: now.Add(-time.Hour),
	}
}

func TestFinish(t *testing.T) {
	d := newDeps(t)
	active := activeTrip()

	want := active
	want.Status = trip.StatusCompleted
	want.FinishedAt = &now

	from := trip.StatusActive
	d.trips.EXPECT().GetForUpdate(gomock.Any(), active.ID).Return(active, nil)
	d.trips.EXPECT().Update(gomock.Any(), want).Return(nil)
	d.trips.EXPECT().AddStatusHistory(gomock.Any(), model.StatusChange{
		TripID: active.ID,
		From:   &from,
		To:     trip.StatusCompleted,
		Reason: "trip finished",
	}).Return(nil)
	d.completed.EXPECT().Inc(gomock.Any())

	got, err := d.usecase.Finish(context.Background(), active.ID)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestFinishCompletedTrip(t *testing.T) {
	d := newDeps(t)
	completed := activeTrip()
	completed.Status = trip.StatusCompleted
	d.trips.EXPECT().GetForUpdate(gomock.Any(), completed.ID).Return(completed, nil)

	_, err := d.usecase.Finish(context.Background(), completed.ID)

	require.ErrorIs(t, err, trip.ErrTripCompleted)
}

func TestFinishNotFound(t *testing.T) {
	d := newDeps(t)
	id := uuid.New()
	d.trips.EXPECT().GetForUpdate(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)

	_, err := d.usecase.Finish(context.Background(), id)

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestFinishStorageErrors(t *testing.T) {
	tests := []struct {
		name   string
		expect func(d deps, value trip.Trip)
	}{
		{name: "update", expect: func(d deps, value trip.Trip) {
			d.trips.EXPECT().Update(gomock.Any(), gomock.Any()).Return(errDB)
		}},
		{name: "history", expect: func(d deps, value trip.Trip) {
			d.trips.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
			d.trips.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).Return(errDB)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDeps(t)
			value := activeTrip()
			d.trips.EXPECT().GetForUpdate(gomock.Any(), value.ID).Return(value, nil)
			tt.expect(d, value)

			_, err := d.usecase.Finish(context.Background(), value.ID)

			require.ErrorIs(t, err, errDB)
		})
	}
}
