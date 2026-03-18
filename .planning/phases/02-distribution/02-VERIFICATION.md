---
phase: 02-distribution
verified: 2026-03-18T14:00:00Z
status: passed
score: 6/6 must-haves verified
re_verification: false
---

# Phase 2: Distribution Verification Report

**Phase Goal:** Create a reproducible build pipeline: OCB builder-config.yaml, top-level go.mod, example collector config, Containerfile, and mise tasks
**Verified:** 2026-03-18T14:00:00Z
**Status:** PASSED
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | OCB builds a binary from builder-config.yaml that registers tetragonreceiver, journaldreceiver, batchprocessor, resourcedetectionprocessor, otlphttpexporter, healthcheckextension, and filestorage | VERIFIED | All 7 components present in `distribution/builder-config.yaml`; 6x `v0.148.0` entries plus local receiver at `v0.1.0` with `path: ./receiver/tetragonreceiver` |
| 2 | Top-level go.mod declares module `github.com/cilium/otelcol-tetragon` with replace directive pointing to `./receiver/tetragonreceiver` | VERIFIED | `go.mod` line 1: `module github.com/cilium/otelcol-tetragon`; line 5: `replace github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver => ./receiver/tetragonreceiver` |
| 3 | Example config at `rootfs/etc/otelcol/config.yaml` defines two log pipelines (tetragon + journald) with batch, resourcedetection, and otlphttp/openobserve | VERIFIED | Both `logs/tetragon:` and `logs/journal:` pipelines present; `batch`, `resourcedetection`, and `otlphttp/openobserve` exporter all defined |
| 4 | Containerfile builds a multi-stage image: Go builder with OCB produces binary, Debian-slim runtime has systemd and ca-certificates | VERIFIED | Two `FROM` stages: `golang:1.25-bookworm AS builder` and `debian:bookworm-slim`; `apt-get install systemd ca-certificates` in runtime stage |
| 5 | Container runs as non-root user otel with UID 10001 and systemd-journal group membership | VERIFIED | `groupadd --system --gid 10001 otel`, `useradd --system --uid 10001 --gid otel --no-create-home otel`, `usermod -aG systemd-journal otel`; `USER otel` before entrypoint; systemd install precedes user creation (correct ordering) |
| 6 | Container entrypoint is `/usr/local/bin/otelcol-tetragon` with default config path `/etc/otelcol/config.yaml` | VERIFIED | `ENTRYPOINT ["/usr/local/bin/otelcol-tetragon"]`, `CMD ["--config", "/etc/otelcol/config.yaml"]` |

