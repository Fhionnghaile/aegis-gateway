package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// Metrics is a tiny, dependency-free Prometheus-text exposer. Swap for
// prometheus/client_golang in prod; the counter names are stable.
type Metrics struct {
	Received  atomic.Int64
	Accepted  atomic.Int64
	Rejected  atomic.Int64 // failed validation -> 400
	Dropped   atomic.Int64 // back-pressure     -> 429
	Published atomic.Int64
}

func (m *Metrics) Handler() http.HandlerFunc {
	return func(wr http.ResponseWriter, _ *http.Request) {
		wr.Header().Set("Content-Type", "text/plain; version=0.0.4")
		for _, kv := range []struct {
			name string
			val  int64
		}{
			{"aegis_events_received_total", m.Received.Load()},
			{"aegis_events_accepted_total", m.Accepted.Load()},
			{"aegis_events_rejected_total", m.Rejected.Load()},
			{"aegis_events_dropped_total", m.Dropped.Load()},
			{"aegis_events_published_total", m.Published.Load()},
		} {
			fmt.Fprintf(wr, "# TYPE %s counter\n%s %d\n", kv.name, kv.name, kv.val)
		}
	}
}
