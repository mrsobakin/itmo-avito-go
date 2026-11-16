//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}
package positions

import (
	"context"
	"time"

	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type (
	tripRepository interface {
		ListStale(ctx context.Context, staleAfter time.Duration, after *model.StaleTrip, limit uint64) ([]model.StaleTrip, error)
	}

	pusher interface {
		RequestPosition(ctx context.Context, value model.StaleTrip) error
	}
)

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type realTicker struct {
	ticker *time.Ticker
}

func NewTicker(interval time.Duration) Ticker {
	return realTicker{ticker: time.NewTicker(interval)}
}

func (t realTicker) C() <-chan time.Time {
	return t.ticker.C
}

func (t realTicker) Stop() {
	t.ticker.Stop()
}
