package ingest

import (
	"errors"
	"testing"

	"github.com/Fhionnghaile/aegis-gateway/internal/event"
)

func TestBackPressure(t *testing.T) {
	q := NewQueue(2)
	if err := q.Enqueue(event.Event{}); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(event.Event{}); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(event.Event{}); !errors.Is(err, ErrFull) {
		t.Fatalf("want ErrFull at cap, got %v", err)
	}
}
