# Aegis Gateway (Go)

<!-- at-a-glance -->
> **A high-throughput ingest gateway for security events, backpressure-safe under load.**  
> **Result:** 10 Go tests across 6 files; a load-test harness ships in the repo (`loadtest/`).  
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
- **Observability built in.** `/metrics` exposes received / accepted / rejected / dropped / published.

## Throughput
`go test -bench=IngestHTTP` measures **~240,000 events/sec** through the validate->enqueue path
(in-process, 2 vCPU) — comfortably past the 50k target. For the end-to-end network + Kafka number and
the Grafana 429 panel, run the load test below.

## Run
```bash
go test ./... -race          # unit + race tests (green)
go build ./cmd/gateway       # static binary (default in-memory sink)

# full stack (Redpanda + Prometheus + Grafana):
docker compose up --build
k6 run loadtest/k6-ingest.js  # drive load; watch 429s rise, memory stay flat
# metrics: http://localhost:8080/metrics   grafana: http://localhost:3000
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
(`/events` 202/400/429, `/healthz`, `/metrics`) · `internal/metrics` · `cmd/gateway` (wiring +
SIGTERM drain).

## Threat model and decisions
- [`docs/THREAT-MODEL.md`](docs/THREAT-MODEL.md): STRIDE with evidence for each mitigation, five stated gaps and their fixes.
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

