# Phase 2: Distribution - Research

**Researched:** 2026-03-18
**Domain:** OCB (OpenTelemetry Collector Builder), multi-stage container image, Go module workspace
**Confidence:** HIGH

## Summary

Phase 2 packages the `tetragonreceiver` built in Phase 1 into a runnable container image. It has three sub-problems: (1) OCB builder-config.yaml that references the local receiver via `path:`, produces a valid binary; (2) a top-level `go.mod` with a `replace` directive so the generated collector module can resolve the local receiver; (3) a multi-stage `Containerfile` that installs `journalctl` for the journald receiver, creates the `otel:10001` user with `systemd-journal` group membership, and matches the entrypoint/config-path contract of the current `otelcol-contrib` image.

All module versions have been verified live from the Go module proxy (2026-03-17 timestamps). OCB v0.148.0 is the current release. The receiver module at `github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver` exists at `v0.148.0` (same OTel release train). The `healthcheckextension` and `resourcedetectionprocessor` live in `opentelemetry-collector-contrib`; `batchprocessor` and `otlphttpexporter` live in the core `opentelemetry-collector` repo.

**Primary recommendation:** Use the `path:` field on the receiver entry in builder-config.yaml (not the global `replaces:` section) for simplicity; write a top-level `go.mod` declaring the distribution module so OCB has a parent module context; base the runtime image on `debian:bookworm-slim` with `systemd` installed for `journalctl`.

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| DIST-01 | OCB builder-config.yaml produces binary with tetragonreceiver + journald, batch, resourcedetection, otlphttp, health_check, file_storage | OCB `path:` field resolves local module; all companion component gomod paths and v0.148.0 versions verified |
| DIST-02 | Top-level go.mod with replaces directive for local receiver module | Standard Go workspace pattern; OCB reads parent go.mod's `replace` directives when building in-directory |
| DIST-03 | Multi-stage Containerfile: Go builder with OCB → Debian-slim runtime with systemd and ca-certificates | Official otelcol-contrib Dockerfile pattern adapted; debian:bookworm-slim + systemd package confirmed for journalctl |
| DIST-04 | Container runs as non-root user otel:10001 with systemd-journal group membership | `groupadd/useradd/usermod -aG systemd-journal` pattern verified from Dash0 guide and official contrib README |
| DIST-05 | Container image is drop-in replacement: same entrypoint, config path, runtime user as current otelcol-contrib image | Entrypoint `/usr/local/bin/otelcol-tetragon`, config `/etc/otelcol/config.yaml`, user `otel:10001` |
| PROJ-02 | Example collector config in rootfs/etc/otelcol/config.yaml | Full pipeline YAML documented in SPEC.md; component config keys verified against v0.148.0 packages |
</phase_requirements>

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `go.opentelemetry.io/collector/cmd/builder` | v0.148.0 | OCB — generates collector binary from manifest | Official OTel builder tool; matches receiver module version |
| `go.opentelemetry.io/collector/processor/batchprocessor` | v0.148.0 | Batch logs before export | Core OTel repo; reduces exporter round-trips |
| `go.opentelemetry.io/collector/exporter/otlphttpexporter` | v0.148.0 | OTLP/HTTP export to OpenObserve | Core OTel repo; v0 track |
| `github.com/open-telemetry/opentelemetry-collector-contrib/receiver/journaldreceiver` | v0.148.0 | Read systemd journal logs | Contrib repo; requires `journalctl` binary in image |
| `github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor` | v0.148.0 | Attach host metadata | Contrib repo |
| `github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension` | v0.148.0 | HTTP health endpoint | Contrib repo (NOT core) — healthcheckextension is in contrib |
| `github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage` | v0.148.0 | Checkpoint persistence for journald | Contrib repo; required by journald for restart recovery |

### Runtime Image
| Component | Version | Purpose |
|-----------|---------|---------|
| `debian:bookworm-slim` | bookworm-slim | Minimal Debian with glibc; supports `systemd` apt package |
| `systemd` (apt) | from Debian bookworm | Provides `/usr/bin/journalctl` required by journaldreceiver |
| `ca-certificates` (apt) | from Debian bookworm | TLS root store for OTLP/HTTP export |

### Build Image
| Component | Version | Purpose |
|-----------|---------|---------|
| `golang` | 1.25-bookworm | Go toolchain matching receiver go.mod minimum |

