# ADR 0001: Bounded ingest queue that sheds load with 429

Status: Accepted · Recorded 2026-11-26, retrospectively, from the design as built

## Context
An ingest gateway under a flood must either buffer, drop silently, or refuse. Unbounded buffering converts a traffic spike into memory exhaustion and a crash, which blinds detection at exactly the wrong moment.

## Decision
Events pass validation into a fixed-size queue drained by a worker pool. When the queue is full the gateway answers 429 Too Many Requests immediately.

## Consequences
Clients see explicit backpressure and can retry; memory stays flat under load (the k6 test watches 2xx versus 429). Events are refused, never silently lost after acceptance. Capacity becomes a number to tune (queue size, workers), not an accident.

## Alternatives considered
Unbounded channel (rejected: memory exhaustion); drop-oldest ring buffer (rejected: silent loss of accepted events).
