package request

import (
	"encoding/json"
	"errors"
	"io"
)

var errTrailingData = errors.New("unexpected data after JSON value")

func DecodeJSON(body io.Reader, destination any) error {
	decoder := json.NewDecoder(body)

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errTrailingData
	}

	return nil
}
