package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type TxBeginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

type TxManager interface {
	Do(context.Context, func(context.Context) error) error
}

type Manager struct {
	pool    TxBeginner
	timeout time.Duration
}

type txKey struct{}

func NewManager(pool TxBeginner, timeout time.Duration) *Manager {
	return &Manager{pool: pool, timeout: timeout}
}

func (m *Manager) Do(ctx context.Context, fn func(context.Context) error) error {
	if transactionFrom(ctx) != nil {
		return fn(ctx)
	}

	beginCtx, cancelBegin := context.WithTimeout(ctx, m.timeout)
	defer cancelBegin()

	tx, err := m.pool.BeginTx(beginCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.timeout)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}

	commitCtx, cancelCommit := context.WithTimeout(ctx, m.timeout)
	defer cancelCommit()

	if err := tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func Executor(ctx context.Context, pool DBTX) DBTX {
	if tx := transactionFrom(ctx); tx != nil {
		return tx
	}

	return pool
}

func transactionFrom(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(txKey{}).(pgx.Tx)
	return tx
}
