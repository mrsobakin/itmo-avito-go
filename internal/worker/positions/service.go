package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type Options struct {
	CheckInterval  time.Duration
	StaleAfter     time.Duration
	Workers        int
	QueueSize      int
	ScanBatchSize  uint64
	RequestTimeout time.Duration
}

type Deps struct {
	Trips     tripRepository
	Push      pusher
	NewTicker func(interval time.Duration) Ticker
	Meter     metric.Meter
	Tracer    trace.Tracer
	Logger    *slog.Logger
}

type Service struct {
	opts    Options
	deps    Deps
	queue   *queue
	metrics *metrics
	active  atomic.Int64

	stopScanner context.CancelFunc
	cancelWork  context.CancelFunc
	scanner     sync.WaitGroup
	workers     sync.WaitGroup
}

func New(opts Options, deps Deps) (*Service, error) {
	s := &Service{opts: opts, deps: deps, queue: newQueue(opts.QueueSize)}

	var err error

	s.metrics, err = newMetrics(deps.Meter, func() int64 { return int64(s.queue.len()) }, s.active.Load)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Service) Start(ctx context.Context) {
	workCtx, cancelWork := context.WithCancel(ctx)
	scanCtx, stopScanner := context.WithCancel(workCtx)
	s.cancelWork, s.stopScanner = cancelWork, stopScanner

	ticker := s.deps.NewTicker(s.opts.CheckInterval)
	s.scanner.Go(func() { s.runScanner(scanCtx, ticker) })

	for range s.opts.Workers {
		s.workers.Go(func() { s.runWorker(workCtx) })
	}

	s.deps.Logger.InfoContext(ctx, "position poller started",
		"workers", s.opts.Workers, "queue_size", s.opts.QueueSize, "check_interval", s.opts.CheckInterval.String())
}

func (s *Service) Stop(ctx context.Context) error {
	s.stopScanner()
	s.scanner.Wait()
	s.deps.Logger.InfoContext(ctx, "stale trip scanner stopped")

	s.queue.close()

	drained := make(chan struct{})
	go func() {
		s.workers.Wait()
		close(drained)
	}()

	var err error

	select {
	case <-drained:
	case <-ctx.Done():
		s.cancelWork()
		<-drained

		err = fmt.Errorf("drain position requests: %w", ctx.Err())
	}

	s.cancelWork()
	s.deps.Logger.InfoContext(ctx, "position request workers stopped", "abandoned", s.queue.len())

	return err
}

func (s *Service) Cancel(ctx context.Context, tripID uuid.UUID) {
	if s.queue.cancel(tripID) {
		s.metrics.requests.Add(ctx, 1, resultAttr(requestCancelled))
		s.deps.Logger.DebugContext(ctx, "position request cancelled", "trip_id", tripID)
	}
}

func (s *Service) runScanner(ctx context.Context, ticker Ticker) {
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			s.scan(ctx)
		}
	}
}

func (s *Service) scan(ctx context.Context) {
	ctx, span := s.deps.Tracer.Start(ctx, "positions.scan")
	defer span.End()

	var (
		after *model.StaleTrip
		read  int64
	)

	defer func() { s.metrics.rowsRead.Record(ctx, read) }()

	for {
		batch, err := s.deps.Trips.ListStale(ctx, s.opts.StaleAfter, after, s.opts.ScanBatchSize)
		if err != nil {
			if ctx.Err() == nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "list stale trips")
				s.deps.Logger.ErrorContext(ctx, "stale trip scan failed", "error", err)
			}

			return
		}

		read += int64(len(batch))

		for _, value := range batch {
			if !s.enqueue(ctx, &task{trip: value, link: span.SpanContext()}) {
				return
			}
		}

		if uint64(len(batch)) < s.opts.ScanBatchSize {
			return
		}

		after = &batch[len(batch)-1]
	}
}

func (s *Service) enqueue(ctx context.Context, t *task) bool {
	queued, dropped := s.queue.push(t)

	switch {
	case queued:
		s.metrics.staleChecks.Add(ctx, 1, resultAttr(checkQueued))
	case dropped == nil:
		s.metrics.staleChecks.Add(ctx, 1, resultAttr(checkDuplicate))
	}

	if dropped != nil {
		s.metrics.staleChecks.Add(ctx, 1, resultAttr(checkDropped))
		s.deps.Logger.WarnContext(ctx, "position request queue is full, trip dropped", "trip_id", dropped.trip.ID)
	}

	return queued || dropped == nil
}

func (s *Service) runWorker(ctx context.Context) {
	for {
		t, ok := s.queue.pop(ctx)
		if !ok {
			return
		}

		s.process(ctx, t)
		s.queue.done(t)
	}
}

func (s *Service) process(ctx context.Context, t *task) {
	s.active.Add(1)
	defer s.active.Add(-1)

	ctx, span := s.deps.Tracer.Start(ctx, "positions.request",
		trace.WithNewRoot(),
		trace.WithLinks(trace.Link{SpanContext: t.link}),
		trace.WithAttributes(attribute.String("trip.id", t.trip.ID.String())),
	)
	defer span.End()

	requestCtx, cancel := context.WithTimeout(ctx, s.opts.RequestTimeout)
	defer cancel()

	start := time.Now()
	err := s.deps.Push.RequestPosition(requestCtx, t.trip)
	elapsed := time.Since(start).Seconds()

	result := requestSuccess
	if err != nil {
		result = requestError
		span.RecordError(err)
		span.SetStatus(codes.Error, "request position")
		s.deps.Logger.WarnContext(ctx, "position request failed", "trip_id", t.trip.ID, "error", err)
	} else {
		s.deps.Logger.DebugContext(ctx, "position requested", "trip_id", t.trip.ID)
	}

	s.metrics.requests.Add(ctx, 1, resultAttr(result))
	s.metrics.duration.Record(ctx, elapsed, resultAttr(result))
}
