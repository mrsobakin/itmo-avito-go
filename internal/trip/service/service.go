package service

import (
	"time"

	"github.com/mrsobakin/itmo-avito-go/internal/transaction"
)

type Service struct {
	repository        repository
	txManager         transaction.TxManager
	idempotencyKeyTTL time.Duration
	now               func() time.Time
}

func New(repository repository, txManager transaction.TxManager, idempotencyKeyTTL time.Duration) *Service {
	return &Service{
		repository:        repository,
		txManager:         txManager,
		idempotencyKeyTTL: idempotencyKeyTTL,
		now:               func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	}
}
