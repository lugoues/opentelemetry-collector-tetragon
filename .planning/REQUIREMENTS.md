# Requirements: OTel Collector Tetragon Receiver

**Defined:** 2026-03-18
**Core Value:** Events flow from Tetragon to the OTel Collector pipeline without filesystem coupling

## v1 Requirements

### Receiver Core

- [x] **RECV-01**: Receiver registers as a logs receiver via `receiver.NewFactory` with `receiver.WithLogs`
- [x] **RECV-02**: Receiver implements `Start()` that connects to Tetragon gRPC endpoint and spawns stream goroutine without blocking
- [x] **RECV-03**: Receiver implements `Shutdown()` that cancels stream context, waits for goroutine exit, and closes gRPC connection
- [x] **RECV-04**: Receiver streams events via `FineGuidanceSensors.GetEvents` server-streaming RPC
- [x] **RECV-05**: Receiver reconnects with exponential backoff on stream errors (configurable initial_interval, max_interval, max_elapsed_time)
- [x] **RECV-06**: Receiver distinguishes clean shutdown (context cancelled) from transient errors (retry)
- [x] **RECV-07**: Receiver logs connection events, errors, and reconnects via zap structured logging
- [x] **RECV-08**: Receiver reports internal telemetry via obsreport (accepted/refused log records)

### Configuration

- [x] **CONF-01**: Config struct with endpoint, TLS, and retry settings using `configgrpc.ClientConfig`
- [x] **CONF-02**: Config validates at startup (empty endpoint, invalid TLS paths fail fast)
- [x] **CONF-03**: Default config: endpoint `localhost:54321`, insecure true, reasonable backoff defaults
- [x] **CONF-04**: Config is YAML-configurable via standard OTel Collector config file

### Event Conversion

- [x] **CONV-01**: Each `GetEventsResponse` becomes one `plog.LogRecord` with scope name `tetragonreceiver`
- [x] **CONV-02**: Log body contains full JSON via `protojson.Marshal` matching Tetragon's own JSON export format
- [x] **CONV-03**: Timestamp set from event's time field (not wall clock); ObservedTimestamp set to receive time
- [x] **CONV-04**: Severity mapped per event type: INFO for exec/exit/loader, WARN for kprobe/tracepoint/lsm with action, ERROR for throttle/rate_limit
- [x] **CONV-05**: Static attributes set: `event.domain`=`tetragon`, `event.name`=event type string
- [x] **CONV-06**: Process attributes extracted: binary, arguments, pid, uid, exec_id, cwd
- [x] **CONV-07**: Parent process attributes extracted when present: binary, pid, exec_id
- [x] **CONV-08**: Event-specific attributes extracted: policy_name, action, function_name (kprobe), subsys/event (tracepoint), exit status/signal
- [x] **CONV-09**: Kubernetes attributes extracted when pod info present: k8s.namespace.name, k8s.pod.name, k8s.container.name
- [x] **CONV-10**: All Tetragon event types handled: exec, exit, kprobe, tracepoint, loader, uprobe, lsm, usdt, throttle, rate_limit_info

### Distribution

- [x] **DIST-01**: OCB builder-config.yaml produces custom collector with tetragonreceiver + journald, batch, resourcedetection, otlphttp, health_check, file_storage
- [x] **DIST-02**: Top-level go.mod with replaces directive for local receiver module
- [ ] **DIST-03**: Multi-stage Containerfile: Go builder with OCB → Debian-slim runtime with systemd and ca-certificates
- [ ] **DIST-04**: Container runs as non-root user (otel:10001) with systemd-journal group membership
- [ ] **DIST-05**: Container image is drop-in replacement: same entrypoint, config path, runtime user as current otelcol-contrib image

### CI/CD

- [ ] **CICD-01**: GitHub Actions workflow: test → build → push to GHCR
- [ ] **CICD-02**: PR builds run tests and build image without pushing
- [ ] **CICD-03**: Main branch pushes tag with `latest` + `sha-<commit>`
- [ ] **CICD-04**: Semver tags produce versioned image tags (v1.2.3 → 1.2.3, 1.2, latest)
- [ ] **CICD-05**: Multi-arch build: linux/amd64 and linux/arm64

### Project Setup

- [x] **PROJ-01**: mise.toml with Go, OCB, and project tasks (build, test, lint)
- [x] **PROJ-02**: Example collector config in rootfs/etc/otelcol/config.yaml
- [x] **PROJ-03**: metadata.yaml declaring receiver type and alpha stability for logs signal
- [ ] **PROJ-04**: README with usage, configuration reference, and build instructions

## v2 Requirements

### Filtering

- **FILT-01**: Server-side event filtering via allow_list/deny_list in receiver config
- **FILT-02**: Filter config maps to Tetragon GetEventsRequest Filter messages

### Observability

- **OBSV-01**: Metrics pipeline from Tetragon events (process exec rate, kprobe trigger counts) via connector component

## Out of Scope

| Feature | Reason |
|---------|--------|
| Server-side event filtering (v1) | Pipeline filter/transform processors handle this; adds config complexity |
| Metrics signal from events | Requires separate connector component design; noted for v2 |
| Event buffering/WAL in receiver | OTel framework handles this at exporter layer via file_storage |
| HTTP polling fallback | Tetragon has no HTTP events endpoint |
| Custom protobuf parsing | protojson.Marshal on official types is only safe path for format compatibility |
| Log record per field (exploded events) | Destroys atomic event boundary; breaks downstream correlation |
| Mobile/desktop clients | N/A — this is infrastructure tooling |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| RECV-01 | Phase 1 | Complete |
| RECV-02 | Phase 1 | Complete |
| RECV-03 | Phase 1 | Complete |
| RECV-04 | Phase 1 | Complete |
| RECV-05 | Phase 1 | Complete |
| RECV-06 | Phase 1 | Complete |
| RECV-07 | Phase 1 | Complete |
| RECV-08 | Phase 1 | Complete |
| CONF-01 | Phase 1 | Complete |
| CONF-02 | Phase 1 | Complete |
| CONF-03 | Phase 1 | Complete |
| CONF-04 | Phase 1 | Complete |
| CONV-01 | Phase 1 | Complete |
| CONV-02 | Phase 1 | Complete |
| CONV-03 | Phase 1 | Complete |
| CONV-04 | Phase 1 | Complete |
| CONV-05 | Phase 1 | Complete |
| CONV-06 | Phase 1 | Complete |
| CONV-07 | Phase 1 | Complete |
| CONV-08 | Phase 1 | Complete |
| CONV-09 | Phase 1 | Complete |
| CONV-10 | Phase 1 | Complete |
| DIST-01 | Phase 2 | Complete |
| DIST-02 | Phase 2 | Complete |
| DIST-03 | Phase 2 | Pending |
| DIST-04 | Phase 2 | Pending |
| DIST-05 | Phase 2 | Pending |
| PROJ-01 | Phase 1 | Complete |
| PROJ-02 | Phase 2 | Complete |
| PROJ-03 | Phase 1 | Complete |
| PROJ-04 | Phase 3 | Pending |
| CICD-01 | Phase 3 | Pending |
| CICD-02 | Phase 3 | Pending |
| CICD-03 | Phase 3 | Pending |
| CICD-04 | Phase 3 | Pending |
| CICD-05 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 32 total
- Mapped to phases: 32
- Unmapped: 0 ✓

---
*Requirements defined: 2026-03-18*
*Last updated: 2026-03-18 after initial definition*
