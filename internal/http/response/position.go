package response

import (
	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
)

func Position(value trip.Position) api.TripPosition {
	return api.TripPosition{
		Id:     value.ID,
		TripId: value.TripID,
		Coordinates: api.Coordinates{
			Latitude:  value.Coordinates.Latitude,
			Longitude: value.Coordinates.Longitude,
		},
		RecordedAt: value.RecordedAt,
	}
}

func Positions(values []trip.Position) api.TripPositionsResponse {
	positions := make([]api.TripPosition, 0, len(values))
	for _, value := range values {
		positions = append(positions, Position(value))
	}

	return api.TripPositionsResponse{Positions: positions}
}
