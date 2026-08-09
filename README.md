# opentelemetry-collector-tetragon

A custom OpenTelemetry Collector distribution with a native Tetragon gRPC receiver for streaming security events into the OTel pipeline without filesystem coupling.

![CI](https://github.com/lugoues/opentelemetry-collector-tetragon/actions/workflows/ci.yml/badge.svg)

## Overview

`opentelemetry-collector-tetragon` replaces the fragile filelog approach by connecting directly to Tetragon's gRPC `FineGuidanceSensors.GetEvents` streaming RPC. It streams all 11 event types (exec, exit, kprobe, tracepoint, loader, uprobe, lsm, usdt, throttle, rate_limit_info, test) as OTel LogRecords with full protojson body, extracted attributes, and proper severity mapping.

The collector is published as a multi-arch container image to the GitHub Container Registry and can be used as a drop-in replacement for any filelog-based Tetragon event pipeline.

## Usage

```bash
docker pull ghcr.io/lugoues/opentelemetry-collector-tetragon:latest
docker run --rm -v ./config.yaml:/etc/otelcol/config.yaml ghcr.io/lugoues/opentelemetry-collector-tetragon:latest
```

The container uses a [distroless](https://github.com/GoogleContainerTools/distroless) base image and runs as non-root (UID 1000). The health check extension listens on port 13133.

## Configuration

The collector is configured through three mechanisms:

### Config File

Mount a YAML config file into the container at `/etc/otelcol/config.yaml` (the default path), or pass a custom path:

```bash
docker run --rm \
  -v ./my-config.yaml:/etc/otelcol/config.yaml \
  ghcr.io/lugoues/opentelemetry-collector-tetragon:latest

# Or with a custom path
docker run --rm \
  -v ./my-config.yaml:/config.yaml \
  ghcr.io/lugoues/opentelemetry-collector-tetragon:latest --config /config.yaml
```

An example config is included at [`container/rootfs/etc/otelcol/config.yaml`](container/rootfs/etc/otelcol/config.yaml).

### Environment Variables

The OTel Collector supports `${env:VAR_NAME}` substitution in config files. The [example config](container/rootfs/etc/otelcol/config.yaml) uses these variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `TETRAGON_ENDPOINT` | Tetragon gRPC server address | `tetragon.kube-system.svc:54321` |
| `OTEL_EXPORTER_ENDPOINT` | OTLP/HTTP exporter endpoint | `http://openobserve:5080/api/default` |
| `OTEL_AUTH` | Base64-encoded `user:password` for exporter auth header | `dXNlcjpwYXNz` |
| `OTEL_STREAM_NAME` | Value for the exporter's `stream-name` header (target stream in OpenObserve) | `tetragon` |
| `OTEL_LOG_LEVEL` | Collector self-telemetry log level (optional, defaults to `info`) | `debug` |

```bash
docker run --rm \
  -e TETRAGON_ENDPOINT=tetragon.kube-system.svc:54321 \
  -e OTEL_EXPORTER_ENDPOINT=http://openobserve:5080/api/default \
  -e OTEL_AUTH=dXNlcjpwYXNz \
  -e OTEL_STREAM_NAME=tetragon \
  -v ./config.yaml:/etc/otelcol/config.yaml \
  ghcr.io/lugoues/opentelemetry-collector-tetragon:latest
```

You can define your own env vars in custom config files using the same `${env:VAR_NAME}` syntax.

### CLI Flags

The collector binary accepts these flags after the entrypoint:

| Flag | Description |
|------|-------------|
| `--config <path>` | Config file path (default: `/etc/otelcol/config.yaml`) |
| `--config <uri>` | Multiple configs merged in order (e.g., `--config base.yaml --config overrides.yaml`) |
| `--set <key>=<value>` | Override a single config value (e.g., `--set receivers.tetragon.endpoint=10.0.0.1:54321`) |
| `--feature-gates <gate>` | Enable/disable feature gates (e.g., `--feature-gates -component.UseLocalHostAsDefaultHost`) |

```bash
docker run --rm \
  -v ./config.yaml:/etc/otelcol/config.yaml \
  ghcr.io/lugoues/opentelemetry-collector-tetragon:latest \
  --config /etc/otelcol/config.yaml \
  --set receivers.tetragon.endpoint=tetragon.kube-system.svc:54321
```

## Receiver Reference

The `tetragon` receiver accepts the following fields in the config file:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `endpoint` | string | `localhost:54321` | Tetragon gRPC server address |
| `tls.insecure` | bool | `true` | Disable TLS verification |
| `tls.cert_file` | string | - | Path to client TLS certificate |
| `tls.key_file` | string | - | Path to client TLS key |
| `tls.ca_file` | string | - | Path to CA certificate |
| `retry.enabled` | bool | `true` | Enable exponential backoff on stream errors |
| `retry.initial_interval` | duration | `1s` | Initial retry backoff interval |
| `retry.max_interval` | duration | `30s` | Maximum retry backoff interval |
| `retry.max_elapsed_time` | duration | `0` (unlimited) | Maximum total retry time (0 = retry forever) |
| `filters.allow_list` | list | - | Server-side allow filters (see below) |
| `filters.deny_list` | list | - | Server-side deny filters (see below) |

Minimal config example:

```yaml
receivers:
  tetragon:
    endpoint: "tetragon.kube-system.svc:54321"
    tls:
      insecure: true

exporters:
  otlphttp:
    endpoint: http://collector:4318

service:
  pipelines:
    logs:
      receivers: [tetragon]
      exporters: [otlphttp]
```

### Filtering events

`filters` is passed to Tetragon on the `GetEvents` request, so filtering happens
**server-side in Tetragon before events cross the wire**, reducing CPU and network
in addition to downstream storage. Tetragon applies `allow_list` first, then
`deny_list`. Each filter entry supports:

| Field | Type | Description |
|-------|------|-------------|
| `event_set` | list | Event types to match, e.g. `PROCESS_EXEC`, `PROCESS_EXIT`, `PROCESS_KPROBE`, `PROCESS_TRACEPOINT`, `PROCESS_UPROBE`. Unknown names fail validation. |
| `binary_regex` | list | Regexes matched against the process binary path. |

Fields within one filter are ANDed; multiple filters in a list are ORed. Prefer
`deny_list` for noise reduction so event types added later keep flowing by default.

Example — drop the high-volume process lifecycle stream, keep tracing-policy
(kprobe) events:

```yaml
receivers:
  tetragon:
    endpoint: "tetragon.monitoring:54321"
    tls:
      insecure: true
    filters:
      deny_list:
        - event_set: [PROCESS_EXEC, PROCESS_EXIT]
```

## Development Setup

### Prerequisites

Install [mise](https://mise.jdx.dev/) (runtime manager):

```bash
curl https://mise.run | sh
```

### Getting Started

```bash
git clone https://github.com/lugoues/opentelemetry-collector-tetragon.git
cd opentelemetry-collector-tetragon
mise install        # installs Go 1.25
```

### Tasks

All development tasks are available via mise:

```bash
mise run test       # run receiver tests with race detector
mise run lint       # run go vet on receiver module
mise run build      # build receiver module
mise run tidy       # tidy receiver module dependencies
mise run ocb        # build custom collector binary (output: /tmp/otelcol-tetragon/; the task overrides builder-config.yaml's default /tmp/dist)
mise run container  # build container image (otelcol-tetragon:local)
mise run smoke      # smoke test: start container, check health, stop
```

### Building the Container Image

```bash
# Local image
mise run container

# Multi-arch image
docker buildx build --platform linux/amd64,linux/arm64 \
  -t otelcol-tetragon:dev -f container/Containerfile .
```

## Components

This distribution includes the following OpenTelemetry Collector components:

| Component | Type | Source |
|-----------|------|--------|
| tetragonreceiver | receiver | this repo |
| batchprocessor | processor | opentelemetry-collector core |
| memorylimiterprocessor | processor | opentelemetry-collector core |
| resourcedetectionprocessor | processor | opentelemetry-collector-contrib |
| otlpexporter | exporter | opentelemetry-collector core |
| otlphttpexporter | exporter | opentelemetry-collector core |
| debugexporter | exporter | opentelemetry-collector core |
| healthcheckextension | extension | opentelemetry-collector-contrib |
| filestorage | extension | opentelemetry-collector-contrib |

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
