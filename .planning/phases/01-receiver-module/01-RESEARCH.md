# Phase 1: Receiver Module - Research

**Researched:** 2026-03-18
**Domain:** OpenTelemetry Collector custom receiver in Go, Tetragon gRPC API
**Confidence:** HIGH

## Summary

This phase builds a standalone Go module (`receiver/tetragonreceiver/`) that consumes Tetragon security events via gRPC server-streaming RPC and converts them to OTLP `plog.LogRecord`s. The OTel collector ecosystem is well-documented and has consistent patterns for gRPC-client receivers. All module versions were verified live from the Go module proxy (2026-03-17 timestamps).

The single highest-risk area is protojson format compatibility: Tetragon's own JSON encoder uses `protojson.MarshalOptions{UseProtoNames: true}` to produce snake_case field names. Our receiver must match this exactly or downstream OpenObserve queries break. Capture reference JSON first, before writing any marshaling code.

The OTel module versioning has two tracks: `component`, `receiver`, `consumer`, `pdata`, `config/configtls`, `config/configretry` are all at `v1.54.0` (stable API track), while `config/configgrpc`, `component/componentstatus`, `receiver/receiverhelper`, `receiver/receivertest` are at `v0.148.0` (unstable track). The SPEC.md's `v0.120.0` is stale — use `v0.148.0`/`v1.54.0` as appropriate.

**Primary recommendation:** Follow the configgrpc.ClientConfig squash pattern, implement ToClientConn with `host.GetExtensions()`, use receiverhelper.ObsReport for telemetry, and lock protojson to `UseProtoNames: true`.

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Tetragon API Version**
- Pin to `github.com/cilium/tetragon/api` v1.6.0 (latest, matches deployed Tetragon)
- No build matrix needed — protobuf wire compatibility means the client works with older/newer daemons
- Document the pinned version in go.mod comments for future maintainers

**Startup Health Behavior**
- `Start()` returns immediately — never blocks waiting for Tetragon connection
- Background goroutine handles connect + stream with exponential backoff retry (retry forever, no startup timeout)
- Report `componentstatus.NewRecoverableErrorEvent(err)` while disconnected or on stream errors
- Report `componentstatus.NewEvent(componentstatus.StatusOK)` when connection succeeds and streaming starts
- The `healthcheckv2extension` automatically aggregates these into HTTP health endpoints for Kubernetes probes

**Config Approach**
- Use `configgrpc.ClientConfig` with `mapstructure:",squash"` — standard OTel pattern
- Replaces the SPEC's custom Config with `configtls.ClientConfig` (see SPEC-DISCREPANCIES.md)
- Use `ToClientConn()` in `Start()` to get a ready `*grpc.ClientConn`
- Add receiver-specific fields alongside the squashed ClientConfig: `retry` (backoff settings)

**Test Data Sourcing**
- Capture reference JSON from `tetra getevents -o json` on a live Tetragon instance for all 10 event types
- Check captured JSON into `testdata/events/` as input fixtures
- Use `pkg/golden` (`ReadLogs`/`WriteLogs`) + `pkg/pdatatest/plogtest.CompareLogs()` for converter output validation
- Document capture command and Tetragon version in testdata/README.md

**Backpressure Handling**
- Decouple `Recv()` from `ConsumeLogs()` with a buffered channel
- Buffer size: 1000 events (configurable, sensible default)
- Overflow behavior: block `Recv()` when buffer is full (backpressure propagates to gRPC stream)
- Do not drop events silently
- Log a warning when buffer reaches 80% capacity

**gRPC Client Mocking**
- Define a narrow Go interface covering only the RPCs we use: `GetEvents` and `GetVersion`
- Mock this interface in tests with a simple struct (or testify/mock)
- Do not use bufconn or a real test gRPC server
- Test reconnection behavior by having the mock return errors then recover

**Go Version and Tooling**
- Use latest stable Go (1.24.x as of March 2026) — update mise.toml accordingly
- mise.toml serves as the canonical tool/task configuration — update it as needed
- PROJ-01 (mise.toml with Go, OCB, and project tasks) is in scope for this phase

**Lifecycle Details**
- Use `context.WithCancel(context.Background())` inside `Start()` — never pass Start()'s ctx to the background goroutine
- Initialize `r.cancel` to a no-op function in the factory — guards against Shutdown-before-Start
- Capture reference JSON from `tetra getevents --output json` before writing any protojson marshaling code

### Claude's Discretion
- Exact backoff parameters (initial_interval, max_interval defaults)
- Internal code organization within convert.go (helper functions, type switches vs maps)
- Error message wording and zap field choices for structured logging
- Test helper organization and shared test utilities
- Whether to use mdatagen for component metadata generation

