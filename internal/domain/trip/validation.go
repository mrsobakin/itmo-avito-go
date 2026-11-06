package trip

import (
	"math"

	"github.com/google/uuid"
)

const (
	maxLatitude  = 90
	maxLongitude = 180
)

func (p Params) Validate() error {
	if p.UserID == uuid.Nil {
		return &ValidationError{Field: "user_id", Reason: "must be a non-empty UUID"}
	}

	if p.DriverID == uuid.Nil {
		return &ValidationError{Field: "driver_id", Reason: "must be a non-empty UUID"}
	}

	if err := p.StartPoint.validate("start_point"); err != nil {
		return err
	}

	if err := p.EndPoint.validate("end_point"); err != nil {
		return err
	}

	if p.Price < 0 {
		return &ValidationError{Field: "price", Reason: "must be non-negative"}
	}

	return nil
}

func (c Coordinates) validate(field string) error {
	if math.IsNaN(c.Latitude) || c.Latitude < -maxLatitude || c.Latitude > maxLatitude {
		return &ValidationError{Field: field + ".latitude", Reason: "must be between -90 and 90"}
	}

	if math.IsNaN(c.Longitude) || c.Longitude < -maxLongitude || c.Longitude > maxLongitude {
		return &ValidationError{Field: field + ".longitude", Reason: "must be between -180 and 180"}
	}

	return nil
}
