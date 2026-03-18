# Roadmap: OTel Collector Tetragon Receiver

## Overview

Three phases deliver a custom OTel Collector that consumes Tetragon security events via gRPC streaming instead of the current fragile filelog approach. Phase 1 builds and tests the standalone receiver Go module — the foundation everything else depends on. Phase 2 assembles the OCB custom collector distribution and packages it as a container image. Phase 3 wires up CI and publishes the multi-arch image to GHCR, making it a drop-in replacement deployable from any pipeline.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Receiver Module** - Standalone tetragonreceiver Go module: factory, config, gRPC stream loop, event converter, tests
- [ ] **Phase 2: Distribution** - OCB-built custom collector binary in a multi-stage container image with example config
- [ ] **Phase 3: CI/CD and Release** - GitHub Actions pipeline publishing multi-arch image to GHCR on main/tag push

## Phase Details

### Phase 1: Receiver Module
**Goal**: A standalone, tested Go module that streams Tetragon events to an OTel pipeline
**Depends on**: Nothing (first phase)
**Requirements**: RECV-01, RECV-02, RECV-03, RECV-04, RECV-05, RECV-06, RECV-07, RECV-08, CONF-01, CONF-02, CONF-03, CONF-04, CONV-01, CONV-02, CONV-03, CONV-04, CONV-05, CONV-06, CONV-07, CONV-08, CONV-09, CONV-10, PROJ-01, PROJ-03
**Success Criteria** (what must be TRUE):
  1. `go test ./...` passes in `receiver/tetragonreceiver/` with no failures, including a test that asserts protojson body output matches a captured Tetragon reference JSON byte-for-byte
  2. The receiver factory registers as a logs receiver and creates a working component via `receivertest` without panicking or returning errors
  3. A synthetic gRPC stream delivering all 10 Tetragon event types (exec, exit, kprobe, tracepoint, loader, uprobe, lsm, usdt, throttle, rate_limit_info) produces one LogRecord per event with correct body, severity, timestamps, and extracted attributes
  4. Calling `Shutdown()` while the stream is active (or before `Start()`) terminates cleanly without blocking, hanging, or triggering reconnects
  5. Config validation rejects an empty endpoint and invalid TLS paths at startup without connecting to any remote
**Plans**: TBD

### Phase 2: Distribution
**Goal**: An OCB-built `otelcol-tetragon` binary running inside a container image that wires the receiver into a full pipeline
**Depends on**: Phase 1
**Requirements**: DIST-01, DIST-02, DIST-03, DIST-04, DIST-05, PROJ-02
**Success Criteria** (what must be TRUE):
  1. `ocb --config distribution/builder-config.yaml` produces a binary that starts without error and registers the tetragonreceiver component alongside journaldreceiver, batch, resourcedetection, otlphttp, health_check, and file_storage
  2. The container image starts and the health_check endpoint responds 200 when given the example `rootfs/etc/otelcol/config.yaml` without any volume mounts or special permissions beyond the otel:10001 user
  3. The container image is a drop-in replacement: same entrypoint path, config path, and runtime user as the current otelcol-contrib image
**Plans**: TBD

### Phase 3: CI/CD and Release
**Goal**: A GitHub Actions pipeline that tests, builds, and publishes the multi-arch container image automatically
**Depends on**: Phase 2
**Requirements**: CICD-01, CICD-02, CICD-03, CICD-04, CICD-05, PROJ-04
**Success Criteria** (what must be TRUE):
  1. A pull request triggers the workflow, runs tests and builds the image, and exits without pushing to GHCR
  2. A merge to main pushes an image tagged `latest` and `sha-<commit>` to GHCR
  3. Pushing a semver tag (e.g., `v1.0.0`) produces image tags `1.0.0`, `1.0`, and `latest` on GHCR
  4. The published image manifest contains both `linux/amd64` and `linux/arm64` layers (verified via `docker manifest inspect`)
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Receiver Module | 0/TBD | Not started | - |
| 2. Distribution | 0/TBD | Not started | - |
| 3. CI/CD and Release | 0/TBD | Not started | - |
