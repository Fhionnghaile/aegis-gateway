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
	hs := &http.Server{Addr: cfg.Addr, Handler: srv.Handler()}
	go func() {
		log.Printf("gateway listening on %s (workers=%d cap=%d)", cfg.Addr, cfg.Workers, cfg.QueueCap)
		if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("SIGTERM: draining...")
	srv.SetReady(false) // stop accepting new events (503)
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = hs.Shutdown(shutCtx) // finish in-flight requests
	pool.Stop()              // drain the queue into the publisher
	_ = pub.Close()
	log.Println("drained; bye")
}
