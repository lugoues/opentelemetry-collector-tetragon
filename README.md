# otelcol-tetragon

A custom OpenTelemetry Collector distribution with a native Tetragon gRPC receiver for streaming security events into the OTel pipeline without filesystem coupling.

![CI](https://github.com/cilium/otelcol-tetragon/actions/workflows/ci.yml/badge.svg)

## Overview

`otelcol-tetragon` replaces the fragile filelog approach by connecting directly to Tetragon's gRPC `FineGuidanceSensors.GetEvents` streaming RPC. It streams all 10 event types (exec, exit, kprobe, tracepoint, loader, uprobe, lsm, usdt, throttle, rate_limit_info) as OTel LogRecords with full protojson body, extracted attributes, and proper severity mapping.

The collector is published as a multi-arch container image to the GitHub Container Registry and can be used as a drop-in replacement for any filelog-based Tetragon event pipeline.

## Usage

```bash
docker pull ghcr.io/cilium/otelcol-tetragon:latest
docker run --rm -v ./config.yaml:/etc/otelcol/config.yaml ghcr.io/cilium/otelcol-tetragon:latest
```

The container runs as non-root user `otel` (UID/GID 10001 by default). The health check extension listens on port 13133.

To build with a custom UID/GID:

```bash
docker build --build-arg UID=1000 --build-arg GID=1000 -f container/Containerfile .
```

## Configuration Reference

The `tetragon` receiver accepts the following configuration fields:

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

Minimal configuration example:

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

An example two-pipeline config (Tetragon + journald) is included at `container/rootfs/etc/otelcol/config.yaml`.

## Development Setup

### Prerequisites

Install [mise](https://mise.jdx.dev/) (runtime manager):

```bash
curl https://mise.run | sh
```

### Getting Started

```bash
git clone https://github.com/cilium/otelcol-tetragon.git
cd otelcol-tetragon
mise install        # installs Go 1.25
```

### Tasks

All development tasks are available via mise:

```bash
mise run test       # run receiver tests with race detector
mise run lint       # run go vet on receiver module
mise run build      # build receiver module
mise run tidy       # tidy receiver module dependencies
mise run ocb        # build custom collector binary (output: /tmp/otelcol-tetragon/)
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
| journaldreceiver | receiver | opentelemetry-collector-contrib |
| batchprocessor | processor | opentelemetry-collector core |
| resourcedetectionprocessor | processor | opentelemetry-collector-contrib |
| otlphttpexporter | exporter | opentelemetry-collector core |
| healthcheckextension | extension | opentelemetry-collector-contrib |
| filestorage | extension | opentelemetry-collector-contrib |

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
