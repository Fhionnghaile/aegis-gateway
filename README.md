# Aegis Gateway (Go)

<!-- at-a-glance -->
> **A high-throughput ingest gateway for security events, backpressure-safe under load.**  
> **Result:** 26 Go tests across 11 files, race-clean; **automated containment measured end to end**: a detection becomes a gateway block in about 1 ms on loopback, with 0 of 201 hostile requests let through afterwards. A k6 load test and an injected-attack check run in GitHub Actions on every push (`Stack` workflow).  
> **Reproduce:** `go test ./...   # load test: see loadtest/`  
> ![CI](https://github.com/fhionnghaile/aegis-gateway/actions/workflows/ci.yml/badge.svg) [![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/Fhionnghaile/aegis-gateway/badge)](https://scorecard.dev/viewer/?uri=github.com/Fhionnghaile/aegis-gateway)
>
> **What this is worth:** The front door of a detection pipeline: if it drops or stalls under a flood, you are blind exactly when you are under attack.


The ingestion front door for the Aegis pipeline: **HTTP -> validate + normalise -> bounded queue ->
worker pool -> Kafka**, with real back-pressure, graceful drain, and Prometheus metrics.

## Why it's interesting
- **Back-pressure, not blowup.** A bounded queue returns **429** the instant it's full; the service
  sheds load instead of running out of memory. Proven in tests and under k6.
- **Graceful drain.** On `SIGTERM` the gateway stops accepting (503), finishes in-flight requests,
  then drains the queue into Kafka before exiting — no lost events.
- **Observability built in.** `/metrics` exposes received / accepted / rejected / dropped / published, on a separate internal-only listener (default `127.0.0.1:9091`, `AEGIS_ADMIN_ADDR`), so event senders cannot read it.
- **Slow clients cut off.** Every listener sets header, read, write and idle timeouts and a 64 KiB header cap (`httpapi.NewHTTPServer`), tested with a client that never finishes its headers.

## Throughput
`go test -bench=IngestHTTP` measures **~240,000 events/sec** through the validate->enqueue path
(in-process, 2 vCPU) — comfortably past the 50k target. For the end-to-end network + Kafka number and
the Grafana 429 panel, run the load test below.

## Containment: detection to block, automatically (respond, not just detect)
```
aegis-detector --(detection JSON)--> responder --POST /admin/block--> gateway --> 403 for the hostile sender
                                                       gateway guard: 20 invalid events in 10 s --> block that client
```
- **Responder** (`cmd/responder`, `internal/respond`): takes the detector's own output format, maps
  `rule_id` to a containment action through a policy (block the event **source**, or a client **IP**,
  for a set time), acts only on high or critical detections, and returns only once the gateway has
  confirmed the block. It reads detections over HTTP, or as JSON lines on stdin, so it can sit straight
  on the detections topic: `rpk topic consume detections -f '%v\n' | responder -stdin`.
- **Gateway** (`internal/contain`): blocks are time-limited and managed on the internal admin listener
  (`/admin/block`, `/admin/unblock`, `/admin/blocked`), never on the public port. A built-in guard blocks
  a client flooding invalid events without waiting for anyone.

**Measured** (`go test ./internal/respond -run Containment -v`, loopback, 28 Nov 2026):
| Path | Detection to block in force | Hostile requests let through after | Collateral |
|---|--:|--:|---|
| Detector detection, via responder | about 1 ms | 0 of 201 | healthy sensor unaffected |
| Built-in guard (invalid-event flood) | blocked on the 21st request (5 ms from the flood's start) | 0 | none |

The CI `Stack` workflow repeats the detector path against the real containers and posts the numbers
to the job summary. **Limits:** the guard keys on the TCP peer address, so behind a load balancer
(one address for every client) set `AEGIS_GUARD_THRESHOLD=0`; blocking a `source` is only as strong
as the source identity, which is self-asserted until per-sensor authentication lands (threat model F1).

## Run
```bash
go test ./... -race          # unit + race tests (green)
go build ./cmd/gateway       # static binary (default in-memory sink)

# full stack (Redpanda + Prometheus + Grafana):
docker compose up --build
k6 run loadtest/k6-ingest.js  # drive load; watch 429s rise, memory stay flat
# metrics: internal only (Prometheus scrapes gateway:9091)   prometheus: http://localhost:9090   grafana: http://localhost:3000
```

### Kafka sink
The default binary uses an in-memory sink so it builds and tests with zero dependencies. For the real
Kafka publisher:
```bash
go get github.com/segmentio/kafka-go
go build -tags kafka ./cmd/gateway
```
(`internal/publish/kafka.go` + `cmd/gateway/publisher_kafka.go` are behind the `kafka` build tag.)

## Layout
`internal/event` (type + Normalise + schema validation) · `internal/ingest` (bounded queue +
worker pool) · `internal/publish` (Publisher interface; memory + kafka impls) · `internal/httpapi`
(`/events` 202/400/429, `/healthz`; `/metrics` on the admin listener) · `internal/metrics` · `internal/contain` (blocklist + guard) · `internal/respond` + `cmd/responder` (detection to block) · `cmd/gateway` (wiring +
SIGTERM drain).

## Threat model and decisions
- [`docs/THREAT-MODEL.md`](docs/THREAT-MODEL.md): STRIDE with evidence for each mitigation; five gaps found, two fixed (timeouts, internal-only metrics), three open with their fixes.
- [`docs/adr/`](docs/adr/): architecture decision records.

## License
MIT.

## Key decisions

- Go for predictable concurrency and backpressure; bounded queues over unbounded buffering.
- Ingest validated at the edge against the event schema contract, so malformed or hostile input is rejected before it reaches detection.
- Grafana dashboards and a load-test shipped alongside, so capacity is a measurement, not a hope.

## Scope & limits

- Single-node reference; horizontal scale-out (partitioned consumers) is a deployment concern.
- Throughput depends on hardware — reproduce with `loadtest/` on your target.

