# Project Research Summary

**Project:** otel-collector-tetragon — Custom OTel Collector with Tetragon gRPC Receiver
**Domain:** Custom OpenTelemetry Collector receiver (gRPC streaming logs pipeline)
**Researched:** 2026-03-18
**Confidence:** HIGH

## Executive Summary

This project builds a custom OpenTelemetry Collector distribution that replaces a fragile filelog-based approach with a purpose-built gRPC streaming receiver for Tetragon security events. The receiver connects to Tetragon's `FineGuidanceSensors.GetEvents` server-streaming RPC, converts each event to an OTLP `plog.LogRecord` with a `protojson`-serialized JSON body, and forwards it through a standard collector pipeline to OpenObserve. The entire distribution is assembled by the OpenTelemetry Collector Builder (OCB) from a manifest that references the custom receiver module alongside standard contrib components (batch processor, resourcedetection processor, otlphttp exporter, journald receiver).

The recommended approach follows the standard OTel receiver SDK pattern: a `receiver.Factory` registered with `receiver.NewFactory` + `receiver.WithLogs`, an unexported receiver struct implementing `component.Component` (`Start`/`Shutdown`), a background goroutine managing the gRPC stream lifecycle, and a pure `converter.go` function handling `GetEventsResponse` to `plog.Logs` conversion. This pattern is well-documented in official OTel sources, has clear interfaces, and the dual-version scheme (core `v1.26.0` / contrib `v0.120.0`) is stable. The Tetragon API module (`github.com/cilium/tetragon/api@v1.6.0`) provides the generated gRPC client and protobuf types; no upstream OTel Tetragon receiver exists, so this is novel but building on thoroughly documented foundations.

The primary risks are implementation traps in the receiver lifecycle, not architectural uncertainty. Five critical pitfalls require explicit prevention: (1) reusing the `Start()` context in the streaming goroutine — silently kills the stream; (2) OCB `otelcol_version` misalignment — causes cryptic build failures; (3) gRPC error type discrimination in the reconnect loop — causes reconnect storms on shutdown; (4) protobuf dependency version conflicts — causes startup panics; and (5) `protojson.Marshal` option mismatch — breaks existing OpenObserve queries. All five have clear prevention strategies documented in PITFALLS.md and must be addressed in Phase 1.

## Key Findings

### Recommended Stack

The core stack is Go 1.24+ with OCB v0.120.0 as the build tool and the OTel collector packages all pinned to the v0.120.0/v1.26.0 release cycle. The Tetragon API module brings in gRPC-Go and protobuf transitively; the receiver module must explicitly pin `google.golang.org/protobuf` to match or exceed the Tetragon API's requirement to prevent duplicate proto registration panics. Version alignment is the single most operationally sensitive aspect of the stack — OCB validates `otelcol_version` against the resolved module graph and fails the build on mismatches.

