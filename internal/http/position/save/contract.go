//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package save

import (
	"context"

	"github.com/mrsobakin/itmo-avito-go/internal/domain/trip"
	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type usecase interface {
	Save(ctx context.Context, input model.SavePositionInput) (trip.Position, error)
}
