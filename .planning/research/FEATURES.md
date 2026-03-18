# Feature Research

**Domain:** Custom OTel Collector receiver — gRPC streaming log ingestion (Tetragon events to OTLP)
**Researched:** 2026-03-18
**Confidence:** HIGH (OTel receiver SDK well-documented; Tetragon gRPC API stable v1.x)

## Feature Landscape

### Table Stakes (Users Expect These)

Features operators assume exist in any production OTel Collector receiver. Missing these = receiver is
unusable or untrustworthy.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `receiver.Logs` factory registration | The receiver cannot participate in any OTel pipeline without registering a typed factory via `receiver.NewFactory()` + `receiver.WithLogs()`. Non-negotiable SDK contract. | LOW | `factory.go` + `config.go` + `receiver.go` three-file minimum module structure |
| `Start()` / `Shutdown()` lifecycle | `component.Component` interface is mandatory. Collectors call these to manage receiver state. Missing Shutdown = goroutine leaks on reload. | LOW | Must cancel background goroutine context; drain in-flight events before returning |
| gRPC connection to Tetragon | Core purpose: connect to `FineGuidanceSensors.GetEvents` at `localhost:54321`. Without this there is no receiver. | MEDIUM | Use `configgrpc.ClientConfig` from the OTel SDK for standard config surface; `insecure: true` as default for localhost |
| Stream consumption loop | `GetEvents` returns a server-streaming RPC. The receiver must read `GetEventsResponse` messages in a loop and push each to the downstream `consumer.Logs`. | MEDIUM | Must handle `io.EOF` and gRPC status errors distinctly |
| Reconnection with exponential backoff | Tetragon restarts; network blips happen. A receiver that stops on first stream error is unusable in production. Users expect "it just reconnects". | MEDIUM | Implement `retry/exponential` with jitter; respect `Shutdown()` cancellation signal; configurable `initial_interval`, `max_interval`, `max_elapsed_time` |
| OTLP log record construction | Every event must be emitted as a `plog.LogRecord`. Missing this means no data exits the receiver. | MEDIUM | Use `pdata` API: `plog.NewLogs()` → `ResourceLogs` → `ScopeLogs` → `LogRecord` |
| Full JSON body (protojson.Marshal) | Downstream consumers (OpenObserve) query against the JSON body. Body format must match Tetragon's own JSON export exactly. Different format = broken queries. | MEDIUM | `protojson.Marshal` on the `GetEventsResponse` wrapper to preserve field naming and nested structure |
| All Tetragon event types handled | Users expect exec, exit, kprobe, tracepoint, loader, uprobe, lsm, usdt, throttle, rate_limit_info to appear. Silently dropping unknown event types is a data loss bug. | MEDIUM | Switch on `GetEventsResponse.Event` oneof; use `protojson.Marshal` to avoid hand-mapping each type |
| Kubernetes attributes extracted | When pod info is present on an event, attributes like `k8s.pod.name`, `k8s.namespace.name`, `k8s.pod.uid` must be set. Without this, Kubernetes-based routing/filtering in downstream pipelines breaks. | MEDIUM | Inspect `process.pod` field on each event; conditionally set resource or log attributes |
| TLS configuration | Operators must be able to configure mTLS for non-localhost Tetragon endpoints. Using `configtls.ClientConfig` satisfies this without custom code. | LOW | Embed `configgrpc.ClientConfig` in the receiver config struct; `insecure: true` default |
| Config validation (`Validate()`) | The OTel framework calls `Validate()` at startup. Invalid config (empty endpoint, negative timeout) must fail fast with a clear error, not panic at runtime. | LOW | Implement `component.ConfigValidator` interface on the Config struct |
| Default config via `CreateDefaultConfig` | Factory must return a sensible default config. Without defaults, the receiver cannot be started with minimal YAML (required by OCB distributions). | LOW | Endpoint: `localhost:54321`, insecure: true, reasonable backoff defaults |
| Structured logging via zap | The OTel framework provides a `zap.Logger` to components. Receivers that use `fmt.Printf` are incompatible with collector log levels and structured output. | LOW | Accept `zap.Logger` in factory `CreateLogsReceiver`; use it for connection events, errors, reconnects |
| Context propagation and cancellation | All blocking calls (gRPC Dial, stream Read, backoff sleeps) must respect the context passed through `Start()`/`Shutdown()`. Missing this = unresponsive shutdown. | LOW | Thread `ctx` through; use `select` on `ctx.Done()` in reconnect loop |
| Internal telemetry (obsreport) | The collector framework tracks `otelcol_receiver_accepted_log_records` and `otelcol_receiver_refused_log_records`. Without obsreport instrumentation, the receiver is invisible to pipeline health monitoring. | LOW | Use `receiver.NewObsReport()` and call `StartLogsOp` / `EndLogsOp` around each `ConsumeLogs` call |

