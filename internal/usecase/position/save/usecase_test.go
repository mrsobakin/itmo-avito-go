package save_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
	"github.com/mrsobakin/itmo-avito-go/internal/usecase/position/save"
)

var (
	now   = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	errDB = errors.New("connection reset")
)

type deps struct {
	trips     *MocktripRepository
	positions *MockpositionRepository
	saved     *Mockcounter
	requests  *MockpositionRequests
	usecase   *save.Usecase
}

func newDeps(t *testing.T) deps {
	ctrl := gomock.NewController(t)
	tx := NewMocktxManager(ctrl)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })

	d := deps{
		trips:     NewMocktripRepository(ctrl),
		positions: NewMockpositionRepository(ctrl),
		saved:     NewMockcounter(ctrl),
		requests:  NewMockpositionRequests(ctrl),
	}
	d.usecase = save.New(d.trips, d.positions, tx, d.saved, d.requests, func() time.Time { return now })

	return d
}

func input() model.SavePositionInput {
	return model.SavePositionInput{
		TripID:      uuid.New(),
		Coordinates: trip.Coordinates{Latitude: 59.93, Longitude: 30.33},
		RecordedAt:  now.Add(-time.Second),
	}
}

func TestSave(t *testing.T) {
	d := newDeps(t)
	in := input()
	position := trip.Position{TripID: in.TripID, Coordinates: in.Coordinates, RecordedAt: in.RecordedAt}
	saved := position
	saved.ID = 42

	gomock.InOrder(
		d.trips.EXPECT().GetForUpdate(gomock.Any(), in.TripID).Return(trip.Trip{ID: in.TripID, Status: trip.StatusActive}, nil),
		d.positions.EXPECT().Save(gomock.Any(), position).Return(saved, nil),
		d.trips.EXPECT().UpdateLastPosition(gomock.Any(), in.TripID, in.RecordedAt).Return(nil),
		d.saved.EXPECT().Inc(gomock.Any()),
		d.requests.EXPECT().Cancel(gomock.Any(), in.TripID),
	)

	got, err := d.usecase.Save(context.Background(), in)

	require.NoError(t, err)
	assert.Equal(t, saved, got)
}

func TestSaveRejectsInvalidPosition(t *testing.T) {
	d := newDeps(t)
	in := input()
	in.RecordedAt = now.Add(time.Hour)

	_, err := d.usecase.Save(context.Background(), in)

	_, ok := errors.AsType[*trip.ValidationError](err)
	assert.True(t, ok)
}

func TestSaveErrors(t *testing.T) {
	tests := []struct {
		name    string
		expect  func(d deps, id uuid.UUID)
		wantErr error
	}{
		{name: "not found", wantErr: trip.ErrTripNotFound, expect: func(d deps, id uuid.UUID) {
			d.trips.EXPECT().GetForUpdate(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)
		}},
		{name: "completed", wantErr: trip.ErrTripCompleted, expect: func(d deps, id uuid.UUID) {
			d.trips.EXPECT().GetForUpdate(gomock.Any(), id).Return(trip.Trip{ID: id, Status: trip.StatusCompleted}, nil)
		}},
		{name: "insert", wantErr: errDB, expect: func(d deps, id uuid.UUID) {
			d.trips.EXPECT().GetForUpdate(gomock.Any(), id).Return(trip.Trip{ID: id, Status: trip.StatusActive}, nil)
			d.positions.EXPECT().Save(gomock.Any(), gomock.Any()).Return(trip.Position{}, errDB)
		}},
		{name: "update", wantErr: errDB, expect: func(d deps, id uuid.UUID) {
			d.trips.EXPECT().GetForUpdate(gomock.Any(), id).Return(trip.Trip{ID: id, Status: trip.StatusActive}, nil)
			d.positions.EXPECT().Save(gomock.Any(), gomock.Any()).Return(trip.Position{ID: 1}, nil)
			d.trips.EXPECT().UpdateLastPosition(gomock.Any(), id, gomock.Any()).Return(errDB)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDeps(t)
			in := input()
			tt.expect(d, in.TripID)

			_, err := d.usecase.Save(context.Background(), in)

			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