The container image uses a multi-stage build: a Go build stage running OCB to produce a static binary, followed by a `debian-slim` runtime stage (required for journald receiver's systemd library dependency). `CGO_ENABLED=0` is the correct default; it is OCB's default and enables static linking.

**Core technologies:**
- Go 1.24+: Language minimum for otel-collector-contrib v0.120.0; CGO disabled for static linking
- OCB v0.120.0: Official tool for custom collector distributions; handles module resolution and binary compilation
- `go.opentelemetry.io/collector/receiver@v0.120.0`: Receiver factory + `consumer.Logs` interface; single entry point via `receiver.WithLogs`
- `go.opentelemetry.io/collector/pdata/plog@v1.26.0`: Internal OTLP log types (`plog.Logs`, `LogRecord`, `ResourceLogs`, `ScopeLogs`)
- `go.opentelemetry.io/collector/config/configgrpc@v0.120.0`: Structured gRPC client config providing standard YAML config surface for free
- `github.com/cilium/tetragon/api@v1.6.0`: Tetragon gRPC proto types and generated client; provides all event subtypes
- `google.golang.org/grpc@v1.79.x`: Use `grpc.NewClient` (not deprecated `grpc.Dial`)
- `google.golang.org/protobuf/encoding/protojson`: Use for JSON body serialization; must match Tetragon's own export format exactly

### Expected Features

The v1 feature set is well-defined and entirely P1 priority. All 14 must-have features are driven by the OTel SDK contract, the Tetragon integration requirement, or OpenObserve query compatibility. There are no ambiguous cases. The differentiating features (no filesystem coupling, `event_type` attribute pre-extracted, Kubernetes attributes from event payload) are either free or low-cost additions on top of the table stakes.

**Must have (table stakes):**
- `receiver.Logs` factory + `Start()`/`Shutdown()` lifecycle — mandatory SDK contract; nothing functions without it
- gRPC connection + stream consumption loop with context cancellation — core purpose
- Reconnection with exponential backoff + jitter — required for unattended production use
- `protojson.Marshal` JSON body matching Tetragon's own export format — OpenObserve query compatibility
- All Tetragon event types passed through without silent drops — data completeness guarantee
- `event_type` attribute extracted as first-class log attribute — enables downstream filtering without JSON parsing
- Kubernetes attributes (`k8s.pod.name`, `k8s.namespace.name`, `k8s.pod.uid`) extracted when pod info present
- Timestamp from event (not wall clock) — forensic accuracy for security events
- TLS config via `configgrpc.ClientConfig` (insecure default for localhost)
- Config validation (`Validate()`) — fail fast at startup
- Structured logging via injected `zap.Logger` — framework-compatible observability
- Internal telemetry via `obsreport` — pipeline health visibility in collector metrics

**Should have (differentiators, add after v1 validation):**
- Severity mapping per event type — enables downstream severity-based routing once query patterns are established
- Process identity attributes (`process.pid`, `process.executable.path`) — indexed process queries in OpenObserve
- Configurable reconnect backoff parameters — operator tuning for high-restart environments
- `metadata.yaml` stability declaration — required only if submitting to upstream contrib

**Defer (v2+):**
- Server-side `allow_list`/`deny_list` filtering — pipeline processors solve this for v1; adds config schema complexity with no incremental value now
- Metrics signal from Tetragon events — requires separate `connector` component architecture
- mTLS client certificate support beyond CA verification — defer until remote Tetragon deployments are a use case

### Architecture Approach

The project follows a clean separation across five files in an isolated Go module (`receiver/tetragonreceiver/`): `factory.go` (factory registration and default config), `config.go` (Config struct with mapstructure tags and `Validate()`), `receiver.go` (Start/Shutdown lifecycle + goroutine management), `converter.go` (pure stateless event-to-LogRecord function), and `factory_test.go`/`receiver_test.go`. The OCB manifest in `distribution/builder-config.yaml` references the local module via a `path:` field. This structure mirrors the standard contrib receiver pattern and is required for OCB's module resolution to work correctly — the receiver must be a standalone Go module with its own `go.mod`.

**Major components:**
1. `tetragonreceiver` (factory + config + receiver): Implements OTel SDK receiver contract; manages gRPC connection lifecycle via a background goroutine launched from `Start()` using a `context.WithCancel(context.Background())` context (not the `Start()` context)
2. `converter.go` (pure function): Converts `*tetragonpb.GetEventsResponse` to `plog.Logs`; stateless so it is unit-testable without a mock gRPC server; uses `protojson.Marshal` for body plus explicit attribute extraction for `event_type`, node name, and Kubernetes pod info
3. OCB manifest + Containerfile: Assembles the distribution binary from the custom receiver plus standard contrib components; multi-stage build produces a static binary on debian-slim for journald support
4. `config/config.yaml`: Runtime pipeline wiring; separate from build config; mounted into container at runtime
5. CI pipeline: Runs tests, builds the OCB distribution, pushes multi-arch image to GHCR on main merge or tag

### Critical Pitfalls

1. **Reusing the `Start()` context in the stream goroutine** — Create `r.ctx, r.cancel = context.WithCancel(context.Background())` inside `Start()` and use `r.ctx` in the goroutine; never pass the `Start()` ctx argument into any goroutine that outlives the method call. Silent failure: stream connects but zero events arrive.

2. **OCB `otelcol_version` misalignment** — Pin every component in `builder-config.yaml` to the same version (`v0.120.0`); inspect the generated `go.mod` after OCB runs to verify resolved versions align; never use `--skip-strict-versioning` in CI. Bump all components together, never piecemeal.

3. **gRPC error discrimination in the reconnect loop** — Check `r.ctx.Err() != nil` before deciding to reconnect; treat `codes.Canceled` with context cancellation as clean shutdown, not a retry trigger. Without this, `Shutdown()` triggers a reconnect storm and blocks for 30+ seconds.

4. **Protobuf dependency version conflict** — Explicitly pin `google.golang.org/protobuf` in the receiver's `go.mod` to match or exceed the Tetragon API's version; run `go mod graph | grep protobuf` after initial setup. A conflict panics the binary at startup with "proto: duplicate proto type registered".

5. **`protojson.Marshal` options mismatch** — Capture reference JSON from `tetra getevents --output json` before writing any marshaling code; write a table-driven test asserting byte-level JSON equality against reference. Wrong options (camelCase vs snake_case, populated vs unpopulated fields) break all existing OpenObserve queries silently.

6. **`Shutdown()` called before `Start()`** — Initialize `r.cancel` to a no-op function in the factory's `createLogsReceiver`, not in `Start()`; guard with nil check in `Shutdown()`. The OTel framework guarantees Shutdown can be called without prior Start during partial initialization failures.

7. **`nextConsumer.ConsumeLogs()` blocking the receive loop** — Decouple the gRPC `Recv()` loop from `ConsumeLogs()` with an internal buffered channel; add `select` on `r.ctx.Done()` so shutdown can interrupt a blocked consumer. Without this, a slow OpenObserve causes gRPC flow control to stall and Tetragon drops events server-side.

## Implications for Roadmap

Based on the dependency graph in FEATURES.md and the build order identified in ARCHITECTURE.md, the natural phase structure is: core receiver implementation first (all Phase 1 pitfalls land here), then OCB distribution build, then containerization and CI. This order is non-negotiable — the OCB manifest cannot reference a receiver module that doesn't compile, and the Containerfile wraps the OCB build.

### Phase 1: Core Receiver Implementation

**Rationale:** All critical pitfalls (Pitfalls 1, 3, 4, 5, 6, 7) must be addressed here. The receiver module is the foundation; nothing else builds without it. Architecture research identifies a strict intra-phase build order: `config.go` first (no external deps), `converter.go` second (testable immediately), then `factory.go` + `receiver.go` together.

**Delivers:** A standalone Go module (`receiver/tetragonreceiver/`) that compiles, passes lifecycle tests via `receivertest`, and converts Tetragon events to OTLP log records with correct JSON body format.

**Addresses:**
- All P1 table stakes: factory + lifecycle, gRPC connection, stream loop, reconnection with backoff, protojson body, all event types, event_type attribute, Kubernetes attributes, timestamp, TLS config, config validation, zap logging, obsreport
- Feature dependency chain: config → converter → factory/receiver → integration test

**Avoids:**
- Pitfall 1 (Start() context): Use `context.WithCancel(context.Background())` in Start()
- Pitfall 3 (gRPC error discrimination): Implement full error type switch with ctx.Err() check
- Pitfall 4 (protobuf conflict): Pin `google.golang.org/protobuf` in go.mod explicitly
- Pitfall 5 (protojson options): Write reference comparison test before any marshaling code
- Pitfall 6 (Shutdown before Start): Initialize cancel to no-op in factory
- Pitfall 7 (backpressure stall): Decouple Recv() from ConsumeLogs() with buffered channel

**Research flag:** Standard patterns — `receivertest`, `componenttest`, and the OTel receiver building guide provide all needed patterns. No additional research phase needed.

### Phase 2: OCB Distribution Build

**Rationale:** Depends entirely on Phase 1 producing a compilable receiver module. The OCB manifest uses a `path:` reference to the local module; without a working module, OCB cannot resolve or compile.

**Delivers:** `distribution/builder-config.yaml` producing a working `otelcol-tetragon` binary that includes the custom receiver alongside `journaldreceiver`, `batch`, `resourcedetection`, `otlphttp`, `health_check`, and `file_storage`. A working `config/config.yaml` wiring the pipeline.

**Uses:** OCB v0.120.0; all components pinned to v0.120.0; `path:` reference to `../receiver/tetragonreceiver` in manifest.

**Avoids:**
- Pitfall 2 (OCB version misalignment): Verify generated go.mod after OCB run; pin all components to same version
- OCB go.mod toolchain version bug (issue #11844): Add post-generation step to pin Go patch version
- Integration gotcha: Use relative path in `replaces` directive, not absolute path

**Research flag:** Standard OCB patterns. The OCB README and official docs cover this well. No additional research phase needed.

### Phase 3: Containerization and CI

**Rationale:** Containerfile wraps the OCB build; CI wraps the Containerfile. Neither is independently verifiable until Phase 2 produces a working binary. The multi-arch image requirement (amd64 + arm64) and the OCB go.mod toolchain pitfall are both verified here.

**Delivers:** Multi-stage `Containerfile` on debian-slim producing a multi-arch image (linux/amd64, linux/arm64); GitHub Actions CI pipeline (test → OCB build → container build → push to GHCR on main/tag).

**Implements:** Docker buildx multi-arch push; `mise.toml` pinning Go and OCB versions for reproducible builds in CI.

**Avoids:**
- Pitfall (OCB go.mod toolchain): CI clean environment will surface the unpinned toolchain bug; fix before CI setup
- Multi-arch gotcha: Use `docker buildx build --push --platform linux/amd64,linux/arm64`; single-arch push silently replaces a manifest list
- OpenObserve endpoint path: Verify `/api/default/v1/logs` vs `/v1/logs` against live OpenObserve instance
- OCB replace in CI: Ensure `path:` works in CI workspace or use `replaces` directive correctly

**Research flag:** Docker buildx multi-arch and GHCR push are standard patterns. OpenObserve OTLP endpoint path may need validation against a running instance — flag for implementation-time verification.

### Phase Ordering Rationale

- Phase 1 before Phase 2: OCB requires the receiver module to compile before it can build the distribution. All core pitfalls are in Phase 1 — they must be caught before they compound.
- Phase 2 before Phase 3: The Containerfile runs OCB; the CI runs the Containerfile. Each layer depends on the previous.
- converter.go before factory.go within Phase 1: The converter is a pure function with no gRPC dependency, making it the fastest path to a testable unit. Writing tests for the converter (including the protojson reference comparison test) before wiring the factory prevents the protojson options mismatch from reaching integration.
- Config validation before stream loop within Phase 1: Config struct with `Validate()` has zero external dependencies; implementing it first establishes the contract that the rest of Phase 1 builds on.

### Research Flags

Phases with standard patterns (skip research-phase):
- **Phase 1:** OTel receiver building guide, `receivertest` package, and the four architecture patterns in ARCHITECTURE.md fully specify the implementation. The pitfalls in PITFALLS.md identify all non-obvious traps with clear prevention code.
- **Phase 2:** OCB manifest format and `path:` reference pattern are well-documented in the OCB README and official OTel docs.
- **Phase 3:** GitHub Actions + Docker buildx + GHCR is a standard CI pattern.

Phases needing targeted implementation-time validation:
- **Phase 1 (protojson options):** Requires a live Tetragon instance to capture reference JSON from `tetra getevents --output json`. This cannot be fully verified without running Tetragon. Flag for first integration test session.
- **Phase 3 (OpenObserve endpoint path):** The OTLP/HTTP path OpenObserve expects (`/api/default/v1/logs`) deviates from the OTLP spec default. Verify against a running OpenObserve instance before completing Phase 3.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All core packages verified via pkg.go.dev and official OTel releases. Version pinning strategy confirmed via OCB issue tracker. Tetragon API go.mod verified directly. |
| Features | HIGH | OTel receiver SDK is well-documented. Tetragon gRPC API is stable v1.x. Feature set is bounded and clearly derived from the SDK contract and integration requirements. |
| Architecture | HIGH | Primary sources are official OTel collector docs and pkg.go.dev. All four architecture patterns have canonical implementations. The 5-file module structure mirrors the established contrib receiver pattern. |
| Pitfalls | HIGH | Most pitfalls verified via official OTel docs, grpc-go issue tracker, and confirmed OCB build issues with issue numbers. Not speculative — these are documented failure modes. |

**Overall confidence:** HIGH

### Gaps to Address

- **protojson MarshalOptions for Tetragon compatibility:** Research identifies the risk and prevention strategy but cannot pre-determine the exact `MarshalOptions` settings without running `tetra getevents --output json` against a live Tetragon instance. Address in Phase 1 by capturing reference output before writing any marshaling code.

- **OpenObserve OTLP endpoint path:** Research identifies that OpenObserve uses a non-standard path but the exact path (`/api/default/v1/logs` or organization-specific variant) must be verified against the target OpenObserve deployment. Address in Phase 3 configuration.

- **Tetragon `GetEventsRequest` default behavior:** Research notes that `GetEvents` with an empty request returns all event types by default, but this should be confirmed against the actual Tetragon version in the target deployment. If the behavior differs, the stream consumption loop requires a filter configuration.

- **`configretry.BackOffConfig` wiring:** Research recommends embedding `configretry.BackOffConfig` in the receiver's Config struct, but the exact field mapping to the reconnect loop's sleep/jitter logic requires implementation-time validation. The pattern is standard but the wiring is not pre-specified.

## Sources

### Primary (HIGH confidence)

- [Build a receiver — OpenTelemetry official docs](https://opentelemetry.io/docs/collector/extend/custom-component/receiver/) — lifecycle contract, factory pattern, context reuse warning
- [Collector Architecture — OpenTelemetry official docs](https://opentelemetry.io/docs/collector/architecture/) — component startup order, pipeline data flow
- [OCB documentation — OpenTelemetry official docs](https://opentelemetry.io/docs/collector/extend/ocb/) — manifest format, path: reference, version alignment
- [pkg.go.dev: go.opentelemetry.io/collector/receiver](https://pkg.go.dev/go.opentelemetry.io/collector/receiver) — `receiver.WithLogs`, `CreateLogsFunc` signature
- [pkg.go.dev: go.opentelemetry.io/collector/pdata/plog](https://pkg.go.dev/go.opentelemetry.io/collector/pdata/plog) — LogRecord, Logs, ResourceLogs, ScopeLogs APIs
- [pkg.go.dev: go.opentelemetry.io/collector/config/configgrpc](https://pkg.go.dev/go.opentelemetry.io/collector/config/configgrpc) — ClientConfig struct, ToClientConn
- [pkg.go.dev: github.com/cilium/tetragon/api/v1/tetragon](https://pkg.go.dev/github.com/cilium/tetragon/api/v1/tetragon) — FineGuidanceSensorsClient, GetEventsRequest/Response
- [Tetragon releases — cilium/tetragon](https://github.com/cilium/tetragon/releases) — v1.6.0 confirmed latest stable
- [pkg.go.dev: google.golang.org/grpc](https://pkg.go.dev/google.golang.org/grpc) — grpc.NewClient current API
- [OCB otelcol_version mismatch issue #8692](https://github.com/open-telemetry/opentelemetry-collector/issues/8692) — version locking requirement
- [OCB go.mod toolchain issue #11844](https://github.com/open-telemetry/opentelemetry-collector/issues/11844) — unpinned Go version bug
- [Shutdown must be safe without Start issue #9682](https://github.com/open-telemetry/opentelemetry-collector/issues/9682) — lifecycle contract
- [grpc-go context canceled race issue #3039](https://github.com/grpc/grpc-go/issues/3039) — error discrimination in Recv()

### Secondary (MEDIUM confidence)

- [Custom OTel Collector Distribution guide — Better Stack](https://betterstack.com/community/guides/observability/custom-opentelemetry-collector/) — OCB manifest structure confirmation
- [OTel Collector backpressure — Axoflow blog](https://axoflow.com/blog/opentelemetry-controller-outages-pipelines-backpressure) — backpressure propagation behavior
- [How to Build a Custom Receiver — oneuptime.com (Feb 2026)](https://oneuptime.com/blog/post/2026-02-06-build-custom-receiver-opentelemetry-collector/view) — recent implementation walkthrough

### Tertiary (needs validation during implementation)

- protojson MarshalOptions matching Tetragon's own export — requires live Tetragon instance to validate
- OpenObserve OTLP/HTTP endpoint path — requires running OpenObserve instance to confirm

---
*Research completed: 2026-03-18*
*Ready for roadmap: yes*
