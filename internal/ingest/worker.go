package ingest

import (
	"context"
	"sync"

	"github.com/Fhionnghaile/aegis-gateway/internal/metrics"
	"github.com/Fhionnghaile/aegis-gateway/internal/publish"
)

// Pool drains the queue into the Publisher with N workers. On Stop it drains
// whatever is left (graceful) before returning.
type Pool struct {
	q    *Queue
	pub  publish.Publisher
	n    int
	m    *metrics.Metrics
	wg   sync.WaitGroup
}

func NewPool(q *Queue, pub publish.Publisher, n int, m *metrics.Metrics) *Pool {
	if n < 1 {
		n = 1
	}
	return &Pool{q: q, pub: pub, n: n, m: m}
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.n; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for e := range p.q.C() {
				if err := p.pub.Publish(ctx, e); err == nil && p.m != nil {
					p.m.Published.Add(1)
				}
			}
		}()
	}
}

// Stop closes the queue and waits for workers to drain the remainder.
func (p *Pool) Stop() {
	p.q.Close()
	p.wg.Wait()
}
