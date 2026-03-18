---
phase: 02-distribution
plan: 01
subsystem: infra
tags: [ocb, opentelemetry, otelcol, go-module, mise, builder-config, journald, tetragon]

requires:
  - phase: 01-receiver-module
    provides: tetragonreceiver Go module at ./receiver/tetragonreceiver

provides:
  - OCB builder manifest (distribution/builder-config.yaml) registering 7 components
  - Top-level go.mod with replace directive for local tetragonreceiver
  - Example two-pipeline collector config (logs/tetragon + logs/journal)
  - mise tasks for ocb, container, and smoke operations

affects: [02-distribution/02-02, containerfile, deployment]

tech-stack:
  added:
    - "OCB (go.opentelemetry.io/collector/cmd/builder v0.148.0) - collector binary generator"
    - "healthcheckextension v0.148.0 (contrib) - HTTP health endpoint at :13133"
    - "filestorage v0.148.0 (contrib) - checkpoint persistence for journald"
    - "journaldreceiver v0.148.0 (contrib) - systemd journal ingestion"
    - "batchprocessor v0.148.0 (core) - log batching before export"
    - "resourcedetectionprocessor v0.148.0 (contrib) - host metadata attachment"
    - "otlphttpexporter v0.148.0 (core) - OTLP/HTTP export to OpenObserve"
  patterns:
    - "OCB path: field for local module resolution (run from repo root)"
    - "output_path: /tmp/dist to avoid polluting source tree"
    - "--skip-strict-versioning required for local receiver v0.1.0 vs OCB v0.148.0 train"
    - "rootfs/ layout for container config files shipped separately from image"

key-files:
  created:
    - distribution/builder-config.yaml
    - rootfs/etc/otelcol/config.yaml
  modified:
    - go.mod
    - .mise/config.toml

key-decisions:
  - "healthcheckextension is in contrib repo (github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension), NOT core - using wrong path causes module not found"
  - "output_path: /tmp/dist (absolute) keeps OCB-generated files outside source tree"
  - "--skip-strict-versioning required because tetragonreceiver v0.1.0 does not match OCB v0.148.0 version train"
  - "OCB path: ./receiver/tetragonreceiver resolves relative to CWD - must run builder from repo root"

patterns-established:
  - "Pattern 1: Local module via path: field - self-contained, no separate replaces: block needed"
  - "Pattern 2: rootfs/ for example configs - not baked into image, consumers mount their own"

requirements-completed: [DIST-01, DIST-02, PROJ-02]

duration: 2min
completed: 2026-03-18
---

# Phase 2 Plan 01: Distribution Build System Summary

**OCB builder manifest with 7 OTel components, corrected go.mod replace directive, two-pipeline example config, and mise tasks for ocb/container/smoke operations**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-18T13:43:07Z
- **Completed:** 2026-03-18T13:44:39Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Created OCB builder-config.yaml registering tetragonreceiver (local path), journaldreceiver, batchprocessor, resourcedetectionprocessor, otlphttpexporter, healthcheckextension, and filestorage — all at v0.148.0
- Replaced placeholder `module testproto` go.mod with `github.com/cilium/otelcol-tetragon` and replace directive for local receiver
- Created example two-pipeline collector config (logs/tetragon + logs/journal) with health_check extension at :13133 and otlphttp/openobserve exporter
- Added 3 mise tasks (ocb, container, smoke) to the existing 4, bringing total to 7

## Task Commits

Each task was committed atomically:

1. **Task 1: Create OCB builder manifest, go.mod, and example collector config** - `d4b8a3e` (feat)
2. **Task 2: Add OCB, container, and smoke mise tasks** - `ef1d5f2` (feat)

**Plan metadata:** (see final commit below)

## Files Created/Modified

- `distribution/builder-config.yaml` - OCB manifest with all 7 components, output_path /tmp/dist
- `go.mod` - Top-level module declaration with replace directive for tetragonreceiver
- `rootfs/etc/otelcol/config.yaml` - Two-pipeline example config with health_check, file_storage, batch, resourcedetection, otlphttp/openobserve
- `.mise/config.toml` - Added tasks.ocb, tasks.container, tasks.smoke (7 total tasks)

## Decisions Made

- Used `path: ./receiver/tetragonreceiver` (relative to repo root) not `../receiver/tetragonreceiver` — ensures correct resolution both locally and in Docker where WORKDIR is /build
- Set `output_path: /tmp/dist` (absolute) to prevent OCB-generated files from appearing in source tree git status
- Added `--skip-strict-versioning` to ocb task because tetragonreceiver v0.1.0 will not match OCB v0.148.0 strict version check

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- All OCB inputs ready: builder-config.yaml, go.mod replace, local receiver at ./receiver/tetragonreceiver
- Plan 02-02 (Containerfile) can now proceed with multi-stage build using `builder --config distribution/builder-config.yaml --output-path /tmp/dist --skip-strict-versioning`
- Smoke task verifies the built container serves health_check at localhost:13133

---
*Phase: 02-distribution*
*Completed: 2026-03-18*
