package trip

import (
	"time"

	"github.com/google/uuid"
)

const MaxPositionClockSkew = time.Minute

type Position struct {
	ID          int64
	TripID      uuid.UUID
	Coordinates Coordinates
	RecordedAt  time.Time
}

func NewPosition(tripID uuid.UUID, coordinates Coordinates, recordedAt, now time.Time) (Position, error) {
	if err := coordinates.validate("coordinates"); err != nil {
		return Position{}, err
	}

	if recordedAt.After(now.Add(MaxPositionClockSkew)) {
		return Position{}, &ValidationError{Field: "recorded_at", Reason: "must not be in the future"}
	}

	return Position{TripID: tripID, Coordinates: coordinates, RecordedAt: recordedAt}, nil
}

func (t *Trip) CanRecordPosition() error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}

	return nil
}
