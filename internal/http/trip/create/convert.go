package create

import (
	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

func toInput(body api.TripData, params api.CreateTripParams) model.CreateTripInput {
	return model.CreateTripInput{
		Params: trip.Params{
			UserID:   body.UserId,
			DriverID: body.DriverId,
			StartPoint: trip.Coordinates{
				Latitude:  body.StartPoint.Latitude,
				Longitude: body.StartPoint.Longitude,
			},
			EndPoint: trip.Coordinates{
				Latitude:  body.EndPoint.Latitude,
				Longitude: body.EndPoint.Longitude,
			},
			Price: body.Price,
		},
		IdempotencyKey: params.IdempotencyKey,
	}
}