### Deferred Ideas (OUT OF SCOPE)
- Server-side event filtering via allow_list/deny_list config — explicitly deferred to post-v1
- Metrics pipeline from Tetragon events — noted for v2
- Build matrix for multiple Tetragon API versions
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| RECV-01 | Receiver registers as logs receiver via `receiver.NewFactory` with `receiver.WithLogs` | receiver.NewFactory + receiver.WithLogs pattern documented; receivertest.NewNopSettings for tests |
| RECV-02 | `Start()` connects to Tetragon gRPC endpoint, spawns stream goroutine without blocking | configgrpc.ClientConfig.ToClientConn(); context.WithCancel(context.Background()) pattern confirmed |
| RECV-03 | `Shutdown()` cancels stream context, waits for goroutine exit, closes gRPC connection | sync.WaitGroup + cancel pattern; grpc.ClientConn.Close() |
| RECV-04 | Streams events via `FineGuidanceSensors.GetEvents` server-streaming RPC | Tetragon API v1.6.0 confirmed stable; pkg.go.dev verified |
| RECV-05 | Reconnects with exponential backoff on stream errors | cenkalti/backoff v4.3.0; ExponentialBackOff + backoff.Retry with context |
| RECV-06 | Distinguishes clean shutdown (context cancelled) from transient errors | ctx.Err() check pattern after Recv() error |
| RECV-07 | Logs connection events, errors, reconnects via zap structured logging | go.uber.org/zap v1.27.1 confirmed |
| RECV-08 | Reports internal telemetry via obsreport (accepted/refused log records) | receiverhelper.ObsReport; StartLogsOp/EndLogsOp pattern confirmed |
| CONF-01 | Config struct with endpoint, TLS, retry via `configgrpc.ClientConfig` | configgrpc.ClientConfig squash pattern verified; v0.148.0 |
| CONF-02 | Config validates at startup (empty endpoint, invalid TLS paths fail fast) | configgrpc.ClientConfig.Validate() available; custom Validate() wraps it |
| CONF-03 | Default config: endpoint `localhost:54321`, insecure true, reasonable backoff defaults | configgrpc.NewDefaultClientConfig(); configtls.ClientConfig{Insecure: true} |
| CONF-04 | Config is YAML-configurable via standard OTel Collector config file | mapstructure squash provides standard YAML field names |
| CONV-01 | Each GetEventsResponse becomes one plog.LogRecord with scope name `tetragonreceiver` | plog.NewLogs(), ScopeLogs().AppendEmpty(), Scope().SetName() |
| CONV-02 | Log body contains full JSON via `protojson.Marshal` matching Tetragon's own JSON export | CRITICAL: UseProtoNames: true confirmed from Tetragon encoder.go |
| CONV-03 | Timestamp from event's time field; ObservedTimestamp = receive time | pcommon.NewTimestampFromTime(); GetEventsResponse has time field |
| CONV-04 | Severity mapped per event type | plog.SeverityNumberInfo/Warn/Error; SeverityText |
| CONV-05 | Static attributes: event.domain=tetragon, event.name=event type string | lr.Attributes().PutStr() |
| CONV-06 | Process attributes: binary, arguments, pid, uid, exec_id, cwd | proc.GetBinary(), GetPid().GetValue() (wrapped UInt32Value) |
| CONV-07 | Parent process attributes when present | nil-check proc.GetParent(); all Process fields are optional |
| CONV-08 | Event-specific attributes: policy_name, action, function_name, etc. | Type switch on GetEvent() oneof; each event type has distinct fields |
| CONV-09 | Kubernetes attributes when pod info present | proc.GetPod() nil check; pod.GetNamespace(), GetName(), GetContainer() |
| CONV-10 | All 10 event types handled | Full oneof mapping in SPEC.md confirmed against API v1.6.0 |
| PROJ-01 | mise.toml with Go, OCB, and project tasks | TOML task syntax verified; [tasks.test], [tasks.build], [tasks.lint] |
| PROJ-03 | metadata.yaml declaring receiver type and alpha stability for logs signal | Minimal format: type + status.class + stability.alpha: [logs] |
</phase_requirements>

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `go.opentelemetry.io/collector/receiver` | v1.54.0 | Factory and receiver.Logs interface | Required OTel component registration |
| `go.opentelemetry.io/collector/component` | v1.54.0 | component.Host, component.Config interfaces | Core OTel lifecycle interface |
| `go.opentelemetry.io/collector/consumer` | v1.54.0 | consumer.Logs interface | Pipeline consumer contract |
| `go.opentelemetry.io/collector/pdata` | v1.54.0 | plog.Logs, plog.LogRecord, pcommon types | OTel in-memory data model |
| `go.opentelemetry.io/collector/config/configgrpc` | v0.148.0 | ClientConfig with ToClientConn() | Standard gRPC client config for OTel |
| `go.opentelemetry.io/collector/config/configtls` | v1.54.0 | TLS config (embedded in configgrpc) | TLS config embedded inside ClientConfig |
| `go.opentelemetry.io/collector/config/configretry` | v1.54.0 | Retry/backoff config struct | Standard retry config type |
| `go.opentelemetry.io/collector/component/componentstatus` | v0.148.0 | ReportStatus, NewRecoverableErrorEvent | Component health reporting |
| `go.opentelemetry.io/collector/receiver/receiverhelper` | v0.148.0 | ObsReport for accepted/refused counters | Replaces deprecated obsreport package |
| `github.com/cilium/tetragon/api` | v1.6.0 | Tetragon gRPC client + proto types | Tetragon event types and RPC client |
| `google.golang.org/grpc` | v1.79.3 | gRPC transport layer | Standard gRPC Go library |
| `google.golang.org/protobuf` | v1.36.11 | protojson.MarshalOptions for JSON body | protojson encoder |
| `go.uber.org/zap` | v1.27.1 | Structured logging | OTel Collector standard logger |
| `github.com/cenkalti/backoff/v4` | v4.3.0 | Exponential backoff for reconnection | Widely used; backoff.Retry + WithContext |

### Test-Only
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `go.opentelemetry.io/collector/receiver/receivertest` | v0.148.0 | NewNopSettings, NewNopFactory | Factory and lifecycle tests |
| `github.com/open-telemetry/opentelemetry-collector-contrib/pkg/golden` | v0.148.0 | ReadLogs/WriteLogs golden YAML files | Converter output validation |
| `github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest` | v0.148.0 | plogtest.CompareLogs() | Golden file diff comparison |
| `github.com/stretchr/testify` | v1.11.1 | require.NoError, assert | Standard Go test assertions |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `configgrpc.ClientConfig` squash | Custom Config + `configtls.ClientConfig` | Custom config loses auth extensions, keepalive, OTel instrumentation — don't do this (see SPEC-DISCREPANCIES.md) |
| `cenkalti/backoff` | Manual sleep loop | Manual loop is error-prone, doesn't handle context cancellation correctly |
| `receiverhelper.ObsReport` | Manual metrics | `obsreport` package is deprecated; receiverhelper is the replacement |
| Interface mocking | bufconn / real gRPC server | Real gRPC server adds complexity; interface mock is the OTel contrib pattern |
| `pkg/golden` YAML | Manual JSON comparison | pkg/golden produces stable diffs and handles pdata serialization edge cases |

