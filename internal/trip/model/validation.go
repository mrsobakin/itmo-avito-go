package model

import "github.com/google/uuid"

const (
	maxLatitude  = 90
	maxLongitude = 180
)

func (p CreateParams) Validate() error {
	if p.UserID == uuid.Nil {
		return InvalidArgument("user_id must be a non-empty UUID")
	}

	if p.DriverID == uuid.Nil {
		return InvalidArgument("driver_id must be a non-empty UUID")
	}

	if err := p.StartPoint.validate("start_point"); err != nil {
		return err
	}

	if err := p.EndPoint.validate("end_point"); err != nil {
		return err
	}

	if p.Price < 0 {
		return InvalidArgument("price must be non-negative")
	}

	return nil
}

func (c Coordinates) validate(field string) error {
	if c.Latitude < -maxLatitude || c.Latitude > maxLatitude {
		return InvalidArgument(field + ".latitude must be between -90 and 90")
	}

	if c.Longitude < -maxLongitude || c.Longitude > maxLongitude {
		return InvalidArgument(field + ".longitude must be between -180 and 180")
	}

	return nil
}
