package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Fhionnghaile/aegis-gateway/internal/event"
	"github.com/Fhionnghaile/aegis-gateway/internal/ingest"
	"github.com/Fhionnghaile/aegis-gateway/internal/metrics"
)

func newTestServer(cap int) *Server {
	v, _ := event.NewSchemaValidator("")
	return New(v, ingest.NewQueue(cap), &metrics.Metrics{})
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/events", strings.NewReader(body))
	wr := httptest.NewRecorder()
	h.ServeHTTP(wr, r)
	return wr
}

func TestPostEvents(t *testing.T) {
	h := newTestServer(100).Handler()
	if got := post(h, `{"source":"edr","type":"login_fail"}`).Code; got != http.StatusAccepted {
		t.Fatalf("valid: want 202, got %d", got)
	}
	if got := post(h, `{"type":"x"}`).Code; got != http.StatusBadRequest {
		t.Fatalf("invalid: want 400, got %d", got)
	}
}

func TestBackPressure429(t *testing.T) {
	s := newTestServer(1)
	h := s.Handler()
	post(h, `{"source":"a","type":"b"}`) // fills cap-1 slot... cap=1 so this one sits in queue
	if got := post(h, `{"source":"a","type":"b"}`).Code; got != http.StatusTooManyRequests {
		t.Fatalf("overload: want 429, got %d", got)
	}
}

func TestHealthAndDrain(t *testing.T) {
	s := newTestServer(10)
	h := s.Handler()
	r := httptest.NewRequest("GET", "/healthz", nil)
	wr := httptest.NewRecorder()
	h.ServeHTTP(wr, r)
	if wr.Code != 200 {
		t.Fatalf("healthz: %d", wr.Code)
	}
	s.SetReady(false)
	if got := post(h, `{"source":"a","type":"b"}`).Code; got != http.StatusServiceUnavailable {
		t.Fatalf("draining: want 503, got %d", got)
	}
}

func TestMetricsExposed(t *testing.T) {
	h := newTestServer(10).Handler()
	post(h, `{"source":"a","type":"b"}`)
	r := httptest.NewRequest("GET", "/metrics", nil)
	wr := httptest.NewRecorder()
	h.ServeHTTP(wr, r)
	if !strings.Contains(wr.Body.String(), "aegis_events_accepted_total 1") {
		t.Fatalf("metrics missing counter:\n%s", wr.Body.String())
	}
}