**Installation:**
```bash
# In receiver/tetragonreceiver/ module:
go get go.opentelemetry.io/collector/receiver@v1.54.0
go get go.opentelemetry.io/collector/component@v1.54.0
go get go.opentelemetry.io/collector/consumer@v1.54.0
go get go.opentelemetry.io/collector/pdata@v1.54.0
go get go.opentelemetry.io/collector/config/configgrpc@v0.148.0
go get go.opentelemetry.io/collector/config/configretry@v1.54.0
go get go.opentelemetry.io/collector/component/componentstatus@v0.148.0
go get go.opentelemetry.io/collector/receiver/receiverhelper@v0.148.0
go get go.opentelemetry.io/collector/receiver/receivertest@v0.148.0
go get github.com/cilium/tetragon/api@v1.6.0
go get google.golang.org/grpc@v1.79.3
go get google.golang.org/protobuf@v1.36.11
go get go.uber.org/zap@v1.27.1
go get github.com/cenkalti/backoff/v4@v4.3.0
go get github.com/open-telemetry/opentelemetry-collector-contrib/pkg/golden@v0.148.0
go get github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest@v0.148.0
go get github.com/stretchr/testify@v1.11.1
```

**Version verification:** All versions above were confirmed from the Go module proxy on 2026-03-17/18.

---

## Architecture Patterns

### Recommended Project Structure
```
receiver/tetragonreceiver/
├── go.mod                  # Standalone module: github.com/YOUR_ORG/otelcol-tetragon/receiver/tetragonreceiver
├── go.sum
├── metadata.yaml           # type: tetragon, stability: alpha: [logs]
├── doc.go                  # Package doc comment
├── config.go               # Config struct (embeds configgrpc.ClientConfig squash) + Validate()
├── config_test.go          # Validate(), defaults, YAML round-trip
├── factory.go              # NewFactory(), createDefaultConfig(), createLogsReceiver()
├── factory_test.go         # receivertest.NewNopSettings, factory creates receiver without panic
├── receiver.go             # tetragonReceiver: Start(), Shutdown(), stream loop goroutine
├── receiver_test.go        # Start/Shutdown lifecycle, reconnect on mock error
├── convert.go              # convertEvent(): GetEventsResponse → plog.Logs
├── convert_test.go         # golden file tests for all 10 event types
└── testdata/
    ├── config.yaml         # Valid, invalid config fixtures for config tests
    ├── README.md           # Capture command and Tetragon version for reference fixtures
    └── events/             # Raw Tetragon JSON fixtures (input to converter tests)
        ├── process_exec.json
        ├── process_exit.json
        ├── process_kprobe.json
        ├── process_tracepoint.json
        ├── process_loader.json
        ├── process_uprobe.json
        ├── process_lsm.json
        ├── process_usdt.json
        ├── process_throttle.json
        └── rate_limit_info.json
    └── golden/             # Expected pdata YAML output (written by converter tests)
        ├── process_exec.yaml
        └── ...
```

### Pattern 1: Config with configgrpc.ClientConfig Squash
**What:** Embed `configgrpc.ClientConfig` in the Config struct with `mapstructure:",squash"`. This makes all ClientConfig fields (endpoint, tls, keepalive, etc.) appear at the top level of the YAML config block.
**When to use:** Any gRPC-client receiver — this is the universal OTel pattern.

```go
// Source: configgrpc package docs (pkg.go.dev/go.opentelemetry.io/collector/config/configgrpc v0.148.0)
type Config struct {
    configgrpc.ClientConfig `mapstructure:",squash"`
    Retry configretry.BackOffConfig `mapstructure:"retry"`
}

func (c *Config) Validate() error {
    if c.Endpoint == "" {
        return errors.New("endpoint is required")
    }
    return c.ClientConfig.Validate()
}

func createDefaultConfig() component.Config {
    cfg := configgrpc.NewDefaultClientConfig()
    cfg.Endpoint = "localhost:54321"
    cfg.TLS = configtls.ClientConfig{Insecure: true}
    return &Config{
        ClientConfig: cfg,
        Retry: configretry.BackOffConfig{
            Enabled:         true,
            InitialInterval: 1 * time.Second,
            MaxInterval:     30 * time.Second,
            MaxElapsedTime:  0, // retry forever
        },
    }
}
```

### Pattern 2: Factory Registration
**What:** Use `receiver.NewFactory` with `receiver.WithLogs` to register the receiver. Stability level is `component.StabilityLevelAlpha`.
**When to use:** Every OTel receiver must have a factory.

```go
// Source: receiver package (pkg.go.dev/go.opentelemetry.io/collector/receiver v1.54.0)
const typeStr = component.Type("tetragon")

func NewFactory() receiver.Factory {
    return receiver.NewFactory(
        typeStr,
        createDefaultConfig,
        receiver.WithLogs(createLogsReceiver, component.StabilityLevelAlpha),
    )
}

func createLogsReceiver(
    _ context.Context,
    settings receiver.Settings,
    cfg component.Config,
    consumer consumer.Logs,
) (receiver.Logs, error) {
    rCfg := cfg.(*Config)
    return &tetragonReceiver{
        cfg:      rCfg,
        logger:   settings.Logger,
        consumer: consumer,
        cancel:   func() {}, // no-op init — guards Shutdown-before-Start
    }, nil
}
```

