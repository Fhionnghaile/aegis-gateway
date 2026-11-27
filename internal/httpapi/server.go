package httpapi

import (
	"errors"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/Fhionnghaile/aegis-gateway/internal/event"
	"github.com/Fhionnghaile/aegis-gateway/internal/ingest"
	"github.com/Fhionnghaile/aegis-gateway/internal/metrics"
)

// Server exposes /events, /healthz, /metrics. ErrInvalid->400, ErrFull->429.
type Server struct {
	val   event.Validator
	q     *ingest.Queue
	m     *metrics.Metrics
	ready atomic.Bool
}

func New(val event.Validator, q *ingest.Queue, m *metrics.Metrics) *Server {
	s := &Server{val: val, q: q, m: m}
	s.ready.Store(true)
	return s
}

// SetReady(false) makes /healthz return 503 and /events 503 (used during drain).
func (s *Server) SetReady(v bool) { s.ready.Store(v) }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /events", s.postEvents)
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.Handle("GET /metrics", s.m.Handler())
	return mux
}

func (s *Server) postEvents(wr http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		http.Error(wr, "draining", http.StatusServiceUnavailable)
		return
	}
	s.m.Received.Add(1)
	body, err := io.ReadAll(http.MaxBytesReader(wr, r.Body, 1<<20))
	if err != nil {
		s.m.Rejected.Add(1)
		http.Error(wr, "bad body", http.StatusBadRequest)
		return
	}
	e, err := s.val.Validate(body)
	if err != nil {
		if errors.Is(err, event.ErrInvalid) {
			s.m.Rejected.Add(1)
			http.Error(wr, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(wr, "internal", http.StatusInternalServerError)
		return
	}
	if err := s.q.Enqueue(e); errors.Is(err, ingest.ErrFull) {
		s.m.Dropped.Add(1)
		http.Error(wr, "overloaded, retry", http.StatusTooManyRequests)
		return
	}
	s.m.Accepted.Add(1)
	wr.WriteHeader(http.StatusAccepted)
	_, _ = io.WriteString(wr, e.ID)
}

func (s *Server) healthz(wr http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		http.Error(wr, "draining", http.StatusServiceUnavailable)
		return
	}
	_, _ = io.WriteString(wr, "ok")
}
