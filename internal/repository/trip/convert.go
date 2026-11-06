package trip

import "github.com/mrsobakin/itmo-avito-go/internal/domain/trip"

var tripColumns = []string{
	"id", "user_id", "driver_id",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", "status", "started_at", "finished_at",
}

func tripValues(value trip.Trip) []any {
	return []any{
		value.ID, value.UserID, value.DriverID,
		value.StartPoint.Latitude, value.StartPoint.Longitude,
		value.EndPoint.Latitude, value.EndPoint.Longitude,
		value.Price, value.Status, value.StartedAt, value.FinishedAt,
	}
}

func tripDestinations(value *trip.Trip) []any {
	return []any{
		&value.ID, &value.UserID, &value.DriverID,
		&value.StartPoint.Latitude, &value.StartPoint.Longitude,
		&value.EndPoint.Latitude, &value.EndPoint.Longitude,
		&value.Price, &value.Status, &value.StartedAt, &value.FinishedAt,
	}
}

func normalize(value trip.Trip) trip.Trip {
	value.StartedAt = value.StartedAt.UTC()

	if value.FinishedAt != nil {
		value.FinishedAt = new(value.FinishedAt.UTC())
	}

	return value
}
