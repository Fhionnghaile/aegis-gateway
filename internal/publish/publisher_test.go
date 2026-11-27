package publish

import (
	"context"
	"sync"
	"testing"

	"github.com/Fhionnghaile/aegis-gateway/internal/event"
)

var _ Publisher = (*MemoryPublisher)(nil) // compile-time: the memory sink satisfies the interface

func TestMemoryPublisherCountsEveryEvent(t *testing.T) {
	m := &MemoryPublisher{}
	for i := 0; i < 3; i++ {
		if err := m.Publish(context.Background(), event.Event{}); err != nil {
			t.Fatal(err)
		}
	}
	if m.Count() != 3 {
		t.Fatalf("count %d, want 3", m.Count())
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}

// Workers publish concurrently; no event may be lost. Run with -race.
func TestMemoryPublisherConcurrentPublishLosesNothing(t *testing.T) {
	m := &MemoryPublisher{}
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				_ = m.Publish(context.Background(), event.Event{})
			}
		}()
	}
	wg.Wait()
	if m.Count() != 8000 {
		t.Fatalf("count %d, want 8000", m.Count())
	}
}
