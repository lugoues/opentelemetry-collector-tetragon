---
phase: 02-distribution
plan: 02
subsystem: infra
tags: [containerfile, docker, ocb, opentelemetry, debian, systemd, journald]

# Dependency graph
requires:
  - phase: 02-distribution
    plan: 01
    provides: distribution/builder-config.yaml with otelcol-tetragon binary definition and output_path /tmp/dist

provides:
  - Multi-stage Containerfile: Go+OCB build stage producing otelcol-tetragon binary, Debian-slim runtime with systemd/ca-certificates
  - Non-root otel user at UID/GID 10001 with systemd-journal group membership
  - Drop-in container entrypoint contract: /usr/local/bin/otelcol-tetragon with config at /etc/otelcol/config.yaml

affects:
  - 02-distribution (plan 03 if any smoke test or config bake-in steps)
  - 03-integration (container image used in compose or deployment)

# Tech tracking
tech-stack:
  added:
    - golang:1.25-bookworm (OCB build base)
    - debian:bookworm-slim (runtime base)
    - systemd apt package (provides journalctl binary for journaldreceiver)
    - ca-certificates apt package (TLS root store for OTLP/HTTP export)
  patterns:
    - Multi-stage container build: separate Go builder from Debian-slim runtime
    - OCB binary build in container: WORKDIR /build + COPY . . for correct relative path resolution
    - systemd install before useradd to ensure systemd-journal group exists for usermod

key-files:
  created:
    - Containerfile
  modified: []

key-decisions:
  - "WORKDIR /build + COPY . . puts repo root at /build so path: ./receiver/tetragonreceiver in builder-config.yaml resolves to /build/receiver/tetragonreceiver (Pitfall 2 avoidance)"
  - "--output-path /tmp/dist keeps OCB-generated files outside source tree (Pitfall 7 avoidance)"
  - "--skip-strict-versioning required because tetragonreceiver is at v0.1.0 vs OCB v0.148.0"
  - "apt-get install systemd before groupadd/useradd so systemd-journal group exists for usermod -aG (Pitfall 4 avoidance)"
  - "debian:bookworm-slim not scratch/distroless because journaldreceiver needs journalctl binary at runtime"
  - "Config NOT baked into image — consumers mount their own at /etc/otelcol/config.yaml"

patterns-established:
  - "Pattern: Run OCB builder from WORKDIR /build (repo root copy) so relative path: fields resolve correctly"
  - "Pattern: systemd package installed before user creation in Containerfile for systemd-journal group availability"

requirements-completed: [DIST-03, DIST-04, DIST-05]

# Metrics
duration: 1min
completed: 2026-03-18
---

# Phase 2 Plan 02: Distribution Container Summary

**Multi-stage Containerfile using golang:1.25-bookworm + OCB v0.148.0 to build otelcol-tetragon binary, packaged into debian:bookworm-slim with systemd (journalctl), otel user UID 10001, and /etc/otelcol/config.yaml entrypoint contract**

## Performance

- **Duration:** 1 min
- **Started:** 2026-03-18T13:47:02Z
- **Completed:** 2026-03-18T13:48:16Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments
- Created multi-stage Containerfile: Go+OCB builder stage produces otelcol-tetragon, Debian-slim runtime packages it with journalctl support
- Non-root otel user at UID/GID 10001 with systemd-journal group membership (correct ordering: systemd install then useradd)
- Drop-in replacement container contract: entrypoint /usr/local/bin/otelcol-tetragon, default config /etc/otelcol/config.yaml, EXPOSE 13133

## Task Commits

Each task was committed atomically:

1. **Task 1: Create multi-stage Containerfile** - `d1c4a09` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified
- `Containerfile` - Multi-stage container build: Go+OCB builder + Debian-slim runtime with systemd, otel user, and collector entrypoint

## Decisions Made
- Used WORKDIR /build in build stage so path: ./receiver/tetragonreceiver in builder-config.yaml resolves correctly relative to repo root copy inside container
- Used --output-path /tmp/dist (absolute) to keep OCB-generated files outside the source tree, preventing git contamination
- Added --skip-strict-versioning because the local tetragonreceiver is at v0.1.0 which does not match OCB v0.148.0's strict version train check
- Installed systemd apt package BEFORE creating the otel user/group, ensuring the systemd-journal group created by apt exists when usermod -aG runs
- Chose debian:bookworm-slim over scratch/distroless because journaldreceiver invokes journalctl at runtime — binary must be present in the image

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - all acceptance criteria passed on first attempt.

## User Setup Required

None - no external service configuration required.

## Self-Check: PASSED

- Containerfile: FOUND at /workspaces/otel-collector-tetragon/Containerfile
- 02-02-SUMMARY.md: FOUND at .planning/phases/02-distribution/02-02-SUMMARY.md
- Task commit d1c4a09: FOUND in git log

## Next Phase Readiness
- Containerfile ready to build with `docker build -t otelcol-tetragon:local -f Containerfile .` from repo root
- Phase 2 distribution artifacts complete: builder-config.yaml (plan 01) + Containerfile (plan 02)
- Container image can be smoke-tested: `docker run --rm otelcol-tetragon:local id` to verify otel user, `docker run -d ... curl localhost:13133` for health_check
- Phase 3 (integration/deployment) can reference this image as the collector component

---
*Phase: 02-distribution*
*Completed: 2026-03-18*
