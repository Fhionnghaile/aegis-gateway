package ingest

import (
	"context"
	"testing"

	"github.com/padraigf/aegis-gateway/internal/event"
	"github.com/padraigf/aegis-gateway/internal/metrics"
	"github.com/padraigf/aegis-gateway/internal/publish"
)

func TestPoolDrainsOnStop(t *testing.T) {
	q := NewQueue(1000)
	pub := &publish.MemoryPublisher{}
	m := &metrics.Metrics{}
	p := NewPool(q, pub, 4, m)
	p.Start(context.Background())
	const N = 500
	for i := 0; i < N; i++ {
		if err := q.Enqueue(event.Event{Source: "s", Type: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	p.Stop() // closes queue, waits for drain
	if pub.Count() != N {
		t.Fatalf("drain lost events: got %d want %d", pub.Count(), N)
	}
	if m.Published.Load() != int64(N) {
		t.Fatalf("published metric: %d", m.Published.Load())
	}
}
