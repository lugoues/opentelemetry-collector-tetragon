# Phase 1: Receiver Module - Context

**Gathered:** 2026-03-18
**Status:** Ready for planning

<domain>
## Phase Boundary

A standalone, tested Go module (`receiver/tetragonreceiver/`) that streams Tetragon security events via gRPC (`FineGuidanceSensors.GetEvents`) into an OTel Collector logs pipeline. Covers: factory, config, gRPC stream loop with reconnection, event-to-LogRecord converter, and unit tests. Does NOT include OCB distribution, container image, or CI pipeline.

</domain>

<decisions>
## Implementation Decisions

### Tetragon API Version
- Pin to `github.com/cilium/tetragon/api` v1.6.0 (latest, matches deployed Tetragon)
- No build matrix needed — protobuf wire compatibility means the client works with older/newer daemons (unknown fields are silently ignored)
- The `GetEvents` RPC has remained stable across all releases; breaking changes have been field renames/removals within event payloads, not RPC-level breaks
- Document the pinned version in go.mod comments for future maintainers

### Startup Health Behavior
- `Start()` returns immediately — never blocks waiting for Tetragon connection
- Background goroutine handles connect + stream with exponential backoff retry (retry forever, no startup timeout)
- Report `componentstatus.NewRecoverableErrorEvent(err)` while disconnected or on stream errors
- Report `componentstatus.NewEvent(componentstatus.StatusOK)` when connection succeeds and streaming starts
- The `healthcheckv2extension` automatically aggregates these into HTTP health endpoints for Kubernetes probes
- This matches the OTel convention established by kafkareceiver and others — `Start()` errors are reserved for truly unrecoverable problems (bad config)

### Config Approach
- Use `configgrpc.ClientConfig` with `mapstructure:",squash"` — standard OTel pattern
- This replaces the SPEC's custom Config with `configtls.ClientConfig` (see SPEC-DISCREPANCIES.md)
- Provides out-of-the-box: TLS, keepalive, compression, auth extensions, load balancing, headers, OTel instrumentation
- Use `ToClientConn()` in `Start()` to get a ready `*grpc.ClientConn` — no manual dial option assembly
- Add receiver-specific fields alongside the squashed ClientConfig: `retry` (backoff settings)
- Users get familiar YAML structure matching other OTel components (`endpoint`, `tls.insecure`, etc.)

### Test Data Sourcing
- Capture reference JSON from `tetra getevents -o json` on a live Tetragon instance for all 10 event types
- Check captured JSON into `testdata/events/` as input fixtures (process_exec.json, process_exit.json, etc.)
- Use `pkg/golden` (`ReadLogs`/`WriteLogs`) + `pkg/pdatatest/plogtest.CompareLogs()` for converter output validation against golden YAML files
- Two-layer approach: real Tetragon JSON in -> converter -> compare against golden YAML out
- Document capture command and Tetragon version in testdata/README.md for reproducibility

### Backpressure Handling
- Decouple `Recv()` from `ConsumeLogs()` with a buffered channel (per STATE.md pre-phase decision)
- Buffer size: 1000 events (configurable, sensible default for high-throughput Tetragon streams)
- Overflow behavior: block `Recv()` when buffer is full (backpressure propagates to gRPC stream, which is the correct signal)
- Do not drop events silently — if the pipeline can't keep up, slowing the stream is safer than data loss
- Log a warning when buffer reaches 80% capacity

### gRPC Client Mocking
- Define a narrow Go interface covering only the RPCs we use: `GetEvents` (streaming) and `GetVersion` (unary)
- Mock this interface in tests with a simple struct (or testify/mock) that returns pre-built protobuf response objects
- Do not use bufconn or a real test gRPC server — interface mocking is the OTel contrib pattern
- Test reconnection behavior by having the mock return errors then recover

### Go Version and Tooling
- Use latest stable Go (1.24.x as of March 2026) — update mise.toml accordingly
- mise.toml serves as the canonical tool/task configuration — update it as needed during implementation
- PROJ-01 (mise.toml with Go, OCB, and project tasks) is in scope for this phase

### Lifecycle Details (from STATE.md pre-phase decisions)
- Use `context.WithCancel(context.Background())` inside `Start()` — never pass Start()'s ctx to the background goroutine
- Initialize `r.cancel` to a no-op function in the factory — guards against Shutdown-before-Start
- Capture reference JSON from `tetra getevents --output json` before writing any protojson marshaling code

### Claude's Discretion
- Exact backoff parameters (initial_interval, max_interval defaults)
- Internal code organization within convert.go (helper functions, type switches vs maps)
- Error message wording and zap field choices for structured logging
- Test helper organization and shared test utilities
- Whether to use mdatagen for component metadata generation

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Receiver Specification
- `SPEC.md` -- Full receiver specification including architecture, event-to-OTLP mapping, config struct, lifecycle pseudocode, repo structure, and testing strategy. NOTE: Some details conflict with decisions made here (see SPEC-DISCREPANCIES.md); decisions in this CONTEXT.md take precedence.

### Requirements
- `.planning/REQUIREMENTS.md` -- Phase 1 requirements: RECV-01 through RECV-08, CONF-01 through CONF-04, CONV-01 through CONV-10, PROJ-01, PROJ-03

### Pre-Phase Decisions
- `.planning/STATE.md` section "Accumulated Context > Decisions" -- Pitfall-driven decisions about context handling, cancel initialization, backpressure channel, and reference JSON capture

### SPEC Divergences
- `.planning/phases/01-receiver-module/SPEC-DISCREPANCIES.md` -- Where research-informed decisions override SPEC.md details

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — greenfield project. Only `SPEC.md` and `.mise/config.toml` exist.

### Established Patterns
- No existing Go code to establish patterns. This phase sets the patterns for subsequent phases.
- mise is the tool/task runner (per `.mise/config.toml`). Currently only has `bun` and `go` (outdated version).

### Integration Points
- Phase 2 (Distribution) will consume this module via OCB `replaces` directive pointing to `./receiver/tetragonreceiver`
- The receiver registers via `receiver.NewFactory` with `receiver.WithLogs` — standard OTel component registration
- Downstream pipeline: batch processor -> otlphttp exporter -> OpenObserve

</code_context>

<specifics>
## Specific Ideas

- SPEC.md is the detailed vision document — use it as the primary reference but defer to CONTEXT.md decisions where they diverge
- The `protojson.Marshal` output must match Tetragon's own JSON export for OpenObserve query compatibility — this is the hardest correctness constraint
- mise.toml should be updated freely as tooling needs emerge (per user: "mise.toml is here to serve you")

</specifics>

<deferred>
## Deferred Ideas

- Server-side event filtering via allow_list/deny_list config — explicitly deferred to post-v1 (REQUIREMENTS.md out-of-scope)
- Metrics pipeline from Tetragon events — noted for v2
- Build matrix for multiple Tetragon API versions — not needed given protobuf wire compat, but revisit if field renames cause issues

</deferred>

---

*Phase: 01-receiver-module*
*Context gathered: 2026-03-18*
