package create_test

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
	"github.com/mrsobakin/itmo-avito-go/internal/usecase/trip/create"
)

var (
	now    = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	keyTTL = 24 * time.Hour
	errDB  = errors.New("connection reset")
)

type deps struct {
	trips   *MocktripRepository
	keys    *MockidempotencyRepository
	tx      *MocktxManager
	created *Mockcounter
	usecase *create.Usecase
}

func newDeps(t *testing.T) deps {
	ctrl := gomock.NewController(t)
	d := deps{
		trips:   NewMocktripRepository(ctrl),
		keys:    NewMockidempotencyRepository(ctrl),
		tx:      NewMocktxManager(ctrl),
		created: NewMockcounter(ctrl),
	}

	d.usecase = create.New(d.trips, d.keys, d.tx, d.created, slog.New(slog.DiscardHandler), keyTTL,
		func() time.Time { return now })

	return d
}

func (d deps) expectTx() {
	d.tx.EXPECT().Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
}

func params() trip.Params {
	return trip.Params{
		UserID:     uuid.New(),
		DriverID:   uuid.New(),
		StartPoint: trip.Coordinates{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   trip.Coordinates{Latitude: 59.93, Longitude: 30.33},
		Price:      1500,
	}
}

func TestCreate(t *testing.T) {
	d := newDeps(t)
	p := params()

	var saved trip.Trip
	d.expectTx()
	d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, value trip.Trip) error {
		saved = value
		return nil
	})
	d.trips.EXPECT().AddStatusHistory(gomock.Any(), gomock.Cond(func(c model.StatusChange) bool {
		return c.TripID == saved.ID && c.From == nil && c.To == trip.StatusActive
	})).Return(nil)
	d.created.EXPECT().Inc(gomock.Any())

	result, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: p})

	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.Equal(t, saved, result.Trip)
	assert.NotEqual(t, uuid.Nil, result.Trip.ID)
	assert.Equal(t, p.DriverID, result.Trip.DriverID)
	assert.Equal(t, trip.StatusActive, result.Trip.Status)
	assert.Equal(t, now, result.Trip.StartedAt)
}

func TestCreateDriverBusy(t *testing.T) {
	d := newDeps(t)
	d.expectTx()
	d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(trip.ErrDriverBusy)

	_, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: params()})

	require.ErrorIs(t, err, trip.ErrDriverBusy)
}

func TestCreateInvalidParamsDoesNotTouchStorage(t *testing.T) {
	d := newDeps(t)
	p := params()
	p.DriverID = uuid.Nil

	_, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: p})

	var validationErr *trip.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "driver_id", validationErr.Field)
}

func TestCreateHistoryErrorIsReturned(t *testing.T) {
	d := newDeps(t)
	d.expectTx()
	d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	d.trips.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).Return(errDB)

	_, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: params()})

	require.ErrorIs(t, err, errDB)
}

func TestCreateTransactionError(t *testing.T) {
	d := newDeps(t)
	d.tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(errDB)

	_, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: params()})

	require.ErrorIs(t, err, errDB)
}

func TestCreateIdempotentFirstRequest(t *testing.T) {
	d := newDeps(t)
	key := uuid.New()

	var record model.IdempotencyRecord
	d.expectTx()
	d.keys.EXPECT().Lock(gomock.Any(), key).Return(nil)
	d.keys.EXPECT().Find(gomock.Any(), key, now.Add(-keyTTL)).Return(model.IdempotencyRecord{}, false, nil)
	d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	d.trips.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).Return(nil)
	d.keys.EXPECT().Save(gomock.Any(), key, gomock.Any(), now).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, r model.IdempotencyRecord, _ time.Time) error {
			record = r
			return nil
		})
	d.created.EXPECT().Inc(gomock.Any())

	result, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: params(), IdempotencyKey: &key})

	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.Equal(t, result.Trip.ID, record.TripID)
	assert.NotEmpty(t, record.RequestHash)
}

func TestCreateIdempotentReplay(t *testing.T) {
	d := newDeps(t)
	key := uuid.New()
	p := params()

	var record model.IdempotencyRecord
	d.tx.EXPECT().Do(gomock.Any(), gomock.Any()).Times(2).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
	d.keys.EXPECT().Lock(gomock.Any(), key).Times(2).Return(nil)
	first := d.keys.EXPECT().Find(gomock.Any(), key, gomock.Any()).Return(model.IdempotencyRecord{}, false, nil)
	d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	d.trips.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).Return(nil)
	d.keys.EXPECT().Save(gomock.Any(), key, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, r model.IdempotencyRecord, _ time.Time) error {
			record = r
			return nil
		})
	d.created.EXPECT().Inc(gomock.Any()).Times(1)

	firstResult, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: p, IdempotencyKey: &key})
	require.NoError(t, err)

	d.keys.EXPECT().Find(gomock.Any(), key, gomock.Any()).After(first).
		DoAndReturn(func(context.Context, uuid.UUID, time.Time) (model.IdempotencyRecord, bool, error) {
			return record, true, nil
		})
	d.trips.EXPECT().Get(gomock.Any(), record.TripID).Return(firstResult.Trip, nil)

	result, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: p, IdempotencyKey: &key})

	require.NoError(t, err)
	assert.False(t, result.Created)
	assert.Equal(t, firstResult.Trip, result.Trip)
}

func TestCreateIdempotencyConflict(t *testing.T) {
	d := newDeps(t)
	key := uuid.New()
	d.expectTx()
	d.keys.EXPECT().Lock(gomock.Any(), key).Return(nil)
	d.keys.EXPECT().Find(gomock.Any(), key, gomock.Any()).
		Return(model.IdempotencyRecord{RequestHash: "other", TripID: uuid.New()}, true, nil)

	_, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: params(), IdempotencyKey: &key})

	require.ErrorIs(t, err, trip.ErrIdempotencyConflict)
}

func TestCreateIdempotencyStorageErrors(t *testing.T) {
	tests := []struct {
		name   string
		expect func(d deps, key uuid.UUID)
	}{
		{name: "lock", expect: func(d deps, key uuid.UUID) {
			d.keys.EXPECT().Lock(gomock.Any(), key).Return(errDB)
		}},
		{name: "find", expect: func(d deps, key uuid.UUID) {
			d.keys.EXPECT().Lock(gomock.Any(), key).Return(nil)
			d.keys.EXPECT().Find(gomock.Any(), key, gomock.Any()).Return(model.IdempotencyRecord{}, false, errDB)
		}},
		{name: "save trip", expect: func(d deps, key uuid.UUID) {
			d.keys.EXPECT().Lock(gomock.Any(), key).Return(nil)
			d.keys.EXPECT().Find(gomock.Any(), key, gomock.Any()).Return(model.IdempotencyRecord{}, false, nil)
			d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(errDB)
		}},
		{name: "save key", expect: func(d deps, key uuid.UUID) {
			d.keys.EXPECT().Lock(gomock.Any(), key).Return(nil)
			d.keys.EXPECT().Find(gomock.Any(), key, gomock.Any()).Return(model.IdempotencyRecord{}, false, nil)
			d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
			d.trips.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).Return(nil)
			d.keys.EXPECT().Save(gomock.Any(), key, gomock.Any(), gomock.Any()).Return(errDB)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDeps(t)
			key := uuid.New()
			d.expectTx()
			tt.expect(d, key)

			_, err := d.usecase.Create(context.Background(), model.CreateTripInput{Params: params(), IdempotencyKey: &key})

			require.ErrorIs(t, err, errDB)
		})
	}
}
