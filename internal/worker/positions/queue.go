package positions

import (
	"bytes"
	"container/heap"
	"context"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

type task struct {
	trip  model.StaleTrip
	link  trace.SpanContext
	index int
}

func (t *task) staler(other *task) bool {
	if !t.trip.SilentSince.Equal(other.trip.SilentSince) {
		return t.trip.SilentSince.Before(other.trip.SilentSince)
	}

	return bytes.Compare(t.trip.ID[:], other.trip.ID[:]) < 0
}

type taskHeap []*task

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool { return h[i].staler(h[j]) }

func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *taskHeap) Push(value any) {
	t, _ := value.(*task)
	t.index = len(*h)
	*h = append(*h, t)
}

func (h *taskHeap) Pop() any {
	old := *h
	t := old[len(old)-1]
	old[len(old)-1] = nil
	t.index = -1
	*h = old[:len(old)-1]

	return t
}

type queue struct {
	mu       sync.Mutex
	ready    *sync.Cond
	tasks    taskHeap
	tracked  map[uuid.UUID]*task
	capacity int
	closed   bool
}

func newQueue(capacity int) *queue {
	q := &queue{tracked: map[uuid.UUID]*task{}, capacity: capacity}
	q.ready = sync.NewCond(&q.mu)

	return q
}

func (q *queue) push(t *task) (queued bool, dropped *task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.tracked[t.trip.ID]; ok {
		return false, nil
	}

	if len(q.tasks) >= q.capacity {
		freshest := q.freshest()
		if !t.staler(freshest) {
			return false, t
		}

		heap.Remove(&q.tasks, freshest.index)
		delete(q.tracked, freshest.trip.ID)
		dropped = freshest
	}

	heap.Push(&q.tasks, t)
	q.tracked[t.trip.ID] = t
	q.ready.Signal()

	return true, dropped
}

func (q *queue) freshest() *task {
	freshest := q.tasks[0]
	for _, t := range q.tasks[len(q.tasks)/2:] {
		if freshest.staler(t) {
			freshest = t
		}
	}

	return freshest
}

func (q *queue) pop(ctx context.Context) (*task, bool) {
	stop := context.AfterFunc(ctx, q.wake)
	defer stop()

	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.tasks) == 0 && !q.closed && ctx.Err() == nil {
		q.ready.Wait()
	}

	if ctx.Err() != nil || len(q.tasks) == 0 {
		return nil, false
	}

	t, _ := heap.Pop(&q.tasks).(*task)

	return t, true
}

func (q *queue) done(t *task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.tracked, t.trip.ID)
}

func (q *queue) cancel(id uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	t, ok := q.tracked[id]
	if !ok || t.index < 0 {
		return false
	}

	heap.Remove(&q.tasks, t.index)
	delete(q.tracked, id)

	return true
}

func (q *queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.ready.Broadcast()
}

func (q *queue) wake() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.ready.Broadcast()
}

func (q *queue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.tasks)
}
