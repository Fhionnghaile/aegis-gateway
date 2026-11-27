package publish

import (
	"context"
	"sync"

	"github.com/Fhionnghaile/aegis-gateway/internal/event"
)

// Publisher is the sink the worker pool drains to (Kafka in prod, memory in tests).
type Publisher interface {
	Publish(ctx context.Context, e event.Event) error
	Close() error
}

// MemoryPublisher is an in-memory Publisher for tests and local runs.
type MemoryPublisher struct {
	mu     sync.Mutex
	events []event.Event
}

func (m *MemoryPublisher) Publish(_ context.Context, e event.Event) error {
	m.mu.Lock()
	m.events = append(m.events, e)
	m.mu.Unlock()
	return nil
}

func (m *MemoryPublisher) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *MemoryPublisher) Close() error { return nil }