### Pattern 3: Lifecycle with Background Goroutine and Cancel Guard
**What:** `Start()` creates a new cancel context from `context.Background()` (NOT from Start's ctx), stores it, spawns goroutine. `Shutdown()` cancels and waits.
**When to use:** Any receiver with long-running background work.

```go
// Source: OTel custom receiver guide (opentelemetry.io/docs/collector/extend/custom-component/receiver/)
// and STATE.md pre-phase decisions

type tetragonReceiver struct {
    cfg      *Config
    logger   *zap.Logger
    consumer consumer.Logs
    conn     *grpc.ClientConn
    cancel   context.CancelFunc  // initialized to no-op in factory
    wg       sync.WaitGroup
    host     component.Host
}

func (r *tetragonReceiver) Start(ctx context.Context, host component.Host) error {
    r.host = host
    conn, err := r.cfg.ClientConfig.ToClientConn(
        ctx,
        host.GetExtensions(),
        r.logger,  // or component.TelemetrySettings
    )
    if err != nil {
        return fmt.Errorf("failed to create gRPC connection: %w", err)
    }
    r.conn = conn

    streamCtx, cancel := context.WithCancel(context.Background())
    r.cancel = cancel

    r.wg.Add(1)
    go r.streamEvents(streamCtx)
    return nil
}

func (r *tetragonReceiver) Shutdown(ctx context.Context) error {
    r.cancel() // safe: initialized to no-op, set in Start
    r.wg.Wait()
    if r.conn != nil {
        return r.conn.Close()
    }
    return nil
}
```

### Pattern 4: Stream Loop with Exponential Backoff and Channel Decoupling
**What:** A `Recv()` goroutine reads from gRPC stream and puts events into a buffered channel. A separate goroutine drains the channel and calls `ConsumeLogs()`. Uses `cenkalti/backoff` for reconnect.
**When to use:** Any streaming receiver where the producer (gRPC) can outpace the consumer (pipeline).

```go
// Source: STATE.md pre-phase decisions + cenkalti/backoff v4 docs
func (r *tetragonReceiver) streamEvents(ctx context.Context) {
    defer r.wg.Done()

    eventCh := make(chan *tetragonv1.GetEventsResponse, 1000)
    go r.consumeChannel(ctx, eventCh)

    b := backoff.NewExponentialBackOff()
    b.InitialInterval = r.cfg.Retry.InitialInterval
    b.MaxInterval = r.cfg.Retry.MaxInterval
    b.MaxElapsedTime = 0 // retry forever

    for {
        if ctx.Err() != nil {
            return
        }
        err := r.runStream(ctx, eventCh)
        if ctx.Err() != nil {
            return // clean shutdown
        }
        componentstatus.ReportStatus(r.host,
            componentstatus.NewRecoverableErrorEvent(err))
        r.logger.Warn("stream error, reconnecting", zap.Error(err))
        wait := b.NextBackOff()
        select {
        case <-ctx.Done():
            return
        case <-time.After(wait):
        }
    }
}
```

### Pattern 5: protojson Body — CRITICAL MarshalOptions
**What:** Always use `protojson.MarshalOptions{UseProtoNames: true}`. This matches Tetragon's own JSON encoder (`tetra getevents -o json`) which explicitly sets `UseProtoNames: true` for backward compatibility with snake_case field names.
**When to use:** Any time a GetEventsResponse is serialized to the log body.

```go
// Source: Tetragon encoder.go (github.com/cilium/tetragon blob d2c40f20)
// Comment: "Our old exporter's behaviour was to use the snake_case names rather than
// camelCase. We want to maintain backward compatibility here."
var jsonMarshaler = protojson.MarshalOptions{UseProtoNames: true}

func convertBody(resp *tetragonv1.GetEventsResponse) (string, error) {
    b, err := jsonMarshaler.Marshal(resp)
    if err != nil {
        return "", err
    }
    return string(b), nil
}
```

**If you omit `UseProtoNames: true`**, protojson defaults to camelCase (e.g., `processExec` instead of `process_exec`). This breaks existing OpenObserve queries.

### Pattern 6: ObsReport for Telemetry (RECV-08)
**What:** Use `receiverhelper.ObsReport` (NOT the deprecated `obsreport` package — that module no longer exists in the registry). Call `StartLogsOp`/`EndLogsOp` around each ConsumeLogs call.
**When to use:** Required for RECV-08 accepted/refused metrics.

```go
// Source: pkg.go.dev/go.opentelemetry.io/collector/receiver/receiverhelper v0.148.0
obsReport, err := receiverhelper.NewObsReport(receiverhelper.ObsReportSettings{
    ReceiverID:             settings.ID,
    Transport:              "grpc",
    ReceiverCreateSettings: settings,
})

// In consume loop:
opCtx := obsReport.StartLogsOp(ctx)
err := r.consumer.ConsumeLogs(opCtx, logs)
obsReport.EndLogsOp(opCtx, "tetragon", 1, err)
```

### Pattern 7: Component Status Reporting (RECV-02/RECV-03)
**What:** Report `StatusOK` when streaming starts, `RecoverableError` when disconnected. Use `componentstatus.ReportStatus(host, event)`.
**When to use:** After each successful connect and on each stream error.

```go
// Source: pkg.go.dev/go.opentelemetry.io/collector/component/componentstatus v0.148.0
// On successful connect:
componentstatus.ReportStatus(r.host, componentstatus.NewEvent(componentstatus.StatusOK))
// On stream error:
componentstatus.ReportStatus(r.host, componentstatus.NewRecoverableErrorEvent(err))
```

### Pattern 8: receivertest for Factory Tests (RECV-01)
**What:** Use `receivertest.NewNopSettings(typeStr)` for factory test settings. Verify that `factory.CreateLogsReceiver(ctx, settings, cfg, consumer)` returns no error and no panic.
**When to use:** factory_test.go to prove RECV-01.

```go
// Source: pkg.go.dev/go.opentelemetry.io/collector/receiver/receivertest v0.148.0
func TestNewFactory(t *testing.T) {
    factory := NewFactory()
    assert.Equal(t, component.Type("tetragon"), factory.Type())

    cfg := factory.CreateDefaultConfig()
    settings := receivertest.NewNopSettings(factory.Type())
    consumer := &consumertest.LogsSink{}

    recv, err := factory.CreateLogsReceiver(context.Background(), settings, cfg, consumer)
    require.NoError(t, err)
    require.NotNil(t, recv)
}
```

### Pattern 9: Golden File Tests for Converter (CONV-01 through CONV-10)
**What:** Parse raw Tetragon JSON fixture with protojson, pass to `convertEvent()`, compare output against golden YAML using pkg/golden + plogtest.CompareLogs.
**When to use:** convert_test.go for each of the 10 event types.

```go
// Source: pkg.go.dev/github.com/open-telemetry/opentelemetry-collector-contrib/pkg/golden v0.148.0
// Source: pkg.go.dev/github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest v0.148.0
func TestConvertProcessExec(t *testing.T) {
    raw, err := os.ReadFile("testdata/events/process_exec.json")
    require.NoError(t, err)

    var resp tetragonv1.GetEventsResponse
    require.NoError(t, protojson.Unmarshal(raw, &resp))

    got := convertEvent(&resp)

    // Update golden: run with -update flag
    // golden.WriteLogs(t, "testdata/golden/process_exec.yaml", got)

    expected, err := golden.ReadLogs("testdata/golden/process_exec.yaml")
    require.NoError(t, err)
    require.NoError(t, plogtest.CompareLogs(expected, got,
        plogtest.IgnoreObservedTimestamp()))
}
```

### Pattern 10: metadata.yaml (PROJ-03)
**What:** Minimal metadata.yaml declaring receiver type and stability. mdatagen is optional for this phase (no generated metrics); a hand-written metadata.yaml suffices for PROJ-03.
**When to use:** Required for component registration; placed in module root.

```yaml
# Source: OTel metadata-schema.yaml (github.com/open-telemetry/opentelemetry-collector/cmd/mdatagen)
type: tetragon

status:
  class: receiver
  stability:
    alpha: [logs]
```

### Pattern 11: mise.toml Task Configuration (PROJ-01)
**What:** TOML task syntax in mise.toml for test, build, lint. Uses `[tasks.name]` sections with `run`, `description`, `dir`.
**When to use:** PROJ-01 requires mise.toml with Go, OCB, and project tasks.

```toml
# Source: mise.jdx.dev/tasks/toml-tasks.html
[tools]
go = "1.24"

[tasks.test]
description = "Run receiver unit tests"
run = "go test -race ./..."
dir = "receiver/tetragonreceiver"

[tasks.lint]
description = "Run golangci-lint"
run = "golangci-lint run ./..."
dir = "receiver/tetragonreceiver"

[tasks.build]
description = "Build custom collector with OCB"
run = "builder --config builder-config.yaml"
```

### Anti-Patterns to Avoid
- **Passing Start()'s ctx to goroutine:** Start's context is cancelled after Start() returns in some OTel versions. Use `context.WithCancel(context.Background())` instead (STATE.md decision).
- **Nil cancel in factory:** If Shutdown() is called before Start(), `r.cancel()` panics if nil. Initialize to `func() {}` in factory (STATE.md decision).
- **Calling ToClientConn with host directly:** The API now takes `host.GetExtensions()` (a `map[component.ID]component.Component`), not `host` itself. Passing `nil` is safe in tests.
- **Using deprecated obsreport package:** The `go.opentelemetry.io/collector/obsreport` module does not exist in the current registry. Use `receiver/receiverhelper` instead.
- **Blocking ConsumeLogs inside Recv() loop:** This causes backpressure from the pipeline to stall the gRPC stream read loop. Use buffered channel to decouple (STATE.md decision).
- **Default protojson (camelCase) for body:** Without `UseProtoNames: true`, field names become camelCase, breaking OpenObserve queries against existing data (CRITICAL — verified from Tetragon source).
- **Starting background work inside Recv() loop without WaitGroup:** Makes Shutdown() unable to wait for goroutine exit, causing data races on cleanup.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| gRPC connection with TLS/auth | Custom grpc.Dial options assembly | `configgrpc.ClientConfig.ToClientConn()` | Handles TLS, auth extensions, keepalive, compression, OTel instrumentation automatically |
| Exponential backoff with context | Manual sleep loop with `time.Sleep` | `cenkalti/backoff/v4` + `backoff.Retry(op, backoff.WithContext(b, ctx))` | Context-aware, jitter, max elapsed time, reset on success |
| Accepted/refused metrics counters | Custom prometheus counters | `receiverhelper.ObsReport` | Plugs into OTel's internal telemetry pipeline; screwed up manually |
| plog comparison in tests | Manual struct diff | `plogtest.CompareLogs()` with options | Handles float precision, ordering, semantic equality for pdata types |
| Golden file I/O | `os.ReadFile`/`os.WriteFile` with JSON | `pkg/golden.ReadLogs`/`WriteLogs` | Handles YAML serialization of pdata, -update flag pattern |
| JSON serialization of proto | Custom JSON marshaling or `encoding/json` | `protojson.MarshalOptions{UseProtoNames: true}` | Only correct path for proto3 JSON; encoding/json ignores protobuf field options |
| Component health reporting | Custom HTTP health endpoint | `componentstatus.ReportStatus()` | Integrates with healthcheckv2extension automatically |

**Key insight:** The configgrpc.ClientConfig squash pattern eliminates roughly 100 lines of boilerplate that every custom gRPC receiver used to write. It also gives users TLS, auth extensions, and keepalive for free with zero receiver code.

---

## Common Pitfalls

### Pitfall 1: Context Lifetime Mismatch in Start()
**What goes wrong:** Background goroutine uses Start()'s `ctx` argument. After Start() returns, the OTel framework may cancel that context.
**Why it happens:** Receiver authors assume Start's ctx is a long-lived context, but it's a setup context.
**How to avoid:** Always `context.WithCancel(context.Background())` inside Start(). Store the cancel function on the struct.
**Warning signs:** Goroutine exits immediately after Start() returns; stream reconnects instantly.

### Pitfall 2: Nil Cancel Function
**What goes wrong:** Shutdown() is called before Start() (valid OTel lifecycle edge case). `r.cancel()` panics on nil.
**Why it happens:** cancel is only assigned in Start().
**How to avoid:** Initialize `cancel: func() {}` in the factory's `createLogsReceiver`.
**Warning signs:** Panic in tests that call Shutdown() without Start().

### Pitfall 3: protojson camelCase Body
**What goes wrong:** JSON body uses camelCase field names (`processExec`, `execId`) instead of snake_case (`process_exec`, `exec_id`). OpenObserve queries fail silently.
**Why it happens:** protojson defaults to lowerCamelCase per proto3 JSON spec.
**How to avoid:** `protojson.MarshalOptions{UseProtoNames: true}` — verified from Tetragon's own encoder.go.
**Warning signs:** Body field names don't match `tetra getevents -o json` output in reference fixtures.

### Pitfall 4: Backpressure Deadlock
**What goes wrong:** `consumer.ConsumeLogs()` blocks inside the `Recv()` loop. The pipeline blocks, which blocks Recv(), which stops reading the gRPC stream, which causes the server-side stream buffer to fill. Eventually the entire system stalls.
**Why it happens:** ConsumeLogs can block if the batch processor or exporter is slow.
**How to avoid:** Decouple with a buffered channel of size 1000. Recv() sends to channel; separate goroutine calls ConsumeLogs.
**Warning signs:** High-throughput tests hang; Shutdown() blocks indefinitely.

### Pitfall 5: Not Capturing Reference JSON First
**What goes wrong:** Tests are written against assumed protojson output. Later, real Tetragon events have different field structure (nil optionals, oneof variants, enum values).
**Why it happens:** Proto3 has many optional/wrapper fields that only appear when set.
**How to avoid:** Capture real Tetragon JSON with `tetra getevents --output json` before writing convert.go. Write golden YAML from real input.
**Warning signs:** Tests pass but converter silently drops fields present in real events.

### Pitfall 6: ToClientConn API Change
**What goes wrong:** Passing `host` directly to `ToClientConn` instead of `host.GetExtensions()`. Compilation error.
**Why it happens:** The API changed — second parameter is now `map[component.ID]component.Component`.
**How to avoid:** Call `r.cfg.ClientConfig.ToClientConn(ctx, host.GetExtensions(), settings.TelemetrySettings)`.
**Warning signs:** Compile error: "cannot use host (type component.Host) as type map[...]".

### Pitfall 7: Using Deprecated obsreport Package
**What goes wrong:** Import `go.opentelemetry.io/collector/obsreport` fails — module not found in registry.
**Why it happens:** The package was deprecated and removed; `receiverhelper.ObsReport` is the replacement.
**How to avoid:** Use `receiver/receiverhelper` package only.
**Warning signs:** `go get go.opentelemetry.io/collector/obsreport` returns "not found".

### Pitfall 8: UInt32Value Nil Dereference
**What goes wrong:** `proc.GetPid().GetValue()` panics if `proc.GetPid()` returns nil.
**Why it happens:** Tetragon uses `google.protobuf.UInt32Value` (wrapped scalar) for PID/UID, which is absent when zero or unknown.
**How to avoid:** `proc.GetPid()` returns nil safely in Go protobuf; `(*UInt32Value)(nil).GetValue()` returns 0. Chain `.GetPid().GetValue()` without nil checks. Parent fields need nil check: `if parent := resp.GetParent(); parent != nil`.
**Warning signs:** Panics only on specific event types lacking parent/uid fields.

### Pitfall 9: Module Version Mismatch
**What goes wrong:** Mixing OTel modules from different release versions (e.g., receiver v1.54.0 with configgrpc v0.120.0). The two tracks have different import paths and incompatible API versions.
**Why it happens:** Stable track (component, receiver, pdata) is v1.x; unstable track (configgrpc, componentstatus, receiverhelper) is v0.x.
**How to avoid:** Keep all OTel modules pinned to the same release epoch (v1.54.0 / v0.148.0, released 2026-03-17).
**Warning signs:** Compilation errors about interface mismatches in component.Host or receiver.Settings types.

---

## Code Examples

### Full go.mod Skeleton
```go
// Source: SPEC.md adapted with verified versions (Go proxy 2026-03-17)
module github.com/YOUR_ORG/otelcol-tetragon/receiver/tetragonreceiver

// Pin to v1.6.0 to match deployed Tetragon daemon — see CONTEXT.md
// Protobuf wire compat allows working with older/newer daemons

go 1.24

require (
    github.com/cenkalti/backoff/v4                                                    v4.3.0
    github.com/cilium/tetragon/api                                                    v1.6.0
    github.com/open-telemetry/opentelemetry-collector-contrib/pkg/golden              v0.148.0
    github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest           v0.148.0
    github.com/stretchr/testify                                                       v1.11.1
    go.opentelemetry.io/collector/component                                           v1.54.0
    go.opentelemetry.io/collector/component/componentstatus                           v0.148.0
    go.opentelemetry.io/collector/config/configgrpc                                   v0.148.0
    go.opentelemetry.io/collector/config/configretry                                  v1.54.0
    go.opentelemetry.io/collector/config/configtls                                    v1.54.0
    go.opentelemetry.io/collector/consumer                                            v1.54.0
    go.opentelemetry.io/collector/pdata                                               v1.54.0
    go.opentelemetry.io/collector/receiver                                            v1.54.0
    go.opentelemetry.io/collector/receiver/receiverhelper                             v0.148.0
    go.opentelemetry.io/collector/receiver/receivertest                               v0.148.0
    go.uber.org/zap                                                                   v1.27.1
    google.golang.org/grpc                                                            v1.79.3
    google.golang.org/protobuf                                                        v1.36.11
)
```

### Narrow gRPC Client Interface for Mocking
```go
// Source: CONTEXT.md decision on gRPC mocking
// Define in a file like internal_test.go or a separate interface file

type tetragonClient interface {
    GetEvents(ctx context.Context, in *tetragonv1.GetEventsRequest, opts ...grpc.CallOption) (
        tetragonv1.FineGuidanceSensors_GetEventsClient, error)
    GetVersion(ctx context.Context, in *tetragonv1.GetVersionRequest, opts ...grpc.CallOption) (
        *tetragonv1.GetVersionResponse, error)
}

// Real implementation wraps the generated client:
type realTetragonClient struct {
    tetragonv1.FineGuidanceSensorsClient
}

// Mock for tests:
type mockTetragonClient struct {
    events []*tetragonv1.GetEventsResponse
    err    error
}
```

### Severity Mapping
```go
// Source: SPEC.md + REQUIREMENTS.md CONV-04
// Source: plog package (pkg.go.dev/go.opentelemetry.io/collector/pdata v1.54.0)
func setSeverity(lr plog.LogRecord, resp *tetragonv1.GetEventsResponse) {
    switch resp.GetEvent().(type) {
    case *tetragonv1.GetEventsResponse_ProcessExec,
        *tetragonv1.GetEventsResponse_ProcessExit,
        *tetragonv1.GetEventsResponse_ProcessLoader:
        lr.SetSeverityNumber(plog.SeverityNumberInfo)
        lr.SetSeverityText("INFO")
    case *tetragonv1.GetEventsResponse_ProcessKprobe,
        *tetragonv1.GetEventsResponse_ProcessTracepoint,
        *tetragonv1.GetEventsResponse_ProcessLsm,
        *tetragonv1.GetEventsResponse_ProcessUprobe,
        *tetragonv1.GetEventsResponse_ProcessUsdt:
        lr.SetSeverityNumber(plog.SeverityNumberWarn)
        lr.SetSeverityText("WARN")
    case *tetragonv1.GetEventsResponse_ProcessThrottle,
        *tetragonv1.GetEventsResponse_RateLimitInfo:
        lr.SetSeverityNumber(plog.SeverityNumberError)
        lr.SetSeverityText("ERROR")
    default:
        lr.SetSeverityNumber(plog.SeverityNumberInfo)
        lr.SetSeverityText("INFO")
    }
}
```

### Timestamp Extraction from GetEventsResponse
```go
// Source: SPEC.md event-to-OTLP mapping table
// GetEventsResponse has a top-level `time` field (google.protobuf.Timestamp)
func setTimestamps(lr plog.LogRecord, resp *tetragonv1.GetEventsResponse) {
    lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))
    if t := resp.GetTime(); t != nil {
        lr.SetTimestamp(pcommon.NewTimestampFromTime(t.AsTime()))
    }
}
```

### Config YAML Format (user-facing)
```yaml
# Source: CONTEXT.md + configgrpc squash pattern
receivers:
  tetragon:
    endpoint: "localhost:54321"
    tls:
      insecure: true
    retry:
      enabled: true
      initial_interval: 1s
      max_interval: 30s
      max_elapsed_time: 0
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `obsreport.Receiver` | `receiverhelper.ObsReport` | ~v0.85.0 | obsreport module removed from registry; must use receiverhelper |
| `host` passed to `ToClientConn` | `host.GetExtensions()` (map) | v0.140+ | Compile error if passing host directly |
| Custom TLS + Endpoint Config struct | `configgrpc.ClientConfig` squash | Established pattern, formalized ~v0.100 | 100+ lines of boilerplate eliminated |
| OTel Collector v0.120.0 (SPEC.md) | v1.54.0 / v0.148.0 (2026-03-17) | Latest release | SPEC.md versions are 28+ releases stale |

**Deprecated/outdated:**
- `go.opentelemetry.io/collector/obsreport`: Removed — use `receiver/receiverhelper`
- SPEC.md's `go = "1.23"` in mise.toml: Use 1.24 per CONTEXT.md decision
- SPEC.md's v0.120.0 OTel versions: Use v1.54.0 / v0.148.0

---

## Open Questions

1. **protojson output stability warning**
   - What we know: `protojson` documentation warns "output is not stable and will change across different program builds." The Tetragon source uses `UseProtoNames: true` and it has been stable in practice.
   - What's unclear: Whether protojson's instability warning affects the byte-for-byte comparison in test success criterion #1.
   - Recommendation: Use `IgnoreObservedTimestamp()` in plogtest.CompareLogs. For the body JSON comparison, verify the golden file is written from real Tetragon output and update it if protojson serialization changes. The instability refers to map key ordering and Timestamp/Duration formatting; for simple message types this is stable in practice.

2. **GetEventsResponse.time field availability**
   - What we know: SPEC.md and REQUIREMENTS.md both reference a `time` field on GetEventsResponse. The Tetragon proto has this field.
   - What's unclear: Whether all 10 event types always populate `time`, or if some may return zero/nil timestamps.
   - Recommendation: Check the captured reference JSON. If `time` is missing for some event types, fall back to `process.start_time` for exec, or use `ObservedTimestamp` only and log a debug message.

3. **ToClientConn TelemetrySettings parameter**
   - What we know: The verified signature is `ToClientConn(ctx, extensions map, settings component.TelemetrySettings, opts...)`.
   - What's unclear: How to get `component.TelemetrySettings` in `Start()` when the receiver struct holds `receiver.Settings` (which contains `TelemetrySettings`).
   - Recommendation: Store `receiver.Settings` (or just `settings.TelemetrySettings`) on the struct during `createLogsReceiver()`. Pass `r.settings.TelemetrySettings` in `Start()`.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package + `github.com/stretchr/testify` v1.11.1 |
| Config file | None required — `go test` runs directly |
| Quick run command | `go test ./... -count=1` (from receiver/tetragonreceiver/) |
| Full suite command | `go test -race -count=1 ./...` (from receiver/tetragonreceiver/) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RECV-01 | Factory registers as logs receiver | unit | `go test -run TestNewFactory ./...` | Wave 0 |
| RECV-02 | Start() returns immediately, spawns goroutine | unit | `go test -run TestReceiverStart ./...` | Wave 0 |
| RECV-03 | Shutdown() cancels and waits cleanly | unit | `go test -run TestReceiverShutdown ./...` | Wave 0 |
| RECV-04 | Streams via GetEvents RPC | unit | `go test -run TestStreamEvents ./...` | Wave 0 |
| RECV-05 | Reconnects with backoff on error | unit | `go test -run TestReconnect ./...` | Wave 0 |
| RECV-06 | Clean shutdown vs transient error | unit | `go test -run TestShutdownVsError ./...` | Wave 0 |
| RECV-07 | Logs connection events via zap | unit | `go test -run TestLogging ./...` | Wave 0 |
| RECV-08 | obsreport accepted/refused counters | unit | `go test -run TestObsReport ./...` | Wave 0 |
| CONF-01 | Config struct with gRPC fields | unit | `go test -run TestConfig ./...` | Wave 0 |
| CONF-02 | Validate rejects empty endpoint, bad TLS | unit | `go test -run TestConfigValidate ./...` | Wave 0 |
| CONF-03 | Default config has localhost:54321, insecure | unit | `go test -run TestDefaultConfig ./...` | Wave 0 |
| CONF-04 | YAML round-trip via confmaptest | unit | `go test -run TestConfigYAML ./...` | Wave 0 |
| CONV-01 through CONV-10 | All event types → correct LogRecord | unit | `go test -run TestConvert ./...` | Wave 0 |
| PROJ-01 | mise.toml with test/build tasks | manual | `mise run test` | Wave 0 |
| PROJ-03 | metadata.yaml exists with correct fields | manual | file check | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -count=1 ./...`
- **Per wave merge:** `go test -race -count=1 ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `receiver/tetragonreceiver/` — module does not exist yet; create go.mod and all source files
- [ ] `receiver/tetragonreceiver/testdata/events/*.json` — must be captured from live Tetragon before convert tests can run
- [ ] `receiver/tetragonreceiver/testdata/golden/*.yaml` — generated by converter tests on first run with `-update` flag
- [ ] `.mise/config.toml` — needs update from `go = "prefix:1.20"` to `go = "1.24"` and task definitions added
- [ ] Framework install: `go` via mise — already configured but version needs update

---

## Sources

### Primary (HIGH confidence)
- Go module proxy (proxy.golang.org) — all module versions verified 2026-03-17/18
- `pkg.go.dev/go.opentelemetry.io/collector/config/configgrpc` — ClientConfig struct, ToClientConn signature
- `pkg.go.dev/go.opentelemetry.io/collector/receiver/receiverhelper` — ObsReport, NewObsReport, StartLogsOp/EndLogsOp
- `pkg.go.dev/go.opentelemetry.io/collector/component/componentstatus` — ReportStatus, NewRecoverableErrorEvent
- `pkg.go.dev/go.opentelemetry.io/collector/receiver/receivertest` — NewNopSettings
- `pkg.go.dev/github.com/open-telemetry/opentelemetry-collector-contrib/pkg/golden` — ReadLogs/WriteLogs
- `pkg.go.dev/github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest/plogtest` — CompareLogs options
- `pkg.go.dev/google.golang.org/protobuf/encoding/protojson` — MarshalOptions fields
- `github.com/cilium/tetragon/blob/d2c40f20/pkg/encoder/encoder.go` — `UseProtoNames: true` confirmed with comment
- `mise.jdx.dev/tasks/toml-tasks.html` — TOML task syntax

### Secondary (MEDIUM confidence)
- `opentelemetry.io/docs/collector/extend/custom-component/receiver/` — Factory + lifecycle pattern
- `github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/kafkareceiver` — componentstatus pattern reference
- `oneuptime.com/blog/post/2026-02-06-build-custom-receiver-opentelemetry-collector/view` — Full receiver lifecycle example (Feb 2026)

### Tertiary (LOW confidence)
- WebSearch result confirming obsreport deprecation at v0.85.0 — verified against registry (obsreport module not found)

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all versions verified from Go module proxy
- Architecture: HIGH — patterns confirmed from official pkg.go.dev docs and OTel source
- Pitfalls: HIGH — sourced from STATE.md decisions (validated pre-phase), Tetragon source, and API change verification
- protojson UseProtoNames: HIGH — confirmed directly from Tetragon encoder.go source with explanatory comment

**Research date:** 2026-03-18
**Valid until:** 2026-04-18 (OTel releases roughly every 2 weeks; versions will be stale but patterns remain stable)
