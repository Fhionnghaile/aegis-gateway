package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Fhionnghaile/aegis-gateway/internal/contain"
	"github.com/Fhionnghaile/aegis-gateway/internal/event"
	"github.com/Fhionnghaile/aegis-gateway/internal/ingest"
	"github.com/Fhionnghaile/aegis-gateway/internal/metrics"
)

// Server exposes /events and /healthz on the public listener, and /metrics on a separate
// internal-only admin listener (threat model I1). ErrInvalid->400, ErrFull->429.
type Server struct {
	val   event.Validator
	q     *ingest.Queue
	m     *metrics.Metrics
	ready atomic.Bool
	bl    *contain.Blocklist // nil = containment off
	guard *contain.Guard     // nil = no automatic blocking
}

// WithContainment turns on blocking (admin API + checks on /events) and, if g is non-nil,
// automatic blocking of clients that flood the gateway with invalid events.
func (s *Server) WithContainment(bl *contain.Blocklist, g *contain.Guard) *Server {
	s.bl, s.guard = bl, g
	return s
}

// clientIP is the TCP peer address. X-Forwarded-For is deliberately NOT trusted: an attacker
// could set it to frame someone else or dodge a block.
func clientIP(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
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
	return mux
}

// AdminHandler serves /metrics. Bind it to an internal address only (default 127.0.0.1:9091),
// so traffic volumes and rejection rates are not visible to event senders (threat model I1).
func (s *Server) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", s.m.Handler())
	if s.bl != nil {
		mux.HandleFunc("POST /admin/block", s.adminBlock)
		mux.HandleFunc("POST /admin/unblock", s.adminUnblock)
		mux.HandleFunc("GET /admin/blocked", s.adminBlocked)
	}
	return mux
}

type blockReq struct {
	Key        string `json:"key"`
	TTLSeconds int    `json:"ttl_seconds"`
	Reason     string `json:"reason"`
}

// POST /admin/block {"key":"ip:1.2.3.4"|"source:<name>","ttl_seconds":900,"reason":"..."}
// Used by the responder: a detection elsewhere in the pipeline becomes a block here.
func (s *Server) adminBlock(wr http.ResponseWriter, r *http.Request) {
	var q blockReq
	if err := json.NewDecoder(http.MaxBytesReader(wr, r.Body, 4<<10)).Decode(&q); err != nil || !contain.ValidKey(q.Key) {
		http.Error(wr, "want {key: ip:<addr>|source:<name>, ttl_seconds, reason}", http.StatusBadRequest)
		return
	}
	if q.TTLSeconds <= 0 || q.TTLSeconds > 7*24*3600 {
		q.TTLSeconds = 900
	}
	e := s.bl.Block(q.Key, q.Reason, time.Duration(q.TTLSeconds)*time.Second)
	wr.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(wr).Encode(e)
}

func (s *Server) adminUnblock(wr http.ResponseWriter, r *http.Request) {
	var q blockReq
	if err := json.NewDecoder(http.MaxBytesReader(wr, r.Body, 4<<10)).Decode(&q); err != nil || !contain.ValidKey(q.Key) {
		http.Error(wr, "want {key}", http.StatusBadRequest)
		return
	}
	wr.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(wr).Encode(map[string]bool{"removed": s.bl.Unblock(q.Key)})
}

func (s *Server) adminBlocked(wr http.ResponseWriter, _ *http.Request) {
	wr.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(wr).Encode(s.bl.List())
}

// Timeouts applied to every listener (threat model D3: slow clients must not hold connections
// open). Bodies are already capped at 1 MiB by MaxBytesReader.
const (
	ReadHeaderTimeout = 5 * time.Second
	ReadTimeout       = 15 * time.Second
	WriteTimeout      = 15 * time.Second
	IdleTimeout       = 60 * time.Second
	MaxHeaderBytes    = 64 << 10
)

// NewHTTPServer wraps a handler in an http.Server with the timeouts above.
func NewHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: ReadHeaderTimeout,
		ReadTimeout:       ReadTimeout,
		WriteTimeout:      WriteTimeout,
		IdleTimeout:       IdleTimeout,
		MaxHeaderBytes:    MaxHeaderBytes,
	}
}

func (s *Server) postEvents(wr http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		http.Error(wr, "draining", http.StatusServiceUnavailable)
		return
	}
	s.m.Received.Add(1)
	ip := clientIP(r)
	if s.bl != nil && s.bl.Blocked("ip:"+ip) {
		s.m.Refused.Add(1)
		http.Error(wr, "blocked", http.StatusForbidden)
		return
	}
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
			if s.guard != nil && s.guard.Bad(ip) {
				s.m.Contained.Add(1)
			}
			http.Error(wr, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(wr, "internal", http.StatusInternalServerError)
		return
	}
	if s.bl != nil && s.bl.Blocked("source:"+e.Source) {
		s.m.Refused.Add(1)
		http.Error(wr, "source blocked", http.StatusForbidden)
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
