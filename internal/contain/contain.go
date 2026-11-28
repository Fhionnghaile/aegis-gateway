// Package contain is the gateway's containment layer (threat model S1/D5, ML n20_10):
//   - Blocklist: time-limited blocks on a client IP ("ip:<addr>") or an event source
//     ("source:<name>"), set automatically by the Guard or by an external responder through the
//     admin API.
//   - Guard: an in-process detector for abuse of the gateway itself. A client that sends
//     Threshold invalid events within Window is blocked for TTL, without waiting for a human.
package contain

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry is one active block.
type Entry struct {
	Key    string    `json:"key"`
	Reason string    `json:"reason"`
	At     time.Time `json:"blocked_at"`
	Until  time.Time `json:"until"`
}

// ValidKey reports whether key is a supported block key: "ip:<addr>" or "source:<name>".
func ValidKey(key string) bool {
	for _, p := range []string{"ip:", "source:"} {
		if strings.HasPrefix(key, p) && len(key) > len(p) && len(key) <= 256 {
			return true
		}
	}
	return false
}

// Blocklist holds time-limited blocks. Safe for concurrent use.
type Blocklist struct {
	mu  sync.RWMutex
	m   map[string]Entry
	Now func() time.Time
}

func NewBlocklist() *Blocklist { return &Blocklist{m: map[string]Entry{}, Now: time.Now} }

// Block adds or extends a block.
func (b *Blocklist) Block(key, reason string, ttl time.Duration) Entry {
	now := b.Now()
	e := Entry{Key: key, Reason: reason, At: now, Until: now.Add(ttl)}
	b.mu.Lock()
	b.m[key] = e
	b.mu.Unlock()
	return e
}

// Unblock removes a block; it reports whether one existed.
func (b *Blocklist) Unblock(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.m[key]
	delete(b.m, key)
	return ok
}

// Blocked reports whether key is currently blocked (expired blocks do not count).
func (b *Blocklist) Blocked(key string) bool {
	b.mu.RLock()
	e, ok := b.m[key]
	b.mu.RUnlock()
	return ok && b.Now().Before(e.Until)
}

// List returns the active blocks, sorted by key, and drops expired ones.
func (b *Blocklist) List() []Entry {
	now := b.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, 0, len(b.m))
	for k, e := range b.m {
		if !now.Before(e.Until) {
			delete(b.m, k)
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Containment records one automatic containment, for measurement.
type Containment struct {
	Key         string        // what was blocked
	FirstBad    time.Time     // first bad request in the window that triggered it
	Detected    time.Time     // when the threshold was crossed (= when the block took effect)
	BadRequests int           // bad requests seen before containment
	TimeToBlock time.Duration // Detected - FirstBad
}

// Guard blocks a client IP that sends Threshold invalid events within Window.
type Guard struct {
	BL        *Blocklist
	Threshold int
	Window    time.Duration
	TTL       time.Duration
	OnContain func(Containment) // optional hook (metrics, logs, tests)

	mu   sync.Mutex
	hits map[string][]time.Time
}

func NewGuard(bl *Blocklist, threshold int, window, ttl time.Duration) *Guard {
	if threshold < 1 {
		threshold = 1
	}
	return &Guard{BL: bl, Threshold: threshold, Window: window, TTL: ttl, hits: map[string][]time.Time{}}
}

// Bad records an invalid request from ip and blocks it once the threshold is crossed.
// It returns true when this call caused the block.
func (g *Guard) Bad(ip string) bool {
	if ip == "" {
		return false
	}
	now := g.BL.Now()
	g.mu.Lock()
	h := g.hits[ip]
	cut := 0
	for cut < len(h) && now.Sub(h[cut]) > g.Window {
		cut++
	}
	h = append(h[cut:], now)
	if len(h) < g.Threshold {
		g.hits[ip] = h
		g.mu.Unlock()
		return false
	}
	delete(g.hits, ip)
	g.mu.Unlock()
	key := "ip:" + ip
	g.BL.Block(key, "guard: invalid-event flood", g.TTL)
	if g.OnContain != nil {
		g.OnContain(Containment{Key: key, FirstBad: h[0], Detected: now, BadRequests: len(h), TimeToBlock: now.Sub(h[0])})
	}
	return true
}
