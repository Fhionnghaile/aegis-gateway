package httpapi

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestMetricsExposedOnAdminListenerOnly(t *testing.T) {
	s := newTestServer(10)
	post(s.Handler(), `{"source":"a","type":"b"}`)

	wr := httptest.NewRecorder()
	s.AdminHandler().ServeHTTP(wr, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(wr.Body.String(), "aegis_events_accepted_total 1") {
		t.Fatalf("admin /metrics missing counter:\n%s", wr.Body.String())
	}

	// Threat model I1: event senders must not be able to read operational metrics.
	pub := httptest.NewRecorder()
	s.Handler().ServeHTTP(pub, httptest.NewRequest("GET", "/metrics", nil))
	if pub.Code != http.StatusNotFound {
		t.Fatalf("public /metrics: want 404, got %d", pub.Code)
	}
}

// Threat model D3: every listener carries timeouts, so slow clients cannot hold connections open.
func TestHTTPServerHasTimeouts(t *testing.T) {
	hs := NewHTTPServer(":0", http.NewServeMux())
	if hs.ReadHeaderTimeout <= 0 || hs.ReadTimeout <= 0 || hs.WriteTimeout <= 0 || hs.IdleTimeout <= 0 {
		t.Fatalf("missing timeouts: %+v", hs)
	}
	if hs.MaxHeaderBytes != MaxHeaderBytes || hs.MaxHeaderBytes > 1<<20 {
		t.Fatalf("MaxHeaderBytes = %d", hs.MaxHeaderBytes)
	}
}

// A client that sends headers slower than ReadHeaderTimeout is disconnected.
func TestSlowHeadersAreCutOff(t *testing.T) {
	hs := NewHTTPServer("127.0.0.1:0", newTestServer(10).Handler())
	hs.ReadHeaderTimeout = 200 * time.Millisecond // shortened for the test
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go hs.Serve(ln)
	defer hs.Close()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("POST /events HTTP/1.1\r\nHost: x\r\n")) // never finishes the headers
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 512)
	start := time.Now()
	for {
		if _, err := c.Read(buf); err != nil {
			break // server closed the connection (or sent its timeout response and closed)
		}
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("slow client held the connection for %v", d)
	}
}