### Differentiators (Competitive Advantage)

Features that distinguish this receiver from a minimal working implementation, or from the file-based
alternative it replaces.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| No filesystem coupling | Eliminates the ACL complexity, shared volume mounts, and `root:600` permission problem of the current filelog approach. Pure network — no shared disk state. | LOW | Inherent to using gRPC streaming; no extra code required beyond the core connection |
| `event_type` attribute extracted | Setting `tetragon.event_type` (e.g. `process_exec`) as a log attribute enables zero-cost filtering in downstream processors without parsing JSON body. | LOW | Inspect `GetEventsResponse.Event` oneof to determine type before `protojson.Marshal` |
| Process identity attributes extracted | `process.pid`, `process.executable.name`, `process.executable.path` extracted as first-class OTLP attributes enable indexed lookup in OpenObserve without JSON parsing. | MEDIUM | Map from `process_exec.process` fields; define a fixed attribute mapping table |
| Timestamp precision preserved | Tetragon events carry nanosecond-precision timestamps. Setting `LogRecord.Timestamp` from the event's `time` field (not the collector's wall clock) preserves forensic ordering. | LOW | Parse `GetEventsResponse.time` field → set `plog.LogRecord.SetTimestamp()` |
| Severity mapping | Map Tetragon event type to OTLP `SeverityNumber` (e.g. `process_kprobe` = `WARN`, `rate_limit_info` = `INFO`) so downstream severity-based routing works. | LOW | Simple lookup table; `process_exec`/`exit` → INFO, kprobe/lsm → WARN |
| `allow_list` / `deny_list` event type filtering | Pre-pipeline server-side filtering reduces volume before data enters the collector. Explicitly deferred to post-v1 per PROJECT.md constraints. | HIGH | Tetragon `GetEventsRequest` supports `allow_list` filter; complexity is in the config schema and test surface |
| Keepalive configuration exposed | Exposing `configgrpc.ClientConfig.Keepalive` lets operators tune gRPC keepalives for long-lived streaming connections through load balancers or firewalls that kill idle TCP connections. | LOW | Comes for free by embedding `configgrpc.ClientConfig` in receiver config |
| Reconnect jitter configurable | Operators in high-restart environments (rolling Tetragon upgrades) may want to tune backoff parameters. Exposing `initial_interval`, `max_interval`, `multiplier` in config avoids hardcoded behavior. | LOW | Add `BackoffConfig` struct to receiver config; wire to reconnect loop |
| `metadata.yaml` stability declaration | Required to contribute to `opentelemetry-collector-contrib`. Declares stability level per signal, code owners, and distributions. Enables automated lifecycle tests. | LOW | Boilerplate file; sets stability to `development` initially |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Server-side event filtering via `allow_list`/`deny_list` config | Reduce data volume before it enters the pipeline | Adds config schema complexity, requires Tetragon API filter struct mapping, and must be tested against all event types. For v1, the pipeline's `filter` processor handles this without receiver changes. Explicitly out of scope in PROJECT.md. | Use OTel Collector `filter` processor or `transform` processor in the pipeline |
| Metrics signal output from events | Derive counters (exec count per namespace) from events inside the receiver | Receivers in OTel are single-signal by convention. Mixing logs + metrics in one receiver creates a non-standard component that fails standard pipeline wiring. Noted as future work in PROJECT.md. | Separate `connector` component that reads from the logs pipeline and emits metrics |
| Event buffering / WAL in receiver | Buffer events locally if downstream is slow or unavailable | The OTel framework provides this at the exporter layer via `file_storage` + sending queue. Duplicating it in the receiver creates two storage layers with split backpressure semantics. | Configure `file_storage` extension + exporter retry queue in the collector pipeline |
| HTTP polling fallback | Fall back to polling a Tetragon HTTP endpoint if gRPC fails | Tetragon does not expose an HTTP events endpoint. This would require a separate code path with no upstream support and would not be a reliable fallback. | Fix the gRPC connection; implement proper reconnect logic |
| Custom protobuf parsing (hand-written) | Avoid importing the Tetragon API module | `protojson.Marshal` on the official `github.com/cilium/tetragon/api/v1/tetragon` types is the only safe way to guarantee format compatibility with Tetragon's own JSON export. Hand-parsing breaks on every Tetragon release. | Import and use the official Tetragon API module |
| Log record per field (exploded events) | One OTLP log record per top-level event field | Destroys the atomic event boundary. Downstream consumers receive partial events and cannot correlate. Queries against `process.pid` fail if the pid record arrives after the arguments record. | One log record per Tetragon event; use attributes for indexed fields, body for full JSON |

