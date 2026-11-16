package ingest

import (
	"errors"

	"github.com/padraigf/aegis-gateway/internal/event"
)

// ErrFull signals back-pressure: the bounded queue is at capacity.
var ErrFull = errors.New("ingest queue full")

// Queue is a bounded, lock-free (channel-backed) hand-off. Back-pressure lives here.
type Queue struct {
	ch chan event.Event
}

func NewQueue(capacity int) *Queue {
	if capacity < 1 {
		capacity = 1
	}
	return &Queue{ch: make(chan event.Event, capacity)}
}

// Enqueue never blocks: it returns ErrFull immediately when the queue is full.
func (q *Queue) Enqueue(e event.Event) error {
	select {
	case q.ch <- e:
		return nil
	default:
		return ErrFull
	}
}

func (q *Queue) C() <-chan event.Event { return q.ch }
func (q *Queue) Len() int              { return len(q.ch) }
func (q *Queue) Cap() int              { return cap(q.ch) }
func (q *Queue) Close()                { close(q.ch) }
