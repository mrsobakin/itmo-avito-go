package create

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/mrsobakin/itmo-avito-go/api"
	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

var errTrailingData = errors.New("unexpected data after JSON value")

func decodeJSON(body io.Reader, destination any) error {
	decoder := json.NewDecoder(body)

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errTrailingData
	}

	return nil
}

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
