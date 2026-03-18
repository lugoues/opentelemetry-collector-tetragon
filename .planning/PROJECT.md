# OTel Collector Tetragon Receiver

## What This Is

A custom OpenTelemetry Collector distribution with a native Tetragon gRPC receiver. Ships as a distroless container image (via GHCR) containing a custom OTel Collector built with OCB, streaming all 10 Tetragon event types as OTel LogRecords with full protojson body, extracted attributes, and severity mapping.

## Core Value

Events flow from Tetragon to the OTel Collector pipeline without filesystem coupling — no shared volumes, no ACLs, no disk I/O for event transfer.

## Requirements

### Validated

- ✓ Custom OTel Collector receiver consuming Tetragon gRPC stream — v1.0
- ✓ Event-to-OTLP log record mapping with full JSON body + extracted attributes — v1.0
- ✓ Reconnection with exponential backoff on stream errors — v1.0
- ✓ TLS configuration support (insecure by default for localhost) — v1.0
- ✓ OCB-built custom collector distribution — v1.0
- ✓ Multi-arch container image (amd64/arm64) published to GHCR — v1.0
- ✓ Drop-in replacement for current otelcol-contrib image — v1.0
- ✓ CI pipeline: test → build → push — v1.0
- ✓ All Tetragon event types mapped (exec, exit, kprobe, tracepoint, loader, uprobe, lsm, usdt, throttle, rate_limit) — v1.0
- ✓ Kubernetes attributes extracted when pod info present — v1.0
- ✓ Compatible JSON body format (protojson.Marshal matches Tetragon's JSON export) — v1.0

### Active

(None — define with `/gsd:new-milestone`)

### Out of Scope

- Server-side event filtering via allow_list/deny_list config — deferred to post-v1
- Metrics pipeline from Tetragon events — noted for future
- Mobile/desktop clients — N/A
- Real-time dashboard — consumers use OpenObserve

## Context

Shipped v1.0 with 1,073 LOC Go across 3 phases.
Tech stack: Go 1.25, OTel Collector SDK v1.54.0/v0.148.0, Tetragon API v1.6.0, OCB, distroless container.
Downstream: OpenObserve at :5080 via OTLP/HTTP — protojson body with UseProtoNames preserves query compatibility.

## Constraints

- **Tech stack**: Go, OTel Collector SDK, gRPC — dictated by ecosystem
- **Compatibility**: JSON body must match Tetragon's own JSON export (protojson.Marshal with UseProtoNames)
- **Runtime**: Distroless static base image (gcr.io/distroless/static-debian12), non-root (UID 1000)
- **Build**: OCB (OpenTelemetry Collector Builder) for custom distribution
- **Tooling**: mise for tool/task management

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| gRPC streaming over file tailing | Eliminates filesystem coupling, ACL complexity, and disk I/O for event transfer | ✓ Good |
| Full JSON body + extracted attributes | Preserves all nested data while enabling efficient filtering | ✓ Good |
| protojson.Marshal with UseProtoNames | Matches Tetragon's own JSON format (snake_case) for OpenObserve query compatibility | ✓ Good |
| OCB for collector build | Standard approach for custom OTel Collector distributions | ✓ Good |
| Distroless container | Minimal attack surface, no shell, small image | ✓ Good |
| Removed journaldreceiver | Out of scope for a Tetragon gRPC receiver — kept distribution focused | ✓ Good |
| backoff/v5 retry forever | Stream reconnection should never give up — supervisor handles process lifecycle | ✓ Good |
| Buffered channel (1000) between Recv and ConsumeLogs | Prevents gRPC backpressure stall when ConsumeLogs is slow | ✓ Good |
| Go 1.25 (not 1.24) | Required by tetragon/api v1.6.0 and OTel v1.54.0 minimum | ✓ Good |

---
*Last updated: 2026-03-18 after v1.0 milestone*