## Feature Dependencies

```
[gRPC connection to Tetragon]
    └──requires──> [TLS configuration (configgrpc.ClientConfig)]
    └──requires──> [Config validation (Validate())]
    └──requires──> [Default config (CreateDefaultConfig)]

[Stream consumption loop]
    └──requires──> [gRPC connection to Tetragon]
    └──requires──> [Context propagation and cancellation]
    └──requires──> [Reconnection with exponential backoff]

[OTLP log record construction]
    └──requires──> [Stream consumption loop]
    └──requires──> [Full JSON body (protojson.Marshal)]

[Kubernetes attributes extracted]
    └──requires──> [OTLP log record construction]

[event_type attribute extracted]
    └──requires──> [Stream consumption loop]
    └──enhances──> [OTLP log record construction]

[Process identity attributes extracted]
    └──requires──> [OTLP log record construction]
    └──enhances──> [event_type attribute extracted]

[Timestamp precision preserved]
    └──requires──> [OTLP log record construction]

[Severity mapping]
    └──requires──> [event_type attribute extracted]

[Internal telemetry (obsreport)]
    └──requires──> [Stream consumption loop]
    └──enhances──> [receiver.Logs factory registration]

[Reconnection with exponential backoff]
    ──conflicts──> [Event buffering / WAL in receiver]
    (backoff is the receiver's responsibility; WAL is the exporter's responsibility)
```

### Dependency Notes

- **Stream consumption loop requires reconnection**: Without reconnect logic, the receiver is single-use. Any Tetragon restart causes permanent data loss.
- **Kubernetes attributes require OTLP log record construction**: Attribute extraction happens during record assembly, not during stream reading.
- **Severity mapping requires event_type extraction**: The event type must be determined before severity can be assigned.
- **Internal telemetry (obsreport) enhances factory registration**: The `ObsReport` object is created once at factory time and used per-batch in the stream loop. It cannot be added as an afterthought without touching the factory.
- **allow_list/deny_list conflicts with filter processor**: They address the same problem at different layers. Building both creates maintenance surface with no incremental value for v1.

## MVP Definition

### Launch With (v1)

Minimum viable to replace the filelog receiver and validate the gRPC approach.

- [ ] `receiver.Logs` factory + `Start()`/`Shutdown()` lifecycle — SDK contract; nothing works without it
- [ ] gRPC connection to `FineGuidanceSensors.GetEvents` — core purpose
- [ ] Stream consumption loop with context cancellation — events must flow
- [ ] Reconnection with exponential backoff — required for any unattended production use
- [ ] `protojson.Marshal` JSON body — preserves OpenObserve query compatibility
- [ ] All Tetragon event types passed through (no silent drops) — completeness guarantee
- [ ] `event_type` attribute extracted — enables downstream filtering without JSON parsing
- [ ] Kubernetes attributes extracted when pod info present — enables K8s-based routing
- [ ] Timestamp from event (not wall clock) — forensic accuracy
- [ ] TLS config via `configgrpc.ClientConfig` (insecure default) — operator flexibility
- [ ] Config validation — fail fast at startup, not mid-stream
- [ ] Structured logging via zap — framework-compatible observability
- [ ] Internal telemetry via obsreport — pipeline health visibility
- [ ] OCB-built distribution with standard contrib components — drop-in replacement goal

### Add After Validation (v1.x)

- [ ] Severity mapping per event type — add once confirmed events reach OpenObserve; low risk, low value until query patterns are established
- [ ] Process identity attributes (`process.pid`, `process.executable.path`) — add when operators request indexed process queries; requires testing attribute cardinality
- [ ] Configurable reconnect backoff parameters — add when operators report need to tune for their environment; defaults are reasonable for v1
- [ ] `metadata.yaml` + lifecycle tests for contrib submission — add if/when submitting to upstream contrib; not needed for private OCB distribution