**Score:** 6/6 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `distribution/builder-config.yaml` | OCB manifest with all 7 components | VERIFIED | 22 lines; `name: otelcol-tetragon`; `output_path: /tmp/dist`; all 7 components with correct module paths |
| `go.mod` | Top-level Go module with replace directive | VERIFIED | 5 lines; correct module name; `go 1.25.0`; replace directive present |
| `rootfs/etc/otelcol/config.yaml` | Two-pipeline example config | VERIFIED | 47 lines; both pipelines; health_check, file_storage/checkpoint, batch, resourcedetection, otlphttp/openobserve all present |
| `.mise/config.toml` | OCB and container build tasks | VERIFIED | 7 total tasks: 4 existing (test, lint, build, tidy) + 3 new (ocb, container, smoke); existing tasks preserved |
| `Containerfile` | Multi-stage container build | VERIFIED | 37 lines; 2 FROM stages; all acceptance criteria from plan 02-02 satisfied |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `distribution/builder-config.yaml` | `receiver/tetragonreceiver` | `path: ./receiver/tetragonreceiver` | WIRED | `path: ./receiver/tetragonreceiver` at line 13 of builder-config.yaml |
| `go.mod` | `receiver/tetragonreceiver` | replace directive | WIRED | `replace github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver => ./receiver/tetragonreceiver` at line 5 |
| `Containerfile` | `distribution/builder-config.yaml` | `builder --config distribution/builder-config.yaml` | WIRED | `--config distribution/builder-config.yaml` at line 11; WORKDIR /build + COPY . . ensures relative paths resolve |
| `Containerfile` | `/usr/local/bin/otelcol-tetragon` | `COPY --from=builder` | WIRED | `COPY --from=builder /tmp/dist/otelcol-tetragon /usr/local/bin/otelcol-tetragon` at line 29 |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| DIST-01 | 02-01 | OCB builder-config.yaml produces custom collector with tetragonreceiver + journald, batch, resourcedetection, otlphttp, health_check, file_storage | SATISFIED | All 7 components in `distribution/builder-config.yaml` |
| DIST-02 | 02-01 | Top-level go.mod with replaces directive for local receiver module | SATISFIED | Replace directive in `go.mod` line 5 |
| DIST-03 | 02-02 | Multi-stage Containerfile: Go builder with OCB to Debian-slim runtime with systemd and ca-certificates | SATISFIED | Two-stage Containerfile with golang:1.25-bookworm + debian:bookworm-slim |
| DIST-04 | 02-02 | Container runs as non-root user (otel:10001) with systemd-journal group membership | SATISFIED | groupadd/useradd at UID/GID 10001, usermod -aG systemd-journal otel, USER otel |
| DIST-05 | 02-02 | Container image is drop-in replacement: same entrypoint, config path, runtime user as otelcol-contrib | SATISFIED | ENTRYPOINT /usr/local/bin/otelcol-tetragon, CMD --config /etc/otelcol/config.yaml, EXPOSE 13133 |
| PROJ-02 | 02-01 | Example collector config in rootfs/etc/otelcol/config.yaml | SATISFIED | `rootfs/etc/otelcol/config.yaml` with two pipelines |

All 6 requirement IDs claimed by both plans (DIST-01, DIST-02, DIST-03, DIST-04, DIST-05, PROJ-02) are accounted for and satisfied. No orphaned requirements for Phase 2.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `.mise/config.toml` | 26 | `ocb` task uses `--output-path /tmp/otelcol-tetragon` while `builder-config.yaml` declares `output_path: /tmp/dist` and Containerfile uses `--output-path /tmp/dist` | INFO | The paths differ between local dev task and container build. The `--output-path` CLI flag overrides the YAML config. The Containerfile and builder-config.yaml are consistent (`/tmp/dist`). The mise `ocb` task was intentionally specified with `/tmp/otelcol-tetragon` in the plan — this is as designed, not a defect. No goal impact. |

No TODO/FIXME/placeholder patterns. No empty implementations. No stub handlers.

### Human Verification Required

#### 1. OCB Binary Build

**Test:** From repo root, run `mise run ocb` (requires OCB v0.148.0 and network access to fetch Go modules)
**Expected:** Binary produced at `/tmp/otelcol-tetragon/otelcol-tetragon`; no strict versioning errors
**Why human:** Requires actual OCB install, Go module downloads, and successful compilation — cannot verify programmatically without running the build

#### 2. Container Build and Smoke Test

**Test:** Run `mise run container` then `mise run smoke`
**Expected:** Container builds, health endpoint at `localhost:13133` returns 200, container cleans up
**Why human:** Requires Docker daemon, network access for apt/go module pulls, and runtime execution

#### 3. systemd-journal Group at Runtime

**Test:** Run `docker run --rm otelcol-tetragon:local id`
**Expected:** Output shows `uid=10001(otel) gid=10001(otel) groups=10001(otel),systemd-journal`
**Why human:** Group membership only verifiable by executing the built container

### Gaps Summary

No gaps. All must-haves verified. All 5 required files exist with substantive, non-stub content. All key links are wired. All 6 requirement IDs satisfied. The only notable item is an INFO-level path difference in the mise `ocb` task vs the builder-config.yaml/Containerfile, which is intentional per the plan specification and does not affect goal achievement.

---

_Verified: 2026-03-18T14:00:00Z_
_Verifier: Claude (gsd-verifier)_
