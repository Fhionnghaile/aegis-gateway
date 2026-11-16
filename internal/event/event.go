package event

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

// Event is the normalised security event the gateway ingests and forwards.
type Event struct {
	ID         string         `json:"id,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
	ReceivedAt time.Time      `json:"received_at"`
	Source     string         `json:"source"`
	Type       string         `json:"type"`
	Severity   string         `json:"severity"`
	Actor      map[string]any `json:"actor,omitempty"`
	Target     map[string]any `json:"target,omitempty"`
	Raw        map[string]any `json:"raw,omitempty"`
}

// Normalise fills ReceivedAt, lowercases Type/Severity, defaults Severity, assigns ID if empty.
func (e *Event) Normalise(now time.Time) {
	e.ReceivedAt = now
	e.Type = strings.ToLower(strings.TrimSpace(e.Type))
	e.Severity = strings.ToLower(strings.TrimSpace(e.Severity))
	if e.Severity == "" {
		e.Severity = "info"
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = now
	}
	if e.ID == "" {
		var b [16]byte
		_, _ = rand.Read(b[:])
		e.ID = hex.EncodeToString(b[:])
	}
}

// PartitionKey keys Kafka by source|type for even, order-preserving distribution.
func (e Event) PartitionKey() []byte {
	return []byte(e.Source + "|" + e.Type)
}
