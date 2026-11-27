# ADR 0002: Graceful drain on shutdown

Status: Accepted · Recorded 2026-11-26, retrospectively, from the design as built

## Context
Deployments and restarts send SIGTERM. Exiting immediately loses events that were accepted (202) but not yet published.

## Decision
On SIGTERM the gateway stops accepting (503), finishes in-flight requests, then drains the queue to the publisher before exiting.

## Consequences
An accepted event is published even across a deploy. Shutdown takes as long as the queue takes to drain, so the orchestrator's grace period must exceed it.

## Alternatives considered
Exit immediately (rejected: loses accepted events); persist the queue to disk (deferred: added complexity for a stateless edge service).