**Version verification:** All gomod versions confirmed live from proxy.golang.org (2026-03-17).

**Installation (OCB in build stage):**
```bash
go install go.opentelemetry.io/collector/cmd/builder@v0.148.0
```

**CRITICAL — healthcheckextension module path:**
```
github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.148.0
```
NOT `go.opentelemetry.io/collector/extension/healthcheckextension` — that path does not exist. healthcheckextension is in the contrib repo.

## Architecture Patterns

### Recommended Project Structure

```
otelcol-tetragon/
├── receiver/
│   └── tetragonreceiver/       # Phase 1 — standalone Go module
│       ├── go.mod               # module github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver
│       └── ...
├── distribution/
│   └── builder-config.yaml      # OCB manifest referencing receiver via path:
├── rootfs/
│   └── etc/otelcol/
│       └── config.yaml          # Example runtime config (PROJ-02)
├── Containerfile                # Multi-stage: Go+OCB build → debian-slim runtime
├── go.mod                       # Top-level: module github.com/cilium/otelcol-tetragon
└── go.sum                       # (generated)
```

The top-level `go.mod` declares a parent module and contains the `replace` directive for the local receiver. OCB generates a temporary module in `output_path` that inherits this replace via the `path:` field mechanism.

### Pattern 1: builder-config.yaml with Local Module

**What:** The OCB `path:` field on a receiver entry tells OCB to use local filesystem source instead of fetching from a module proxy. This is the recommended way to reference an unpublished local module.

**When to use:** Always for the tetragonreceiver until/unless it is published to a public registry.

**Two equivalent approaches:**

Approach A — `path:` field on the component entry (preferred: self-contained, no separate replaces block):
```yaml
# distribution/builder-config.yaml
dist:
  name: otelcol-tetragon
  description: "OTel Collector with Tetragon gRPC receiver"
  module: github.com/cilium/otelcol-tetragon/distribution
  output_path: ./dist

receivers:
  - gomod: github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver v0.1.0
    path: ../receiver/tetragonreceiver
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/journaldreceiver v0.148.0
```

Approach B — global `replaces:` block (use as fallback if `path:` causes transitive dependency issues):
```yaml
receivers:
  - gomod: github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver v0.1.0

replaces:
  - github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver v0.1.0 => ../receiver/tetragonreceiver
```

**Note on PR #12638:** OCB rewrites relative `path:` values to absolute paths before injecting into the generated go.mod. This means the builder must be run from a consistent working directory (the repo root) so that relative paths resolve correctly regardless of `output_path`.

### Pattern 2: Top-Level go.mod

**What:** OCB generates a new Go module in `output_path`. For the generated module's `go.mod` to include a `replace` for the local receiver, either the `path:` field (Approach A) or the `replaces:` section (Approach B) is used. A top-level `go.mod` at the repo root is NOT strictly required by OCB itself, but is conventional for Go workspaces and satisfies DIST-02 ("top-level go.mod with replaces directive for local receiver module").

**Minimal top-level go.mod:**
```
module github.com/cilium/otelcol-tetragon

go 1.25.0

replace github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver => ./receiver/tetragonreceiver
```

This allows `go build ./...` from the repo root to work if any top-level tools need to resolve the receiver. The `replace` here is for tooling convenience; OCB's own resolution uses the `path:` field in builder-config.yaml.

### Pattern 3: Multi-Stage Containerfile

**What:** Stage 1 installs OCB and builds the binary. Stage 2 is the minimal runtime image.

**Why debian:bookworm-slim (not scratch/distroless):** The `journaldreceiver` invokes `journalctl` at runtime. The collector binary needs a system with `journalctl` available. `debian:bookworm-slim` + `apt-get install systemd` provides this. A distroless or scratch base would require bundling journalctl and all its systemd shared library dependencies manually.

