# Spec: Custom OpenTelemetry Collector Receiver for Tetragon gRPC Events

## Problem

Tetragon's JSON log export (`tetragon.log`) is the current integration point for getting events into an OTel Collector pipeline via the `filelog` receiver. This has three problems:

1. **Permissions** — Tetragon writes the log as root with mode 600. Cross-container access requires host bind mounts, a dedicated group, and POSIX ACLs with default inheritance. Fragile and adds host-level setup.
2. **Security** — The collector container needs read access to a directory written by a fully-privileged eBPF agent. Any file-sharing mechanism widens the attack surface.
3. **Performance** — On cloud servers with network-attached storage, the write-to-disk-then-read-back path adds latency and IOPS. Tetragon already holds events in memory before serializing to disk.

Tetragon exposes a gRPC streaming API (`FineGuidanceSensors.GetEvents`) on `localhost:54321`. A custom OTel Collector receiver that consumes this stream directly eliminates all three issues: no shared filesystem, no ACLs, no disk I/O for event transfer.

There is no existing receiver in `otel-collector-contrib` for Tetragon (open issue [cilium/tetragon#1419](https://github.com/cilium/tetragon/issues/1419), 3+ years old, no implementation).


## Deliverable

A standalone GitHub repository that produces a container image published to GHCR. The image is a custom OTel Collector distribution (built with OCB) that includes the Tetragon receiver alongside the standard contrib components we need (journald, batch, resourcedetection, otlphttp, health_check, file_storage).

The image is a drop-in replacement for the current `otelcol-contrib`-based image — same entrypoint, same config file path, same runtime user. Consumers just pull the new image and update their OTel Collector config to use `tetragon:` instead of `filelog/tetragon:`.


## Architecture

```
┌──────────────┐  gRPC stream   ┌──────────────────────────┐  OTLP/HTTP  ┌─────────────┐
│   Tetragon   │───────────────▶│  OTel Collector          │────────────▶│ OpenObserve │
│  :54321      │  GetEvents     │  tetragonreceiver        │             │  :5080      │
│              │                │  ├─ logs/tetragon pipeline│             │             │
└──────────────┘                │  └─ batch → otlphttp     │             └─────────────┘
                                └──────────────────────────┘
```

The receiver runs as a logs receiver inside the OTel Collector. It connects to Tetragon's gRPC endpoint as a client, opens a `GetEvents` server-streaming RPC, and converts each `GetEventsResponse` into an OTLP `plog.LogRecord` before passing it to the collector pipeline.


## Tetragon gRPC API

### Service

**Package:** `github.com/cilium/tetragon/api/v1/tetragon`

**Service:** `FineGuidanceSensors` (defined in `sensors.proto`)

**RPC:** `GetEvents(GetEventsRequest) returns (stream GetEventsResponse)`

**Default endpoint:** `localhost:54321` (configurable via `--server-address` on the Tetragon daemon)

### GetEventsRequest

```proto
message GetEventsRequest {
  repeated Filter allow_list = 1;   // Include only matching events
  repeated Filter deny_list = 2;    // Exclude matching events
  AggregationOptions aggregation_options = 3;
  repeated FieldFilter field_filters = 4;
}
```

The `Filter` message supports binary regex, namespace, health, pod regex, PID, event type, pod label selectors, capability filters, container ID/name, CEL expressions, and ancestor binary patterns. For v1, send an empty request (all events, no filtering). Server-side filtering can be exposed as config options later.

### GetEventsResponse

The response uses a `oneof event` field:

| Field name          | Message type       | EventType enum value |
|---------------------|--------------------|----------------------|
| `process_exec`      | `ProcessExec`      | `PROCESS_EXEC = 1`  |
| `process_exit`      | `ProcessExit`      | `PROCESS_EXIT = 5`  |
| `process_kprobe`    | `ProcessKprobe`    | `PROCESS_KPROBE = 9`|
| `process_tracepoint`| `ProcessTracepoint`| `PROCESS_TRACEPOINT = 10` |
| `process_loader`    | `ProcessLoader`    | `PROCESS_LOADER = 11` |
| `process_uprobe`    | `ProcessUprobe`    | `PROCESS_UPROBE = 12` |
| `process_throttle`  | `ProcessThrottle`  | `PROCESS_THROTTLE = 27` |
| `process_lsm`       | `ProcessLsm`       | `PROCESS_LSM = 28`  |
| `process_usdt`       | `ProcessUsdt`       | `PROCESS_USDT = 29` |
| `rate_limit_info`   | `RateLimitInfo`    | `RATE_LIMIT_INFO = 40001` |

Every event type except `rate_limit_info` has a `Process process` field and most have `Process parent` and `repeated Process ancestors`.

### Process Message (key fields)

```proto
message Process {
  string exec_id = 1;
  UInt32Value pid = 2;
  UInt32Value uid = 3;
  string cwd = 4;
  string binary = 5;
  string arguments = 6;
  string flags = 7;
  Timestamp start_time = 8;
  UInt32Value auid = 9;
  Pod pod = 10;
  string docker = 11;
  string parent_exec_id = 12;
  Capabilities cap = 14;
  Namespaces ns = 15;
  UInt32Value tid = 16;
  ProcessCredentials process_credentials = 17;
  BinaryProperties binary_properties = 18;
  UserRecord user = 19;
}
```


## Event-to-OTLP Mapping

Each `GetEventsResponse` becomes one `plog.LogRecord`. The mapping preserves the full event as a structured body while extracting key fields into OTLP resource/log attributes for indexing and filtering.

### Resource Attributes

Set once when the receiver starts (or per-event if Kubernetes metadata is present):

| OTLP Resource Attribute        | Source                          |
|--------------------------------|---------------------------------|
| `host.name`                    | Set by `resourcedetection` processor (not this receiver) |
| `tetragon.version`             | From `GetVersion()` RPC at startup |

### Log Record Fields

| OTLP Log Field      | Source                                              |
|----------------------|-----------------------------------------------------|
| `Timestamp`          | `GetEventsResponse.time` (if present) or `process.start_time` for exec events, `time` field for exit events |
| `ObservedTimestamp`  | `time.Now()` at receive time                        |
| `SeverityNumber`     | `INFO` for exec/exit/loader, `WARN` for kprobe/tracepoint/lsm with action, `ERROR` for throttle/rate_limit |
| `SeverityText`       | Corresponding text (`INFO`, `WARN`, `ERROR`)        |
| `Body`               | JSON serialization of the full `GetEventsResponse` (preserves all data for downstream query) |

### Log Record Attributes

| Attribute Key                    | Source                                    |
|----------------------------------|-------------------------------------------|
| `event.domain`                   | `"tetragon"` (static)                     |
| `event.name`                     | Event type as string: `"process_exec"`, `"process_exit"`, `"process_kprobe"`, etc. |
| `tetragon.event_type`            | `EventType` enum integer value            |
| `tetragon.process.binary`        | `process.binary`                          |
| `tetragon.process.arguments`     | `process.arguments`                       |
| `tetragon.process.pid`           | `process.pid`                             |
| `tetragon.process.uid`           | `process.uid`                             |
| `tetragon.process.exec_id`       | `process.exec_id`                         |
| `tetragon.process.cwd`           | `process.cwd`                             |
| `tetragon.parent.binary`         | `parent.binary` (if present)              |
| `tetragon.parent.pid`            | `parent.pid` (if present)                 |
| `tetragon.parent.exec_id`        | `parent.exec_id` (if present)             |
| `tetragon.policy_name`           | `policy_name` (kprobe/tracepoint/lsm only)|
| `tetragon.action`                | `action` string (kprobe/tracepoint/lsm)   |
| `tetragon.function_name`         | `function_name` (kprobe only)             |
| `tetragon.subsys`                | `subsys` (tracepoint only)                |
| `tetragon.event`                 | `event` (tracepoint only)                 |
| `tetragon.exit.status`           | `status` (exit only)                      |
| `tetragon.exit.signal`           | `signal` (exit only)                      |

### Kubernetes Attributes (if `process.pod` is populated)

| Attribute Key           | Source              |
|-------------------------|---------------------|
| `k8s.namespace.name`    | `pod.namespace`     |
| `k8s.pod.name`          | `pod.name`          |
| `k8s.container.name`    | `pod.container.name`|

### Design Rationale

The full event JSON goes in `Body` because Tetragon events are deeply nested (ancestors chains, kprobe arguments, stack traces) and lossy extraction into flat attributes would discard data. The extracted attributes enable efficient filtering and indexing in OpenObserve without requiring full-body JSON parsing at query time. This mirrors how the `filelog` receiver currently ingests the raw JSON — downstream queries already work against the JSON body.

Use `protojson.Marshal` (from `google.golang.org/protobuf/encoding/protojson`) for the JSON body — this produces canonical proto3 JSON that matches Tetragon's own JSON export format, so existing OpenObserve queries against the filelog-ingested data remain compatible.


## Receiver Configuration

```yaml
receivers:
  tetragon:
    # Tetragon gRPC endpoint
    endpoint: "localhost:54321"

    # Connection settings
    tls:
      insecure: true           # Default: true (localhost)
      # ca_file: /path/to/ca.pem
      # cert_file: /path/to/cert.pem
      # key_file: /path/to/key.pem

    # Reconnection backoff
    retry:
      initial_interval: 1s
      max_interval: 30s
      max_elapsed_time: 0      # 0 = retry forever

    # Server-side event filtering (maps to GetEventsRequest filters)
    # Optional, empty = all events
    allow_list: []
    deny_list: []
```

### Config Struct

```go
type Config struct {
    Endpoint  string                       `mapstructure:"endpoint"`
    TLS       configtls.ClientConfig       `mapstructure:"tls"`
    Retry     RetryConfig                  `mapstructure:"retry"`
    AllowList []tetragon.Filter            `mapstructure:"allow_list"`
    DenyList  []tetragon.Filter            `mapstructure:"deny_list"`
}

type RetryConfig struct {
    InitialInterval time.Duration `mapstructure:"initial_interval"`
    MaxInterval     time.Duration `mapstructure:"max_interval"`
    MaxElapsedTime  time.Duration `mapstructure:"max_elapsed_time"`
}
```

### Defaults

```go
func createDefaultConfig() component.Config {
    return &Config{
        Endpoint: "localhost:54321",
        TLS: configtls.ClientConfig{
            Insecure: true,
        },
        Retry: RetryConfig{
            InitialInterval: 1 * time.Second,
            MaxInterval:     30 * time.Second,
            MaxElapsedTime:  0,
        },
    }
}
```


## Repository Structure

```
otelcol-tetragon/
├── .github/
│   └── workflows/
│       └── build.yaml              # Build + push to GHCR
├── receiver/
│   └── tetragonreceiver/
│       ├── go.mod
│       ├── go.sum
│       ├── metadata.yaml           # OTel component metadata
│       ├── config.go               # Config struct + Validate()
│       ├── config_test.go
│       ├── factory.go              # NewFactory(), createDefaultConfig(), createLogsReceiver()
│       ├── factory_test.go
│       ├── receiver.go             # tetragonReceiver: Start(), Shutdown(), stream loop
│       ├── receiver_test.go
│       ├── convert.go              # GetEventsResponse → plog.LogRecord mapping
│       ├── convert_test.go
│       ├── doc.go                  # Package documentation
│       └── testdata/
│           ├── config.yaml         # Test config fixtures
│           └── events/             # Sample Tetragon event JSON for golden file tests
│               ├── process_exec.json
│               ├── process_exit.json
│               └── process_kprobe.json
├── builder-config.yaml             # OCB manifest
├── Containerfile                   # Multi-stage: Go build → Debian runtime
├── rootfs/
│   └── etc/otelcol/
│       └── config.yaml             # Default/example collector config (not baked in as entrypoint default)
├── go.mod                          # Top-level module (for OCB replaces directive)
├── go.sum
├── README.md
└── LICENSE
```

### metadata.yaml

```yaml
type: tetragon

status:
  class: receiver
  stability:
    alpha: [logs]
```

### Key Dependencies

```go
require (
    github.com/cilium/tetragon/api  v1.x.x      // Tetragon gRPC client + proto types
    go.opentelemetry.io/collector/component       // component.Component interface
    go.opentelemetry.io/collector/consumer        // consumer.Logs interface
    go.opentelemetry.io/collector/receiver        // receiver.Factory, receiver.Settings
    go.opentelemetry.io/collector/pdata           // plog.Logs, plog.LogRecord
    go.opentelemetry.io/collector/config/configtls
    google.golang.org/grpc
    go.uber.org/zap
)
```


## Receiver Implementation

### Lifecycle

```go
type tetragonReceiver struct {
    cfg       *Config
    logger    *zap.Logger
    consumer  consumer.Logs
    conn      *grpc.ClientConn
    cancel    context.CancelFunc
    wg        sync.WaitGroup
}
```

**`Start(ctx, host)`:**
1. Dial Tetragon gRPC endpoint with TLS config
2. Call `GetVersion()` to verify connectivity and log Tetragon version
3. Spawn goroutine: `streamEvents(ctx)`

**`streamEvents(ctx)`:**
```
loop:
    client.GetEvents(ctx, &GetEventsRequest{AllowList, DenyList})
    for {
        resp, err := stream.Recv()
        if err != nil:
            if ctx.Done(): return
            log.Warn("stream error, reconnecting", err)
            backoff()
            continue loop
        logs := convertEvent(resp)
        consumer.ConsumeLogs(ctx, logs)
    }
```

The reconnection loop uses exponential backoff (configured via `retry`). On context cancellation (shutdown), the loop exits cleanly.

**`Shutdown(ctx)`:**
1. Cancel the stream context
2. `wg.Wait()` for the stream goroutine to exit
3. Close the gRPC connection

### Event Conversion (`convert.go`)

```go
func convertEvent(resp *tetragon.GetEventsResponse) plog.Logs {
    logs := plog.NewLogs()
    rl := logs.ResourceLogs().AppendEmpty()
    sl := rl.ScopeLogs().AppendEmpty()
    sl.Scope().SetName("tetragonreceiver")
    lr := sl.LogRecords().AppendEmpty()

    // Set timestamps
    lr.SetTimestamp(...)
    lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))

    // Set severity based on event type
    setSeverity(lr, resp)

    // Set body to full JSON
    jsonBytes, _ := protojson.Marshal(resp)
    lr.Body().SetStr(string(jsonBytes))

    // Extract attributes
    attrs := lr.Attributes()
    attrs.PutStr("event.domain", "tetragon")
    attrs.PutStr("event.name", eventTypeName(resp))

    // Extract process fields
    if proc := extractProcess(resp); proc != nil {
        attrs.PutStr("tetragon.process.binary", proc.GetBinary())
        attrs.PutStr("tetragon.process.arguments", proc.GetArguments())
        attrs.PutInt("tetragon.process.pid", int64(proc.GetPid().GetValue()))
        // ... etc
    }

    // Extract event-type-specific fields
    extractEventSpecificAttrs(attrs, resp)

    return logs
}
```


## OTel Collector Builder (OCB) Configuration

### builder-config.yaml

```yaml
dist:
  name: otelcol-tetragon
  description: OTel Collector with Tetragon gRPC receiver
  output_path: /tmp/otelcol-tetragon

extensions:
  - gomod: go.opentelemetry.io/collector/extension/healthcheckextension v0.120.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage v0.120.0

receivers:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/journaldreceiver v0.120.0
  - gomod: github.com/YOUR_ORG/otelcol-tetragon/receiver/tetragonreceiver v0.1.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.120.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor v0.120.0

exporters:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/exporter/otlphttpexporter v0.120.0

replaces:
  - github.com/YOUR_ORG/otelcol-tetragon/receiver/tetragonreceiver => ./receiver/tetragonreceiver
```

The `replaces` directive lets OCB resolve the receiver from the local source tree during build, avoiding the need to publish the Go module to a registry before building the image.

Version numbers should match across all components. Pin to a single OTel Collector release (currently `0.120.0`).


## Containerfile

Multi-stage build: compile the custom collector with OCB in a Go builder stage, copy the binary into a Debian runtime with journalctl and microcheck.

```dockerfile
ARG OTEL_COLLECTOR_VERSION=0.120.0
ARG GO_VERSION=1.23

FROM ghcr.io/tarampampam/microcheck:1 AS healthcheck

# --- Build stage ---
FROM docker.io/library/golang:${GO_VERSION}-bookworm AS builder

ARG OTEL_COLLECTOR_VERSION

# Install OCB
RUN go install go.opentelemetry.io/collector/cmd/builder@v${OTEL_COLLECTOR_VERSION}

WORKDIR /build
COPY . .

RUN builder --config builder-config.yaml --output-path /tmp/dist

# --- Runtime stage ---
FROM docker.io/library/debian:13-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends systemd ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd --system --gid 10001 otel && \
    useradd --system --uid 10001 --gid otel otel && \
    usermod -aG systemd-journal otel

COPY --from=builder /tmp/dist/otelcol-tetragon /usr/local/bin/otelcol-tetragon
COPY --from=healthcheck /bin/httpcheck /bin/httpcheck

USER otel

ENTRYPOINT ["/usr/local/bin/otelcol-tetragon"]
CMD ["--config", "/etc/otelcol/config.yaml"]
```

No config is baked into the image. The consumer mounts their own config at `/etc/otelcol/config.yaml` (or overrides `CMD`).


## GitHub Actions: Build and Publish to GHCR

### .github/workflows/build.yaml

```yaml
name: Build and Publish

on:
  push:
    branches: [main]
    tags: ["v*"]
  pull_request:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"

      - name: Run receiver tests
        working-directory: receiver/tetragonreceiver
        run: go test -v -race ./...

  build:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - uses: actions/checkout@v4

      - uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/metadata-action@v5
        id: meta
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=sha
            type=raw,value=latest,enable={{is_default_branch}}

      - uses: docker/setup-buildx-action@v3

      - uses: docker/build-push-action@v6
        with:
          context: .
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          platforms: linux/amd64,linux/arm64
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

### Workflow Behavior

- **Pull requests**: Run tests, build image (no push). Validates the build works on both amd64 and arm64.
- **Push to main**: Run tests, build and push with `latest` + `sha-<commit>` tags.
- **Tags (`v*`)**: Run tests, build and push with semver tags (`v1.2.3` → `1.2.3`, `1.2`, `latest`).
- **Multi-arch**: Builds for `linux/amd64` and `linux/arm64` using buildx. The Go build stage handles cross-compilation natively.
- **Cache**: Uses GitHub Actions cache (`type=gha`) to avoid re-downloading Go modules and re-compiling unchanged layers.


## Example Collector Config

This is the example config that ships in `rootfs/etc/otelcol/config.yaml` for reference. Not baked into the image — mount your own.

```yaml
extensions:
  health_check:
    endpoint: 0.0.0.0:13133

  file_storage/checkpoint:
    directory: /var/lib/otelcol/checkpoint

receivers:
  tetragon:
    endpoint: "tetragon:54321"
    tls:
      insecure: true

  journald:
    directory: /var/log/journal
    priority: info
    storage: file_storage/checkpoint

processors:
  batch:
    timeout: 5s
    send_batch_size: 1024

  resourcedetection:
    detectors: [system]
    system:
      hostname_sources: [os]

exporters:
  otlphttp/openobserve:
    endpoint: http://openobserve:5080/api/default
    headers:
      Authorization: Basic ${env:OTEL_AUTH}
      stream-name: default

service:
  extensions: [health_check, file_storage/checkpoint]
  pipelines:
    logs/tetragon:
      receivers: [tetragon]
      processors: [resourcedetection, batch]
      exporters: [otlphttp/openobserve]
    logs/journal:
      receivers: [journald]
      processors: [resourcedetection, batch]
      exporters: [otlphttp/openobserve]
```


## Testing Strategy

### Unit Tests

- **`config_test.go`**: Validate config parsing, defaults, and validation errors (missing endpoint, etc.)
- **`convert_test.go`**: Test each event type conversion with golden file comparison. Use the sample events from `testdata/events/` (captured from a real Tetragon instance via `tetra getevents -o json`). Verify:
  - Correct attribute extraction for each event type
  - JSON body matches `protojson.Marshal` output
  - Severity mapping is correct
  - Kubernetes attributes populated when pod info present, omitted when not
  - Nil-safe access to optional fields (parent, ancestors)
- **`receiver_test.go`**: Test Start/Shutdown lifecycle, reconnection on stream error (mock gRPC server)

### Integration Test

Run locally with:
1. Tetragon in a container (or a mock gRPC server replaying captured events)
2. Custom OTel Collector built with OCB
3. Verify events arrive at an OTLP endpoint (can use a debug exporter)

### Compatibility Test

Compare output of the new receiver against the existing `filelog/tetragon` pipeline:
1. Run both pipelines in parallel (dual exporters)
2. Verify the JSON body content is equivalent (field ordering may differ between `protojson.Marshal` and Tetragon's own JSON serializer — document any differences)


## Implementation Order

1. **Scaffold** — Repository structure, `go.mod`, `metadata.yaml`, `config.go`, `factory.go` with default config
2. **Convert** — `convert.go` with `ProcessExec` and `ProcessExit` mapping + tests against golden files
3. **Stream** — `receiver.go` with gRPC client, stream loop, reconnection backoff
4. **Extend** — Add remaining event types to `convert.go` (kprobe, tracepoint, loader, uprobe, lsm, usdt, throttle)
5. **OCB** — `builder-config.yaml` with `replaces` directive for local receiver source
6. **Containerfile** — Multi-stage build: Go builder → Debian runtime
7. **CI** — GitHub Actions workflow: test → build → push to GHCR
8. **Test** — Integration test with live Tetragon, verify OpenObserve ingestion, compare with filelog output


## Open Questions

1. **Tetragon version pinning** — The `github.com/cilium/tetragon/api` module version should match the deployed Tetragon version (currently v1.6.0). Proto compatibility across versions needs checking.
2. **Server-side filtering** — Should the receiver expose `allow_list`/`deny_list` in config from day one, or start with all-events and add filtering later?
3. **Graceful degradation** — If Tetragon is not running when the collector starts, the receiver retries forever (via backoff). Should there be a startup timeout that marks the collector unhealthy?
4. **Metrics pipeline** — Tetragon events could also feed a metrics pipeline (e.g., process exec rate, kprobe trigger counts). Out of scope for v1 but worth considering in the module structure.