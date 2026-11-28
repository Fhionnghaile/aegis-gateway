# Aegis Gateway: threat model

Method: STRIDE per element, written 2026-11-26 against the code as it stands. Every mitigation names
its evidence; every gap is stated, with its fix. Status: **Mitigated**, **Partial**, **Gap**, **Accepted**.

## What it is, and where the trust boundaries are
```
 event senders ──HTTP──▶ │ gateway: validate+normalise → bounded queue → workers │ ──▶ Kafka ──▶ detection
 (untrusted network)     │ /events  /healthz  /metrics                           │    (trusted zone)
                    boundary 1                                               boundary 2
```
**Assets:** the integrity and completeness of the event stream (detection is only as good as what
arrives), and the gateway's availability under load, which is exactly when an attack is under way.

## STRIDE
| # | Threat | Element | Mitigation (evidence) | Status |
|---|---|---|---|---|
| S1 | Anyone who can reach `/events` can submit events, and `source` is self-asserted, so an attacker can inject false events or impersonate a real sensor | /events | Detection-driven containment: a hostile source or client IP can be blocked within about 1 ms of a detection, and a client flooding invalid events is blocked automatically (`internal/contain`, `internal/respond`, e2e tests). Limits damage; does not prove identity | Partial (containment 28 Nov 2026); prevention still needs F1 |
| T1 | Malformed or hostile payloads reach detection | /events | Field-level validation at the edge implementing the contract in `schemas/event.schema.json` (no schema library), plus normalisation; bad input gets 400 (`internal/event/validate.go`, `TestValidateRejects`, `TestPostEvents`) | Mitigated |
| T2 | Events altered between gateway and Kafka | boundary 2 | Kafka client has no TLS or auth configured | **Gap**, fix F3 |
| R1 | A sender denies having sent an event; no way to attribute | /events | Events carry an ID and `ReceivedAt` (`Normalise`), but there is no authenticated sender identity | Partial, closes with F1 |
| I1 | `/metrics` reveals traffic volumes and rejection rates to anyone | /metrics | Served only on a separate admin listener, default `127.0.0.1:9091`; the public handler returns 404 (`TestMetricsExposedOnAdminListenerOnly`) | Mitigated (F4 done 27 Nov 2026) |
| D1 | Flood exhausts memory | queue | Bounded queue, immediate 429 when full (ADR 0001; `TestBackPressure`, `TestBackPressure429`; k6 harness) | Mitigated |
| D2 | Oversized bodies exhaust memory | /events | `http.MaxBytesReader` caps each body at 1 MiB (`internal/httpapi/server.go`) | Mitigated |
| D3 | Slow clients hold connections open (slowloris) | HTTP server | Header 5 s, read 15 s, write 15 s, idle 60 s timeouts and a 64 KiB header cap on every listener (`httpapi.NewHTTPServer`; `TestHTTPServerHasTimeouts`, `TestSlowHeadersAreCutOff`) | Mitigated (F2 done 27 Nov 2026) |
| D4 | Restart or deploy loses accepted events | shutdown | Stop accepting (503), finish in-flight, drain queue to the publisher (ADR 0002; `TestPoolDrainsOnStop`, `TestHealthAndDrain`) | Mitigated |
| D5 | One noisy sender starves the rest | queue | Invalid-event floods are blocked per client by the guard; a detector `sensor-flood` detection blocks the source. No per-sender quota for *valid* traffic yet | Partial, fix F5 |
| E1 | Compromise via a vulnerable dependency or image | build | Default build has zero third-party Go dependencies; Kafka client only behind a build tag; CI on every push and weekly; OpenSSF Scorecard weekly | Partial |

## Fixes, in priority order
- **F1 (S1, R1):** per-sender authentication: mTLS, or HMAC-signed requests with a per-sensor key; bind `source` to the authenticated identity and reject mismatches.
- ~~**F2 (D3):** server timeouts.~~ Done 27 Nov 2026.
- **F3 (T2):** TLS and SASL on the Kafka connection; ACLs so only the gateway can write the topic.
- ~~**F4 (I1):** `/metrics` on a separate internal-only port.~~ Done 27 Nov 2026.
- **F5 (D5):** per-sender token-bucket rate limit in front of the shared queue, returning 429 with `Retry-After`.

## Out of scope
TLS termination for `/events` (assumed at a load balancer), Kafka cluster hardening, and the detection
engine itself (separate Aegis components).
