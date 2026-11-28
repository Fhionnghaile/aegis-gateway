// Command gateway: HTTP ingest front door -> validate+normalise -> queue -> Publisher.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Fhionnghaile/aegis-gateway/internal/config"
	"github.com/Fhionnghaile/aegis-gateway/internal/contain"
	"github.com/Fhionnghaile/aegis-gateway/internal/event"
	"github.com/Fhionnghaile/aegis-gateway/internal/httpapi"
	"github.com/Fhionnghaile/aegis-gateway/internal/ingest"
	"github.com/Fhionnghaile/aegis-gateway/internal/metrics"
)

func main() {
	cfg := config.FromEnv()
	val, err := event.NewSchemaValidator("schemas/event.schema.json")
	if err != nil {
		log.Fatalf("validator: %v", err)
	}
	m := &metrics.Metrics{}
	q := ingest.NewQueue(cfg.QueueCap)

	// Default sink is in-memory; build with -tags kafka to use Kafka (see publish/kafka.go).
	pub := newPublisher(cfg)
	pool := ingest.NewPool(q, pub, cfg.Workers, m)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	pool.Start(ctx)

	srv := httpapi.New(val, q, m)
	bl := contain.NewBlocklist()
	var guard *contain.Guard
	if cfg.GuardThreshold > 0 {
		guard = contain.NewGuard(bl, cfg.GuardThreshold, time.Duration(cfg.GuardWindowSec)*time.Second, time.Duration(cfg.BlockTTLSec)*time.Second)
		guard.OnContain = func(c contain.Containment) {
			log.Printf("contained %s after %d invalid events in %s", c.Key, c.BadRequests, c.TimeToBlock)
		}
	}
	srv.WithContainment(bl, guard)
	hs := httpapi.NewHTTPServer(cfg.Addr, srv.Handler())
	admin := httpapi.NewHTTPServer(cfg.AdminAddr, srv.AdminHandler())
	go func() {
		log.Printf("gateway listening on %s (workers=%d cap=%d)", cfg.Addr, cfg.Workers, cfg.QueueCap)
		if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http: %v", err)
		}
	}()
	go func() {
		log.Printf("metrics (internal) on %s", cfg.AdminAddr)
		if err := admin.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("admin http: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("SIGTERM: draining...")
	srv.SetReady(false) // stop accepting new events (503)
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = hs.Shutdown(shutCtx) // finish in-flight requests
	_ = admin.Shutdown(shutCtx)
	pool.Stop() // drain the queue into the publisher
	_ = pub.Close()
	log.Println("drained; bye")
}