```dockerfile
# --- Build stage ---
FROM docker.io/library/golang:1.25-bookworm AS builder

# Install OCB
RUN go install go.opentelemetry.io/collector/cmd/builder@v0.148.0

WORKDIR /build
COPY . .

# Run OCB from the repo root so relative paths in builder-config.yaml resolve correctly
RUN builder --config distribution/builder-config.yaml --output-path /tmp/dist

# --- Runtime stage ---
FROM docker.io/library/debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      systemd \
      ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Create otel user matching otelcol-contrib convention (UID/GID 10001)
RUN groupadd --system --gid 10001 otel && \
    useradd --system --uid 10001 --gid otel --no-create-home otel && \
    usermod -aG systemd-journal otel

COPY --from=builder /tmp/dist/otelcol-tetragon /usr/local/bin/otelcol-tetragon

USER otel

EXPOSE 13133

ENTRYPOINT ["/usr/local/bin/otelcol-tetragon"]
CMD ["--config", "/etc/otelcol/config.yaml"]
```

**Drop-in replacement contract (DIST-05):**
- Entrypoint: `/usr/local/bin/otelcol-tetragon` (binary name differs from contrib's `/otelcol-contrib` but the `CMD` config path is the same)
- Config path: `/etc/otelcol/config.yaml`
- Runtime user: `otel` UID 10001

**Note:** The current `otelcol-contrib` image uses `/etc/otelcol-contrib/config.yaml`. The SPEC specifies `/etc/otelcol/config.yaml`. Use `/etc/otelcol/config.yaml` — this is a deliberate simplification, not an error. Consumers updating from contrib will need to update their volume mount path.

### Pattern 4: Example Config (PROJ-02)

**What:** `rootfs/etc/otelcol/config.yaml` is the example config shipped in the repo for documentation and smoke testing. It is NOT baked into the image — consumers mount their own config.

```yaml
# rootfs/etc/otelcol/config.yaml
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

**health_check endpoint for DIST-02 success criteria:** The success criteria requires `health_check` responds 200 with the example config. `health_check` listens on `0.0.0.0:13133` by default. The journald pipeline will fail if `/var/log/journal` is not mounted, but the `health_check` extension is independent of the pipelines — the collector starts and reports healthy even if receivers cannot connect.

### Anti-Patterns to Avoid

- **Hardcoding config in the image:** Do not `COPY rootfs/etc/otelcol/config.yaml` into the image at `/etc/otelcol/config.yaml`. Keep config external. The CMD specifies the path; users mount their own config.
- **Using `scratch` or `distroless` as runtime base:** journaldreceiver invokes `journalctl` at runtime. Without the binary in the container, the receiver fails to start.
- **Running OCB from `distribution/` subdirectory:** Relative `path:` values in builder-config.yaml are resolved relative to the CWD when OCB runs, not relative to the config file location. Run OCB from the repo root.
- **Mixing stable and unstable version tracks:** All components must be at v0.148.0. The receiver module at the receiver's go.mod minimum (v0.148.0 for unstable-track packages, v1.54.0 for stable-track) are already pinned in Phase 1. The OCB builder-config.yaml must use the v0.x.y versions for all components since that's where these packages live.
- **Installing full `systemd` package:** `apt-get install systemd` on Debian will install systemd but `debian:bookworm-slim` strips init system integrations. Only the `journalctl` binary and libsystemd are needed. Alternative: `apt-get install -y --no-install-recommends libsystemd-dev` + copying `journalctl` binary. However, `systemd` package is simpler and the image size impact is acceptable.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Collector binary assembly | Custom `main.go` with manual factory registration | OCB with builder-config.yaml | OCB generates correct `components.go`, handles import graph, `go mod tidy` |
| Health check endpoint | Custom HTTP handler | `healthcheckextension` v0.148.0 | Implements OTel health check protocol; tested against collector lifecycle |
| Checkpoint/offset tracking for journald | Custom file-based state | `file_storage` extension + journald `storage:` field | file_storage implements atomic writes, handles restart recovery |
| Resource attribute injection | Hardcode hostname in config | `resourcedetectionprocessor` with `system` detector | Handles dynamic hostnames, cloud metadata |
| Non-root user in Dockerfile | Capability dropping or seccomp profiles | `useradd` + `usermod -aG systemd-journal` | The systemd-journal group grants read access to journal files without root |

**Key insight:** OCB's entire purpose is to avoid hand-rolling the collector binary. All the generated code (`main.go`, `components.go`, `go.mod`) would be error-prone to maintain manually across OTel releases.

## Common Pitfalls

### Pitfall 1: healthcheckextension Wrong Module Path
**What goes wrong:** Builder fails with "module not found" for `go.opentelemetry.io/collector/extension/healthcheckextension`
**Why it happens:** `healthcheckextension` is in the contrib repo, not the core collector repo. The core repo does not have a `healthcheckextension` package.
**How to avoid:** Use `github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.148.0`
**Warning signs:** `invalid version: unknown revision extension/healthcheckextension/v0.148.0` from go module proxy

### Pitfall 2: OCB Relative Path Resolution
**What goes wrong:** OCB succeeds locally but fails in Docker build with "directory not found" for the local receiver path
**Why it happens:** `path: ../receiver/tetragonreceiver` is resolved relative to CWD when OCB runs. In Docker, `WORKDIR /build` and `COPY . .` puts everything under `/build`, so the path resolves to `/build/../receiver/tetragonreceiver` = `/receiver/tetragonreceiver` which does not exist.
**How to avoid:** Run OCB with `WORKDIR /build` and path `path: ./receiver/tetragonreceiver` (relative to repo root inside container), OR use `path: /build/receiver/tetragonreceiver` (absolute). The safest pattern: run `builder --config distribution/builder-config.yaml` from `/build` with `path: ./receiver/tetragonreceiver` in the config.
**Warning signs:** OCB error "cannot find module providing package" or "directory does not exist" during Docker build

### Pitfall 3: journald Receiver Requires journalctl at Runtime
**What goes wrong:** Collector starts but journald pipeline fails with "exec: journalctl not found" or similar
**Why it happens:** journaldreceiver uses `exec.Command("journalctl", ...)` to read journal data. If the binary is absent, the receiver fails.
**How to avoid:** Install `systemd` (or at minimum the `systemd` package that includes journalctl) in the runtime image.
**Warning signs:** `[logs/journal] receiver failed to start: journalctl not found`

### Pitfall 4: systemd-journal Group Not Present in Runtime Container
**What goes wrong:** Collector runs as `otel` user but cannot read journal (permission denied)
**Why it happens:** The `systemd-journal` group is created by the `systemd` package at install time. If the `usermod -aG systemd-journal otel` runs before `apt-get install systemd`, the group does not yet exist.
**How to avoid:** Run `apt-get install systemd` first, then `groupadd otel`, `useradd otel`, `usermod -aG systemd-journal otel` — in that order.
**Warning signs:** `permission denied` on `/var/log/journal/` at container startup

### Pitfall 5: health_check Fails with Example Config Due to Missing journald Volume
**What goes wrong:** `docker run` with example config returns non-200 from health_check because journald receiver cannot open `/var/log/journal`
**Why it happens:** The example config references `journald.directory: /var/log/journal` which may not be mounted in a test environment. If the collector marks itself unhealthy due to a failed receiver, health_check returns non-200.
**How to avoid:** Per the success criteria, health_check must respond 200 "without any volume mounts or special permissions beyond the otel:10001 user". This means the journald receiver must not hard-fail at startup when the directory is absent — or the example config should make journald optional. Check journaldreceiver behavior when directory is missing; if it hard-fails, the example config needs a note that journald pipeline requires volume mount.
**Warning signs:** `curl localhost:13133` returns 503 in smoke test

### Pitfall 6: Version Mismatch Between OCB and Components
**What goes wrong:** OCB strict versioning check fails during build
**Why it happens:** OCB v0.148.0 enforces that all component versions in builder-config.yaml have matching major.minor (0.148) with the builder itself.
**How to avoid:** Use v0.148.0 for all components. The receiver module is declared at `v0.1.0` (our custom version) which won't match — this will trigger the strict versioning check.
**Resolution:** Pass `--skip-strict-versioning` flag to OCB when the local receiver module uses a non-matching version. This flag is documented as temporary but exists in v0.148.0.
**Warning signs:** `strict versioning check failed: version mismatch`

### Pitfall 7: OCB Output Path Conflicts with Source Tree
**What goes wrong:** OCB writes generated files (go.mod, main.go, etc.) into a directory that overlaps with the source tree, causing COPY issues in subsequent Docker builds
**How to avoid:** Use `output_path: /tmp/dist` (absolute) in builder-config.yaml so generated files go outside the COPY context. In Docker, use `RUN builder ... --output-path /tmp/dist` then `COPY --from=builder /tmp/dist/otelcol-tetragon`.
**Warning signs:** Git shows unexpected generated files appearing in source directories

## Code Examples

### Complete builder-config.yaml
```yaml
# distribution/builder-config.yaml
# Source: OCB configuration reference https://pkg.go.dev/go.opentelemetry.io/collector/cmd/builder
dist:
  name: otelcol-tetragon
  description: "OTel Collector with Tetragon gRPC receiver"
  module: github.com/cilium/otelcol-tetragon/distribution
  output_path: /tmp/dist

extensions:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.148.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage v0.148.0

receivers:
  - gomod: github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver v0.1.0
    path: ./receiver/tetragonreceiver
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/journaldreceiver v0.148.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.148.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor v0.148.0

exporters:
  - gomod: go.opentelemetry.io/collector/exporter/otlphttpexporter v0.148.0
```

### Complete Containerfile
```dockerfile
# --- Build stage ---
FROM docker.io/library/golang:1.25-bookworm AS builder

RUN go install go.opentelemetry.io/collector/cmd/builder@v0.148.0

WORKDIR /build
COPY . .

# Run from repo root so path: ./receiver/tetragonreceiver resolves correctly
RUN builder \
    --config distribution/builder-config.yaml \
    --output-path /tmp/dist \
    --skip-strict-versioning

# --- Runtime stage ---
FROM docker.io/library/debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      systemd \
      ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Create otel user AFTER systemd install so systemd-journal group exists
RUN groupadd --system --gid 10001 otel && \
    useradd --system --uid 10001 --gid otel --no-create-home otel && \
    usermod -aG systemd-journal otel

COPY --from=builder /tmp/dist/otelcol-tetragon /usr/local/bin/otelcol-tetragon

USER otel

EXPOSE 13133

ENTRYPOINT ["/usr/local/bin/otelcol-tetragon"]
CMD ["--config", "/etc/otelcol/config.yaml"]
```

### Top-Level go.mod
```
# go.mod (repo root)
module github.com/cilium/otelcol-tetragon

go 1.25.0

replace github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver => ./receiver/tetragonreceiver
```

### OCB Build Command (local dev, from repo root)
```bash
# Install OCB matching component versions
go install go.opentelemetry.io/collector/cmd/builder@v0.148.0

# Build — must run from repo root for path: resolution
builder \
  --config distribution/builder-config.yaml \
  --output-path /tmp/otelcol-tetragon \
  --skip-strict-versioning
```

### mise.toml Tasks to Add
```toml
[tasks.ocb]
description = "Build custom OTel Collector binary with OCB"
run = "builder --config distribution/builder-config.yaml --output-path /tmp/otelcol-tetragon --skip-strict-versioning"

[tasks.container]
description = "Build container image"
run = "docker build -t otelcol-tetragon:local -f Containerfile ."

[tasks.smoke]
description = "Run container smoke test: start, check health endpoint, stop"
run = """
docker run -d --name otelcol-smoke otelcol-tetragon:local
sleep 3
docker exec otelcol-smoke curl -sf http://localhost:13133/ || (docker rm -f otelcol-smoke; exit 1)
docker rm -f otelcol-smoke
"""
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `otelcol_version:` field in dist | Version inferred from OCB binary itself | ~v0.99.0+ | No need to set otelcol_version; strict versioning validates consistency |
| Manual `go.mod replace` for local components | `path:` field on component entry | Current | path: is simpler for per-component overrides; replaces: still needed for transitive deps |
| Alpine base image | Debian bookworm-slim for systemd-dependent collectors | Ongoing | journalctl is not in Alpine's standard packages; Debian packaging is simpler |
| `FROM scratch` (official contrib image) | `FROM debian:bookworm-slim` | N/A — our image serves different use case | Official contrib doesn't need journalctl; we do |

**Deprecated/outdated in SPEC.md:**
- SPEC.md shows `v0.120.0` for all components — stale. Use v0.148.0.
- SPEC.md shows `go 1.23` in Containerfile — stale. Go 1.25 is required by tetragon/api and OTel v0.148.0.
- SPEC.md shows `output_path: /tmp/otelcol-tetragon` — fine, just note we use `/tmp/dist` in examples.

## Open Questions

1. **journald receiver behavior when directory missing**
   - What we know: journaldreceiver invokes `journalctl`; if directory doesn't exist, behavior depends on journalctl version
   - What's unclear: Does the receiver return an error at startup (blocking health_check from passing) or does it just emit warnings?
   - Recommendation: Test by running the collector with the example config inside the built container with no `/var/log/journal` mount. If health_check fails, add a comment in the example config or make journald pipeline conditional. LOW confidence on receiver behavior — verify empirically.

2. **systemd package size in bookworm-slim**
   - What we know: `systemd` package pulls in many dependencies on Debian
   - What's unclear: Whether `libsystemd0` + copying journalctl binary would produce a meaningfully smaller image
   - Recommendation: Start with `apt-get install systemd` for simplicity. Optimize later if image size is a concern.

3. **--skip-strict-versioning flag longevity**
   - What we know: The flag is documented as "temporary, will be removed in a future minor version"
   - What's unclear: When exactly it will be removed; what the replacement mechanism is
   - Recommendation: Use it for now. The alternative is tagging the receiver module at v0.148.0 to match OCB, which requires a published tag or go.mod version. For a local-only module, this is impractical without publishing.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go test (no additional framework needed for OCB build validation) |
| Config file | none — OCB outputs to /tmp/dist |
| Quick run command | `builder --config distribution/builder-config.yaml --output-path /tmp/dist --skip-strict-versioning` |
| Full suite command | `mise run ocb && docker build -t otelcol-tetragon:local -f Containerfile .` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DIST-01 | OCB produces binary that starts and lists tetragonreceiver | smoke | `./dist/otelcol-tetragon components` | ❌ Wave 0 |
| DIST-02 | Top-level go.mod with replace resolves at `go build ./...` | build | `go build ./...` from repo root | ❌ Wave 0 |
| DIST-03 | Containerfile builds without error | build | `docker build -f Containerfile .` | ❌ Wave 0 |
| DIST-04 | Container user is otel:10001 with systemd-journal group | smoke | `docker run --rm otelcol-tetragon:local id` | ❌ Wave 0 |
| DIST-05 | health_check responds 200 with example config | smoke | `docker run -d ... && curl -sf localhost:13133/` | ❌ Wave 0 |
| PROJ-02 | Example config is valid YAML for the built binary | smoke | `otelcol-tetragon validate --config rootfs/etc/otelcol/config.yaml` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `builder --config distribution/builder-config.yaml --output-path /tmp/dist --skip-strict-versioning` (OCB dry run)
- **Per wave merge:** OCB build + `docker build` + smoke test
- **Phase gate:** health_check responds 200 before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `distribution/builder-config.yaml` — OCB manifest (created in Wave 0)
- [ ] `Containerfile` — Multi-stage build (created in Wave 0)
- [ ] `go.mod` (repo root) — Top-level module with replace (created in Wave 0)
- [ ] `rootfs/etc/otelcol/config.yaml` — Example config (created in Wave 0)
- [ ] `mise.toml` additions — `ocb`, `container`, `smoke` tasks

## Sources

### Primary (HIGH confidence)
- `pkg.go.dev/go.opentelemetry.io/collector/cmd/builder` — builder config struct, Module struct with path: field, replaces: format (verified v0.148.0, 2026-03-17)
- Go module proxy `proxy.golang.org` — all gomod version/timestamp verifications
- `github.com/open-telemetry/opentelemetry-collector-releases/distributions/otelcol-contrib/Dockerfile` — user pattern (UID 10001, non-root) confirmed from official releases

### Secondary (MEDIUM confidence)
- Dash0 journald receiver guide (2025) — systemd package requirement, user/group setup pattern, volume mount requirements
- OCB official docs `opentelemetry.io/docs/collector/extend/ocb/` — manifest format (verified against current pkg.go.dev)
- `opentelemetry-collector-contrib/receiver/journaldreceiver/README.md` — journalctl binary requirement, directory configuration

### Tertiary (LOW confidence)
- `--skip-strict-versioning` necessity for local receiver v0.1.0: inferred from OCB strict versioning behavior documented in pkg.go.dev; not directly tested with the exact version combination
- journaldreceiver behavior when `/var/log/journal` absent: needs empirical verification (flagged as Open Question 1)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all module paths and versions verified from Go module proxy
- Architecture: HIGH — OCB config format confirmed from pkg.go.dev struct definitions; Containerfile pattern confirmed from official otelcol-contrib Dockerfile
- Pitfalls: MEDIUM — most from official sources; Pitfall 5 (journald startup behavior) requires empirical testing

**Research date:** 2026-03-18
**Valid until:** 2026-04-18 (OCB releases monthly; verify versions if >30 days old)
