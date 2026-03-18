# Stack Research

**Domain:** Custom OpenTelemetry Collector receiver in Go — gRPC streaming to OTLP logs pipeline
**Researched:** 2026-03-18
**Confidence:** HIGH (core OTel stack), MEDIUM (version pinning for v0.120.0 target)

---

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.24+ | Language | Minimum required by otel-collector-contrib v0.120.0+; two-latest-major policy means 1.24/1.25 are current targets |
| OCB (OpenTelemetry Collector Builder) | v0.120.0 | Assemble custom collector binary from manifest | Official tool for custom distributions; generates Go source + resolves deps + compiles; required for including a private receiver alongside contrib components |
| `go.opentelemetry.io/collector/component` | v1.26.0 (ships with v0.120.0 core) | Component lifecycle interfaces (Start/Shutdown/Host) | Every receiver must implement `component.Component`; v1.x is stable API — no breaking changes expected |
| `go.opentelemetry.io/collector/receiver` | v0.120.0 | Receiver factory + logs consumer interface | `receiver.NewFactory` + `receiver.WithLogs` is the single correct entry point for a logs receiver; `CreateLogsFunc` signature is the contract OCB expects |
| `go.opentelemetry.io/collector/pdata/plog` | v0.120.0 (ships with core) | Internal OTLP log representation | `plog.Logs`, `ResourceLogs`, `ScopeLogs`, `LogRecord` — canonical types passed to `consumer.Logs.ConsumeLogs()` |
| `go.opentelemetry.io/collector/pdata/pcommon` | v0.120.0 (ships with core) | Shared pdata types (Timestamp, Map, Value) | Required for attribute manipulation on `LogRecord`; co-versioned with plog |
| `go.opentelemetry.io/collector/consumer` | v0.120.0 (ships with core) | Downstream consumer interface | `consumer.Logs` is the `next` parameter injected into your receiver at factory construction time |
| `github.com/cilium/tetragon/api` | v1.6.0 (latest stable) | Tetragon gRPC proto types + generated client | Provides `FineGuidanceSensorsClient`, `GetEventsRequest`, `GetEventsResponse` and all event subtypes; standalone Go module at `github.com/cilium/tetragon/api/v1/tetragon` |
| `google.golang.org/grpc` | v1.79.x (pulled transitively via Tetragon API) | gRPC transport for `FineGuidanceSensors.GetEvents` stream | Industry-standard Go gRPC implementation; `grpc.NewClient` (not deprecated `Dial`) is the current API for connection creation |
| `google.golang.org/protobuf` | v1.36.x (pulled transitively) | Protobuf runtime + `protojson.Marshal` | `protojson.Marshal` on `GetEventsResponse` produces JSON matching Tetragon's own JSON export format — required for OpenObserve query compatibility |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `go.opentelemetry.io/collector/config/configgrpc` | v0.120.0 | Structured gRPC client config (endpoint, TLS, keepalive) | Use for the receiver's `Config` struct to get standard YAML config fields for free; `ClientConfig.ToClientConn()` handles dial option assembly |
| `go.opentelemetry.io/collector/config/configtls` | v0.120.0 | TLS config struct for gRPC | Use as embedded field in receiver config; handles `insecure`, `cert_file`, `key_file`, `ca_file` — covers the "insecure for localhost, TLS for remote" requirement |
| `go.opentelemetry.io/collector/config/configretry` | v0.120.0 | Retry/backoff config struct | Use to offer standard exponential-backoff config that operators already know; implement reconnect loop against Tetragon stream using its settings |
| `go.opentelemetry.io/collector/component/componenttest` | v0.120.0 | Test doubles (NopHost, NopTelemetry) | Required for unit tests of Start/Shutdown without a real collector host |
| `go.opentelemetry.io/collector/receiver/receivertest` | v0.120.0 | Sink consumer for receiver tests | `receivertest.NewNopSettings()` + `consumertest.NewNopLogsConsumer()` — standard test scaffold for verifying factory and receiver behavior |
| `go.uber.org/zap` | v1.27.x (pulled transitively by collector) | Structured logging inside receiver | Accessed via `receiver.Settings.TelemetrySettings.Logger`; do not add as a direct dep — use the injected logger |
| `google.golang.org/protobuf/encoding/protojson` | v1.36.x (pulled transitively) | Marshal proto message to JSON string | The one call that produces a JSON body matching Tetragon's own JSON export: `protojson.Marshal(resp)` → set as `logRecord.Body()` string value |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| OCB binary (`ocb`) | Build the custom collector distribution from `otelcol-builder.yaml` | Pin version to match target collector version (0.120.0); install via `go install go.opentelemetry.io/collector/cmd/builder@v0.120.0` or download release binary |
| mise | Task + tool version management | Project already mandates this; use `mise.toml` to pin Go version and OCB version so CI and local builds are identical |
| `go test ./...` | Unit + integration testing | Standard Go testing; no special harness needed for receiver unit tests |
| Docker buildx | Multi-arch image builds (amd64/arm64) | Use `docker buildx build --platform linux/amd64,linux/arm64`; OCB produces a statically-linked CGO-disabled binary by default, making cross-compilation straightforward |
| `ghcr.io` + GitHub Actions | Container registry + CI | Standard for open-source Go projects; push on tag or main merge |
| `golangci-lint` | Go linting | Run in CI; catches common mistakes in pdata manipulation and gRPC error handling |

