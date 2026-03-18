# SPEC.md Discrepancies

Where research-informed decisions from Phase 1 context gathering override SPEC.md details.

**Rule:** Decisions in `01-CONTEXT.md` take precedence over SPEC.md. Update SPEC.md after Phase 1 is complete to keep it accurate.

## 1. Config Struct: configgrpc.ClientConfig vs configtls.ClientConfig

**SPEC says:** Custom `Config` struct with `configtls.ClientConfig` for TLS and manual endpoint/retry fields.

**Decision:** Use `configgrpc.ClientConfig` with `mapstructure:",squash"`. This is the standard OTel pattern (used by OTLP exporter and all gRPC-client components). Provides TLS, keepalive, compression, auth extensions, load balancing, headers, and OTel instrumentation out of the box. `ToClientConn()` assembles a ready `*grpc.ClientConn` — no manual dial option assembly needed.

**Impact:** Config YAML changes slightly (uses standard OTel field names). The `retry` config remains receiver-specific alongside the squashed ClientConfig.

## 2. Go Version

**SPEC says:** Go 1.23, `.mise/config.toml` has `go = "prefix:1.20"`.

**Decision:** Use latest stable Go (1.24.x). Update mise.toml to match.

## 3. Top-Level go.mod

**SPEC says:** Top-level `go.mod` with `replaces` directive for local receiver module.

**Clarification:** The top-level go.mod is a Phase 2 concern (OCB distribution). Phase 1 only creates `receiver/tetragonreceiver/go.mod` as a standalone module. The SPEC's repo structure is correct but spans multiple phases.

## 4. Test Infrastructure

**SPEC says:** Golden file tests comparing `protojson.Marshal` output. No specific framework mentioned.

**Decision:** Use OTel contrib's `pkg/golden` (`ReadLogs`/`WriteLogs`) + `pkg/pdatatest/plogtest.CompareLogs()` for pdata comparison. Captured Tetragon JSON fixtures in `testdata/events/` for protojson compatibility validation.

## 5. gRPC Mocking

**SPEC says:** Mock gRPC server (implied by "mock gRPC server replaying captured events" in integration test section).

**Decision:** Mock at the client interface level, not with a real gRPC server. Define a narrow Go interface for `GetEvents` + `GetVersion`, mock it in tests. This is the OTel contrib pattern (see mongodbatlasreceiver, mysqlreceiver). A real gRPC test server is unnecessary overhead.

## 6. Component Status Reporting

**SPEC says:** Not mentioned — receiver lifecycle only covers Start/Shutdown/stream loop.

**Decision:** Use `componentstatus.ReportStatus()` to report `RecoverableError` when disconnected and `StatusOK` when connected. This integrates with the healthcheck extension automatically. The kafkareceiver establishes this as the standard pattern.

---

*Created: 2026-03-18 during Phase 1 context gathering*
