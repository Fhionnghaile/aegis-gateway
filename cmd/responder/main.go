// Command responder: turns aegis-detector detections into gateway blocks (ML n20_10).
//
//	HTTP:   POST /detections on AEGIS_RESPONDER_ADDR (default 127.0.0.1:9092)
//	stdin:  responder -stdin    one detection JSON per line, e.g. straight from the detections topic:
//	        rpk topic consume detections -f '%v\n' | responder -stdin
//
// AEGIS_ADMIN_URL is the gateway's internal admin listener (default http://127.0.0.1:9091).
// AEGIS_RESPONDER_POLICY optionally names a JSON file {"rule_id":{"kind":"source|ip","ttl_seconds":900}}.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Fhionnghaile/aegis-gateway/internal/httpapi"
	"github.com/Fhionnghaile/aegis-gateway/internal/respond"
)

func main() {
	stdin := flag.Bool("stdin", false, "read detections (JSON lines) from stdin instead of serving HTTP")
	flag.Parse()
	r := respond.New(env("AEGIS_ADMIN_URL", "http://127.0.0.1:9091"))
	if p := os.Getenv("AEGIS_RESPONDER_POLICY"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			log.Fatalf("policy: %v", err)
		}
		pol := map[string]respond.Action{}
		if err := json.Unmarshal(b, &pol); err != nil {
			log.Fatalf("policy: %v", err)
		}
		r.Policy = pol
	}
	r.OnResult = func(x respond.Result) {
		log.Printf("detection %s (%s) key=%q -> %s in %s", x.Detection.RuleID, x.Detection.Severity, x.Key, x.Outcome, x.Took)
	}
	if *stdin {
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var d respond.Detection
			if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
				log.Printf("skip unreadable line: %v", err)
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			r.Handle(ctx, d)
			cancel()
		}
		return
	}
	addr := env("AEGIS_RESPONDER_ADDR", "127.0.0.1:9092")
	log.Printf("responder on %s -> %s", addr, r.AdminURL)
	hs := httpapi.NewHTTPServer(addr, r.Handler())
	if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
