# OTel Collector Tetragon Receiver

## What This Is

A custom OpenTelemetry Collector receiver that consumes Tetragon security events via gRPC streaming (`FineGuidanceSensors.GetEvents`) instead of the current file-based approach. Ships as a container image (via GHCR) containing a custom OTel Collector distribution built with OCB, including the Tetragon receiver alongside standard contrib components (journald, batch, resourcedetection, otlphttp, health_check, file_storage).

## Core Value

Events flow from Tetragon to the OTel Collector pipeline without filesystem coupling — no shared volumes, no ACLs, no disk I/O for event transfer.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Custom OTel Collector receiver consuming Tetragon gRPC stream
- [ ] Event-to-OTLP log record mapping with full JSON body + extracted attributes
- [ ] Reconnection with exponential backoff on stream errors
- [ ] TLS configuration support (insecure by default for localhost)
- [ ] OCB-built custom collector distribution
- [ ] Multi-arch container image (amd64/arm64) published to GHCR
- [ ] Drop-in replacement for current otelcol-contrib image
- [ ] CI pipeline: test → build → push
- [ ] All Tetragon event types mapped (exec, exit, kprobe, tracepoint, loader, uprobe, lsm, usdt, throttle, rate_limit)
- [ ] Kubernetes attributes extracted when pod info present
- [ ] Compatible JSON body format (protojson.Marshal matches Tetragon's JSON export)

### Out of Scope

- Server-side event filtering via allow_list/deny_list config — deferred to post-v1
- Metrics pipeline from Tetragon events — noted for future
- Mobile/desktop clients — N/A
- Real-time dashboard — consumers use OpenObserve

## Context

- Tetragon exposes gRPC on localhost:54321 via `FineGuidanceSensors.GetEvents`
- Current approach uses `filelog` receiver reading Tetragon's JSON log (root:600 perms, fragile ACLs)
- Open issue cilium/tetragon#1419 (3+ years, no implementation) for OTel integration
- Target OTel Collector version: 0.120.0
- Target Tetragon API: github.com/cilium/tetragon/api v1.x
- Downstream: OpenObserve at :5080 via OTLP/HTTP
- Existing OpenObserve queries work against JSON body — must preserve format compatibility

## Constraints

- **Tech stack**: Go, OTel Collector SDK, gRPC — dictated by ecosystem
- **Compatibility**: JSON body must match Tetragon's own JSON export (protojson.Marshal)
- **Runtime**: Debian-slim base image with systemd (for journald receiver)
- **Build**: OCB (OpenTelemetry Collector Builder) for custom distribution
- **Tooling**: mise for tool/task management, modern Go standards

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| gRPC streaming over file tailing | Eliminates filesystem coupling, ACL complexity, and disk I/O | — Pending |
| Full JSON body + extracted attributes | Preserves all nested data while enabling efficient filtering | — Pending |
| protojson.Marshal for JSON body | Matches Tetragon's own JSON format for query compatibility | — Pending |
| OCB for collector build | Standard approach for custom OTel Collector distributions | — Pending |
| Multi-stage Containerfile | Minimal runtime image with only needed binaries | — Pending |

---
*Last updated: 2026-03-18 after initialization*