### Future Consideration (v2+)

- [ ] Server-side `allow_list`/`deny_list` filtering — defer until volume reduction at source becomes a measured need; pipeline processors solve this for v1
- [ ] Metrics signal from Tetragon events — defer until a `connector` architecture is validated; requires separate component design
- [ ] mTLS client certificate support beyond CA verification — defer until remote (non-localhost) Tetragon deployments are a use case

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| gRPC connection + stream loop | HIGH | MEDIUM | P1 |
| Reconnection with exponential backoff | HIGH | MEDIUM | P1 |
| protojson.Marshal JSON body | HIGH | LOW | P1 |
| All event types handled | HIGH | LOW | P1 |
| Kubernetes attributes extracted | HIGH | MEDIUM | P1 |
| Timestamp from event | HIGH | LOW | P1 |
| event_type attribute | HIGH | LOW | P1 |
| TLS / configgrpc.ClientConfig | HIGH | LOW | P1 |
| receiver.Logs factory + lifecycle | HIGH | LOW | P1 |
| Config validation + defaults | HIGH | LOW | P1 |
| Structured logging (zap) | MEDIUM | LOW | P1 |
| Internal telemetry (obsreport) | MEDIUM | LOW | P1 |
| Severity mapping | MEDIUM | LOW | P2 |
| Process identity attributes | MEDIUM | LOW | P2 |
| Configurable backoff params | LOW | LOW | P2 |
| metadata.yaml for contrib | LOW | LOW | P2 |
| allow_list/deny_list filtering | MEDIUM | HIGH | P3 |
| Metrics signal from events | LOW | HIGH | P3 |

**Priority key:**
- P1: Must have for v1 launch
- P2: Should have, add after validation
- P3: Nice to have, future consideration

## Competitor / Reference Analysis

This receiver has no direct competitors — no existing Tetragon OTel receiver exists (cilium/tetragon#1419
open 3+ years). The reference implementations are other streaming gRPC receivers in the contrib repo.

| Feature | filelog receiver (current) | OTLP receiver (reference) | This receiver |
|---------|--------------------------|--------------------------|---------------|
| Filesystem dependency | YES (shared volume, ACLs) | NO | NO |
| Reconnect on upstream failure | N/A (file always present) | N/A (push model) | YES (exponential backoff) |
| Full event fidelity | Depends on Tetragon JSON log config | Native OTLP | YES (protojson.Marshal) |
| Kubernetes attributes | Via resourcedetection processor | Via resource processor | YES (extracted from event payload) |
| Event type routing | Requires JSON parsing in processor | N/A | YES (attribute extracted pre-pipeline) |
| Event filtering | Via transform/filter processor | N/A | Via pipeline processor (v1) |
| Root permission required | YES (root:600 log file) | NO | NO |

## Sources

- [Build a receiver — OpenTelemetry](https://opentelemetry.io/docs/collector/extend/custom-component/receiver/)
- [Receiver package — pkg.go.dev](https://pkg.go.dev/go.opentelemetry.io/collector/receiver)
- [configgrpc package — pkg.go.dev](https://pkg.go.dev/go.opentelemetry.io/collector/config/configgrpc)
- [configtls README — opentelemetry-collector GitHub](https://github.com/open-telemetry/opentelemetry-collector/blob/main/config/configtls/README.md)
- [Component stability levels — opentelemetry-collector GitHub](https://github.com/open-telemetry/opentelemetry-collector/blob/main/docs/component-stability.md)
- [OTel Logs Data Model — opentelemetry.io](https://opentelemetry.io/docs/specs/otel/logs/data-model/)
- [Tetragon gRPC API reference — tetragon.io](https://tetragon.io/docs/reference/grpc-api/)
- [Tetragon event types — tetragon.io](https://tetragon.io/docs/concepts/events/)
- [Collector resiliency — opentelemetry.io](https://opentelemetry.io/docs/collector/resiliency/)
- [How to Build a Custom Receiver — oneuptime.com (Feb 2026)](https://oneuptime.com/blog/post/2026-02-06-build-custom-receiver-opentelemetry-collector/view)

---
*Feature research for: Custom OTel Collector Tetragon gRPC streaming log receiver*
*Researched: 2026-03-18*
