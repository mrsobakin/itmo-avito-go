//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type usecase interface {
	Create(ctx context.Context, input model.CreateTripInput) (model.CreateTripResult, error)
}