---

## Installation

```bash
# Install OCB at target version
go install go.opentelemetry.io/collector/cmd/builder@v0.120.0

# Receiver module dependencies (go.mod for the receiver module)
go get go.opentelemetry.io/collector/component@v1.26.0
go get go.opentelemetry.io/collector/receiver@v0.120.0
go get go.opentelemetry.io/collector/pdata@v1.26.0
go get go.opentelemetry.io/collector/consumer@v0.120.0
go get go.opentelemetry.io/collector/config/configgrpc@v0.120.0
go get go.opentelemetry.io/collector/config/configtls@v0.120.0
go get go.opentelemetry.io/collector/config/configretry@v0.120.0

# Tetragon API (brings in grpc + protobuf transitively)
go get github.com/cilium/tetragon/api@v1.6.0

# Dev/test only
go get go.opentelemetry.io/collector/component/componenttest@v0.120.0
go get go.opentelemetry.io/collector/receiver/receivertest@v0.120.0
```

**Note on version numbering:** The OTel Collector uses a dual-version scheme. Stable APIs (`component`, `pdata`, `consumer`, `confmap` providers) are versioned at `v1.x.0`. The OCB and signal-specific packages (`receiver`, `processor`, `exporter`, `configgrpc`, etc.) remain at `v0.x.0`. They are released in lockstep: core `v1.26.0` and contrib `v0.120.0` are the same release. Match all packages to the same release cycle.

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `receiver.WithLogs` factory pattern | Standalone binary reading from gRPC directly | Never for this project — the point is a drop-in collector distribution with standard processor/exporter chains |
| OCB-generated distribution | Hand-written `main.go` with `otelcol.NewCommand` | Only if you need dynamic component loading or the manifest format is insufficient; adds complexity with no benefit here |
| `configgrpc.ClientConfig` embedded in receiver config | Raw `grpc.DialOptions` in code | `configgrpc` gives standard YAML configuration surface that operators know; plain dial options are not user-configurable |
| `protojson.Marshal` for JSON body | Custom struct serialization | `protojson` matches the canonical Tetragon JSON format exactly; custom serialization risks format drift breaking existing OpenObserve queries |
| Multi-stage Containerfile with Debian-slim base | Alpine or distroless base | Debian-slim required for journald receiver (systemd libs); project requirement is explicit |
| `grpc.NewClient` | `grpc.Dial` (deprecated) | `grpc.Dial` is deprecated as of gRPC-Go v1.69+; `grpc.NewClient` is the current non-deprecated API |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `github.com/golang/protobuf/jsonpb` (old jsonpb) | Deprecated; replaced by `google.golang.org/protobuf/encoding/protojson` in 2020; produces subtly different JSON for some types | `google.golang.org/protobuf/encoding/protojson` |
| `grpc.Dial` / `grpc.DialContext` | Deprecated as of gRPC-Go v1.69; will be removed in a future major version | `grpc.NewClient` with context passed at call time |
| `go.opentelemetry.io/collector/receiver/otlpreceiver` as a code dependency | You are writing a receiver, not consuming one; importing otlpreceiver in your receiver module creates a circular-style confusion and bloat | Implement `receiver.Logs` interface directly |
| Importing `github.com/open-telemetry/opentelemetry-collector-contrib` as a whole module | The contrib repo is a monorepo; individual receiver packages are independently versioned — importing the root module pulls in hundreds of unrelated dependencies | Import specific sub-packages only (e.g., `receiver/journaldreceiver`) and only in the OCB manifest |
| `otelcol_version` in OCB manifest that mismatches component versions | OCB validates that `otelcol_version` matches the version of the collector packages listed in receivers/exporters/etc. A mismatch causes build failure (GitHub Issue #8692) | Set `otelcol_version: 0.120.0` and use `v0.120.0` for all component `gomod` entries |
| `CGO_ENABLED=1` in the collector build | Increases image size and prevents static linking; not needed for this receiver (no C libraries required) | Keep OCB default `cgo_enabled: false` |

---

## Stack Patterns by Variant

**If the Tetragon endpoint is on localhost (default, no TLS):**
- Set `configtls.ClientConfig{Insecure: true}` in default config
- `grpc.NewClient("localhost:54321", grpc.WithTransportCredentials(insecure.NewCredentials()))`
- Because Tetragon uses unauthenticated localhost gRPC by default

**If the Tetragon endpoint uses mutual TLS:**
- Use `configtls.ClientConfig{CertFile, KeyFile, CAFile}` fields
- `configgrpc.ClientConfig.ToClientConn()` handles this transparently when TLS fields are populated
- No code path changes needed in the receiver itself

**If reconnect backoff needs to be user-tunable:**
- Embed `configretry.BackOffConfig` in the receiver's `Config` struct
- Use `configretry.BackOffConfig.InitialInterval`, `MaxInterval`, `MaxElapsedTime` to drive the sleep loop after stream error
- Because operators running this in production need to tune reconnect behavior without recompiling

---

## Version Compatibility

| Package | Compatible With | Notes |
|---------|-----------------|-------|
| `go.opentelemetry.io/collector/receiver@v0.120.0` | `go.opentelemetry.io/collector/component@v1.26.0` | Released in the same cycle; always match the `v0.x` and `v1.x` from the same release |
| `github.com/cilium/tetragon/api@v1.6.0` | `google.golang.org/grpc@v1.71.0`, `google.golang.org/protobuf@v1.36.5` | Tetragon API go.mod pins these versions; your receiver's go.mod must be compatible (same or newer) |
| `go.opentelemetry.io/collector/config/configgrpc@v0.120.0` | `google.golang.org/grpc@v1.68+` | configgrpc uses `grpc.NewClient` internally; requires gRPC-Go v1.68+ |
| OCB `v0.120.0` | Collector component packages `v0.120.0` / `v1.26.0` | OCB validates `otelcol_version` matches component versions in manifest; mismatches cause compile-time failure |
| Go `1.24` | All above packages | otel-collector-contrib v0.120.0 bumped minimum to Go 1.23; Go 1.24 is fully supported |

---

## Sources

- [OCB README — open-telemetry/opentelemetry-collector](https://github.com/open-telemetry/opentelemetry-collector/blob/main/cmd/builder/README.md) — builder manifest format, installation methods, version locking (MEDIUM confidence: README references v0.129.0; v0.120.0 verified from release page)
- [Build a receiver — OpenTelemetry official docs](https://opentelemetry.io/docs/collector/extend/custom-component/receiver/) — receiver.Factory, CreateLogsFunc, component.Component interfaces (HIGH confidence)
- [pkg.go.dev: go.opentelemetry.io/collector/receiver](https://pkg.go.dev/go.opentelemetry.io/collector/receiver) — `receiver.WithLogs`, `CreateLogsFunc` signature, v1.54.0/v0.148.0 at time of research (HIGH confidence)
- [pkg.go.dev: go.opentelemetry.io/collector/pdata/plog](https://pkg.go.dev/go.opentelemetry.io/collector/pdata/plog) — LogRecord, Logs, ResourceLogs, ScopeLogs APIs (HIGH confidence)
- [pkg.go.dev: go.opentelemetry.io/collector/component](https://pkg.go.dev/go.opentelemetry.io/collector/component) — Component, Host, Config interfaces (HIGH confidence)
- [pkg.go.dev: go.opentelemetry.io/collector/config/configgrpc](https://pkg.go.dev/go.opentelemetry.io/collector/config/configgrpc) — ClientConfig struct, ToClientConn signature (HIGH confidence)
- [OTel Collector releases — open-telemetry/opentelemetry-collector](https://github.com/open-telemetry/opentelemetry-collector/releases) — Latest release is v1.54.0/v0.148.0; dual versioning scheme confirmed (HIGH confidence)
- [otelcol-contrib releases](https://github.com/open-telemetry/opentelemetry-collector-contrib/releases) — v0.148.0 is latest; v0.120.0 confirmed to exist (HIGH confidence)
- [pkg.go.dev: github.com/cilium/tetragon/api/v1/tetragon](https://pkg.go.dev/github.com/cilium/tetragon/api/v1/tetragon) — FineGuidanceSensorsClient, GetEventsRequest/Response, event types (HIGH confidence)
- [Tetragon releases — cilium/tetragon](https://github.com/cilium/tetragon/releases) — v1.6.0 is latest stable (Oct 2024) (HIGH confidence)
- [Tetragon api/go.mod — cilium/tetragon](https://github.com/cilium/tetragon/blob/main/api/go.mod) — grpc v1.79.2, protobuf v1.36.11, Go 1.26 (HIGH confidence)
- [pkg.go.dev: google.golang.org/grpc](https://pkg.go.dev/google.golang.org/grpc) — v1.79.3 latest; grpc.NewClient is current API (HIGH confidence)
- [pkg.go.dev: google.golang.org/protobuf/encoding/protojson](https://pkg.go.dev/google.golang.org/protobuf/encoding/protojson) — MarshalOptions, UseProtoNames, EmitDefaultValues (HIGH confidence)
- [OCB otelcol_version mismatch issue #8692](https://github.com/open-telemetry/opentelemetry-collector/issues/8692) — confirms version locking requirement between OCB and component packages (HIGH confidence)

---

*Stack research for: Custom OTel Collector receiver consuming Tetragon gRPC stream*
*Researched: 2026-03-18*
