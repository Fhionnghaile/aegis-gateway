package metrics

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func scrape(t *testing.T, m *Metrics) (string, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler()(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String(), rec.Header().Get("Content-Type")
}

func TestExposesEveryCounterInPrometheusTextFormat(t *testing.T) {
	var m Metrics
	m.Received.Add(5)
	m.Accepted.Add(3)
	m.Rejected.Add(1)
	m.Dropped.Add(1)
	m.Published.Add(2)
	m.Refused.Add(4)
	m.Contained.Add(1)
	body, ct := scrape(t, &m)
	if ct != "text/plain; version=0.0.4" {
		t.Fatalf("content type %q", ct)
	}
	for name, v := range map[string]string{
		"aegis_events_received_total": "5", "aegis_events_accepted_total": "3",
		"aegis_events_rejected_total": "1", "aegis_events_dropped_total": "1",
		"aegis_events_published_total": "2", "aegis_events_refused_total": "4",
		"aegis_containments_total": "1",
	} {
		if !strings.Contains(body, "# TYPE "+name+" counter\n") {
			t.Errorf("missing TYPE line for %s", name)
		}
		if !strings.Contains(body, "\n"+name+" "+v+"\n") {
			t.Errorf("want %s %s in:\n%s", name, v, body)
		}
	}
}

// Counters are bumped from many request goroutines at once; run with -race.
func TestConcurrentIncrementsAreExact(t *testing.T) {
	var m Metrics
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				m.Received.Add(1)
			}
			scrape(t, &m)
		}()
	}
	wg.Wait()
	if body, _ := scrape(t, &m); !strings.Contains(body, "\naegis_events_received_total 10000\n") {
		t.Fatalf("want 10000 received, got:\n%s", body)
	}
}
