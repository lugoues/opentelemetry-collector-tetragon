# Phase 3: CI/CD and Release - Research

**Researched:** 2026-03-18
**Domain:** GitHub Actions, Docker Buildx, GHCR, multi-arch container images, Go CI, project README
**Confidence:** HIGH

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| CICD-01 | GitHub Actions workflow: test → build → push to GHCR | Full workflow pattern documented in Standard Stack and Architecture Patterns |
| CICD-02 | PR builds run tests and build image without pushing | `push: ${{ github.event_name != 'pull_request' }}` pattern verified against official Docker docs |
| CICD-03 | Main branch pushes tag with `latest` + `sha-<commit>` | `type=sha` default prefix is `sha-`, `type=ref,event=branch` produces branch name; `flavor: latest=auto` auto-tags latest on non-PR push |
| CICD-04 | Semver tags produce versioned image tags (v1.2.3 → 1.2.3, 1.2, latest) | `type=semver,pattern={{version}}`, `type=semver,pattern={{major}}.{{minor}}`, `type=semver,pattern={{major}}` with `flavor: latest=auto` |
| CICD-05 | Multi-arch build: linux/amd64 and linux/arm64 | `platforms: linux/amd64,linux/arm64` with QEMU emulation via `docker/setup-qemu-action` |
| PROJ-04 | README with usage, configuration reference, and build instructions | Content requirements identified: image pull, config YAML reference, build from source, CI/GHCR badge |
</phase_requirements>

---

## Summary

Phase 3 delivers two artifacts: a GitHub Actions CI/CD workflow file and a README. The CI workflow must gate on PR vs. push-to-main vs. semver-tag events and produce different image tag sets for each case. The standard Docker-maintained action set (`docker/setup-qemu-action`, `docker/setup-buildx-action`, `docker/login-action`, `docker/metadata-action`, `docker/build-push-action`) covers all requirements without any custom scripting. Multi-arch QEMU-based builds on a single `ubuntu-latest` runner are the simplest path for linux/amd64 + linux/arm64 given the project's scale.

The Go test step must target `./receiver/tetragonreceiver/` since that is the only module with test files. The top-level `go.mod` is a wrapper with no direct Go source. A `defaults: run: working-directory` scoped to the test job cleanly handles the subdirectory.

GHCR authentication requires `permissions: packages: write` plus `docker/login-action` with `registry: ghcr.io` and `password: ${{ secrets.GITHUB_TOKEN }}`. Image names must be lowercase — `github.repository` is lowercase by convention for `cilium/otelcol-tetragon` but the safe pattern is `${{ env.REGISTRY }}/${{ github.repository_owner }}/otelcol-tetragon` with an explicit lowercase step if org names ever contain uppercase characters.

**Primary recommendation:** One workflow file (`.github/workflows/ci.yml`) with a `test` job and a `build-push` job that depends on it. The `docker/metadata-action` handles all tag generation. No custom scripting is needed for tag logic.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `docker/setup-qemu-action` | v3 | Registers QEMU binfmt handlers for cross-arch builds | Required for arm64 emulation on amd64 runner |
| `docker/setup-buildx-action` | v3 | Creates a BuildKit builder instance | Required for `--platform` multi-arch support |
| `docker/login-action` | v4 | Authenticates to GHCR | Official Docker action, handles token refresh |
| `docker/metadata-action` | v5 | Generates OCI-compliant image tags and labels from Git ref | Handles all semver/sha/branch/latest logic |
| `docker/build-push-action` | v6 | Builds and optionally pushes the image | Wraps Buildx; supports `push: false` for PRs |
| `actions/checkout` | v4 | Checks out the repository | Standard; v4 is Node 20, broadly compatible |
| `actions/setup-go` | v5 | Installs Go toolchain | Caches modules by default via `go.sum` |

Note on versions: v7 of `docker/build-push-action` and v6 of `actions/checkout` were released in late 2025/early 2026 with Node 24 runtime requiring Actions Runner v2.327.1+. Use v6/v4 respectively for broader runner compatibility unless the repo explicitly targets the latest runner version.

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `actions/cache` | v4 | Caches Go build artifacts separately from Docker layer cache | Use if build times become slow; Docker GHA cache (`type=gha`) covers layer cache already |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| QEMU single-runner multi-arch | Native arm64 runners (separate jobs + manifest merge) | Native is faster for large images; QEMU simpler for small images like this one |
| `docker/build-push-action` | `redhat-actions/buildah-build` | Buildah supports Containerfile natively but less mature multi-arch support; stick with Docker action since `file:` input accepts Containerfile path |

**Installation:** No `npm install` needed — GitHub Actions actions are referenced by tag in workflow YAML.

---

## Architecture Patterns

### Recommended Project Structure
```
.github/
└── workflows/
    └── ci.yml          # Single workflow covering test, build, push
README.md               # Project root (PROJ-04)
```

### Pattern 1: Job-Split Workflow (test then build-push)

**What:** Two jobs — `test` runs Go tests in the receiver module subdirectory, `build-push` depends on `test` and handles Docker build/push.

**When to use:** Always — isolates test failures from expensive multi-arch builds, and allows the test job to run cheaply on PRs.

**Example:**
```yaml
# Source: https://docs.github.com/en/actions/tutorials/build-and-test-code/go
# Source: https://docs.docker.com/build/ci/github-actions/manage-tags-labels/
name: CI

on:
  push:
    branches: [main]
    tags: ['v*.*.*']
  pull_request:
    branches: [main]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ startsWith(github.ref, 'refs/pull/') }}

jobs:
  test:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: receiver/tetragonreceiver
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: receiver/tetragonreceiver/go.mod
          cache-dependency-path: receiver/tetragonreceiver/go.sum
      - run: go test ./...

  build-push:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        if: github.event_name != 'pull_request'
        uses: docker/login-action@v4
        with:
          registry: ghcr.io
          username: ${{ github.repository_owner }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/cilium/otelcol-tetragon
          tags: |
            type=ref,event=branch
            type=ref,event=pr
            type=sha
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}
          flavor: |
            latest=auto

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          file: ./Containerfile
          platforms: linux/amd64,linux/arm64
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

### Pattern 2: metadata-action Tag Behavior per Event

**What:** The `docker/metadata-action` produces different tag sets depending on the Git event:

| Event | Tags Produced |
|-------|---------------|
| `pull_request` | `pr-<number>` only (push is disabled) |
| `push` to `main` | `main`, `sha-<7-char-hash>`, `latest` |
| `push` tag `v1.2.3` | `1.2.3`, `1.2`, `1`, `latest` |

Note: `flavor: latest=auto` adds `latest` tag automatically when the event is a push to the default branch or a semver tag push (not a pre-release). For pre-release semver tags (e.g., `v1.0.0-rc.1`), `latest` is NOT added automatically.

**When to use:** Always — this matches CICD-03 and CICD-04 requirements exactly.

### Pattern 3: `go-version-file` for Reproducible Go Version

**What:** Pass `go-version-file: receiver/tetragonreceiver/go.mod` to `actions/setup-go` instead of a hardcoded version string. The action reads the `go` directive from `go.mod` and installs the matching toolchain.

**When to use:** Always — the receiver module declares `go 1.25.0`, so this ensures CI matches the module's minimum version without manual maintenance.

### Anti-Patterns to Avoid

- **Hardcoding `go-version: '1.25'`:** Requires manual update when go.mod changes. Use `go-version-file` instead.
- **Single job for test + build:** Makes debugging harder and runs the expensive multi-arch build even when tests fail.
- **`push: true` unconditionally:** Pushes on every PR triggering run, exposing credentials to fork PRs.
- **Using `github.repository` directly for image name without lowercase check:** Image names must be all-lowercase; for `cilium/otelcol-tetragon` this is safe, but the explicit GHCR path `ghcr.io/cilium/otelcol-tetragon` is safer than dynamically constructing from `${{ github.repository }}`.
- **No `concurrency` group:** Without it, multiple rapid pushes queue redundant workflows. The `cancel-in-progress` pattern for PRs prevents wasted runner minutes.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Semver tag parsing (1.2.3 → 1.2, 1) | Custom bash/sed to split version components | `docker/metadata-action` with `type=semver` patterns | Edge cases: pre-release labels, `v` prefix stripping, major=0 behavior |
| `latest` tag logic | Conditional steps checking `github.ref` | `docker/metadata-action` with `flavor: latest=auto` | Handles pre-release exclusion, branch detection, tag detection correctly |
| Multi-arch manifest creation | Manual `docker buildx imagetools create` | `docker/build-push-action` with `platforms:` | BuildKit handles platform-specific layer packing and manifest list creation |
| GHCR login token handling | Custom curl to GitHub API | `docker/login-action` with `secrets.GITHUB_TOKEN` | Handles token refresh, credential scope, proper logout |
| SHA tag formatting | `git rev-parse --short HEAD` in bash | `type=sha` in `docker/metadata-action` | Default `prefix=sha-,format=short` matches CICD-03 requirement exactly |

**Key insight:** The Docker GitHub Actions ecosystem covers all tag logic completely. The workflow YAML should contain zero custom bash logic for image naming.

---

## Common Pitfalls

### Pitfall 1: Missing `permissions: packages: write`
**What goes wrong:** Workflow fails with `denied: permission_unknown` or `403 Forbidden` when pushing to GHCR.
**Why it happens:** `GITHUB_TOKEN` defaults to read-only for packages. Without explicit `packages: write`, the login succeeds but push fails.
**How to avoid:** Add `permissions: contents: read\n packages: write` to the `build-push` job (or workflow-level, scoped per job is better practice).
**Warning signs:** Build succeeds, push step fails with HTTP 403 or "denied" error.

### Pitfall 2: QEMU not set up before Buildx
**What goes wrong:** The arm64 build layer fails with `exec format error` or the platform is silently skipped.
**Why it happens:** `docker/setup-buildx-action` creates the builder, but without QEMU binfmt handlers registered first, the builder cannot run arm64 binaries on an amd64 host.
**How to avoid:** Always run `docker/setup-qemu-action` BEFORE `docker/setup-buildx-action` in the step order.
**Warning signs:** Build completes quickly (too quickly for a multi-arch Go build), manifest has only one platform entry.

### Pitfall 3: `context: .` required for Containerfile at repo root
**What goes wrong:** Build fails with `COPY` errors — files not found because build context defaults to the Dockerfile's directory.
**Why it happens:** The Containerfile is at repo root and copies the entire source tree. Without `context: .`, BuildKit may use a default context that doesn't include the receiver module.
**How to avoid:** Explicitly set `context: .` in `docker/build-push-action` inputs even when Containerfile is at root.
**Warning signs:** `COPY . .` step fails with "file not found" for `receiver/` or `distribution/`.

### Pitfall 4: Semver `v` prefix in Docker tags
**What goes wrong:** Image is tagged `v1.2.3` instead of `1.2.3`, breaking consumers who expect unversioned tags.
**Why it happens:** `type=semver,pattern={{version}}` strips the `v` prefix by default. This is the CORRECT behavior. Using `pattern=v{{version}}` would add it back.
**How to avoid:** Use `type=semver,pattern={{version}}` (no `v` prefix in pattern) — this matches CICD-04's requirement of `1.0.0` (not `v1.0.0`).
**Warning signs:** Tag in GHCR starts with `v`.

### Pitfall 5: Pre-release tags get `latest`
**What goes wrong:** Pushing `v1.0.0-rc.1` stamps `latest` on a release candidate.
**Why it happens:** Without `flavor: latest=auto`, a naive `type=raw,value=latest` always adds the tag.
**How to avoid:** Use `flavor: latest=auto`. With `auto`, metadata-action only adds `latest` for non-pre-release semver tags and default-branch pushes.
**Warning signs:** `ghcr.io/cilium/otelcol-tetragon:latest` points to an RC image.

### Pitfall 6: OCB build inside Docker takes longer without layer cache
**What goes wrong:** Every CI run does a full `go install go.opentelemetry.io/collector/cmd/builder@v0.148.0` and downloads all Go modules from scratch.
**Why it happens:** The Containerfile has a `RUN go install` step that pulls the internet without a layer cache.
**How to avoid:** Use `cache-from: type=gha` and `cache-to: type=gha,mode=max` in `docker/build-push-action`. The GHA cache backend stores BuildKit layers between runs, so the `go install` and `go mod download` layers are cached after the first run.
**Warning signs:** Every build takes 10+ minutes; no "CACHED" lines in Buildx output.

### Pitfall 7: `go test ./...` at repo root finds no test files
**What goes wrong:** `go test ./...` passes trivially because the root `go.mod` has no Go source files — it's a wrapper with only a `replace` directive.
**Why it happens:** The test files live in `receiver/tetragonreceiver/` which is a separate Go module. Running `./...` from the root module doesn't cross module boundaries.
**How to avoid:** Set `working-directory: receiver/tetragonreceiver` for the test job, or use `go test github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver/...` from the root (but this requires module download). Simpler: use `defaults: run: working-directory`.
**Warning signs:** Test job completes in under 1 second with "no test files" or 0 tests run.

---

## Code Examples

### Complete CI Workflow (verified patterns from official sources)
```yaml
# Source: https://docs.docker.com/build/ci/github-actions/manage-tags-labels/
# Source: https://docs.docker.com/build/ci/github-actions/multi-platform/
# Source: https://docs.github.com/en/actions/tutorials/build-and-test-code/go
name: CI

on:
  push:
    branches: [main]
    tags: ['v*.*.*']
  pull_request:
    branches: [main]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ startsWith(github.ref, 'refs/pull/') }}

jobs:
  test:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: receiver/tetragonreceiver
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: receiver/tetragonreceiver/go.mod
          cache-dependency-path: receiver/tetragonreceiver/go.sum
      - run: go test ./...

  build-push:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        if: github.event_name != 'pull_request'
        uses: docker/login-action@v4
        with:
          registry: ghcr.io
          username: ${{ github.repository_owner }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/cilium/otelcol-tetragon
          tags: |
            type=ref,event=branch
            type=ref,event=pr
            type=sha
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}
          flavor: |
            latest=auto

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          file: ./Containerfile
          platforms: linux/amd64,linux/arm64
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

### Verifying Multi-Arch Manifest After Push
```bash
# Source: https://docs.docker.com/reference/cli/docker/manifest/
docker manifest inspect ghcr.io/cilium/otelcol-tetragon:latest

# Or with docker buildx imagetools (newer, preferred):
docker buildx imagetools inspect ghcr.io/cilium/otelcol-tetragon:latest
```

Expected output contains two platform entries:
```json
{
  "manifests": [
    { "platform": { "architecture": "amd64", "os": "linux" } },
    { "platform": { "architecture": "arm64", "os": "linux" } }
  ]
}
```

### PROJ-04 README Sections Required
```markdown
# otelcol-tetragon

A custom OpenTelemetry Collector distribution with a Tetragon gRPC receiver.

## Usage
docker pull ghcr.io/cilium/otelcol-tetragon:latest
docker run --rm -v ./config.yaml:/etc/otelcol/config.yaml ghcr.io/cilium/otelcol-tetragon:latest

## Configuration Reference
(table of receiver config fields: endpoint, insecure, retry settings)

## Build from Source
Requirements: Go 1.25+, Docker with Buildx
docker buildx build --platform linux/amd64,linux/arm64 -t otelcol-tetragon:dev .

## Development
go test ./... (in receiver/tetragonreceiver/)
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `docker build` + `docker manifest create` manual flow | `docker/build-push-action` with `platforms:` | ~2020 (Buildx GA) | Single step handles manifest list creation |
| `GITHUB_SHA` substring in bash for SHA tag | `type=sha` in `docker/metadata-action` | v3+ of metadata-action | Zero bash required, consistent 7-char format |
| Separate workflows for PR vs. push | Single workflow with `push: ${{ github.event_name != 'pull_request' }}` | ~2021 | Simpler, single source of truth |
| Hardcoded Go version in workflow | `go-version-file: go.mod` | `actions/setup-go` v4+ | Auto-tracks go.mod minimum version |

**Deprecated/outdated:**
- `docker/build-push-action@v5` and earlier: Use v6 (Node 20 runtime, stable API).
- `DOCKER_BUILD_NO_SUMMARY` env var: Removed in v7. Not needed in v6.
- Manual `docker login` bash step: Replaced by `docker/login-action`.

---

## Open Questions

1. **Repository visibility and package linking**
   - What we know: GHCR packages are private by default when first pushed; they must be linked to the repository and made public (or left private for internal use).
   - What's unclear: Whether the `cilium/otelcol-tetragon` repository exists on GitHub or will be created; if it's a fork vs. new repo, package namespace matters.
   - Recommendation: Document in README that after first push, the package must be linked to the repo via GHCR settings. The workflow itself works regardless.

2. **OCB `--skip-strict-versioning` in CI build time**
   - What we know: The Containerfile uses `--skip-strict-versioning`; this is required because tetragonreceiver is v0.1.0 while OCB is v0.148.0.
   - What's unclear: Whether OCB v0.148.0 multi-module download significantly increases CI build time without module cache.
   - Recommendation: The GHA layer cache (`type=gha,mode=max`) will cache the `go install` and `go mod download` layers after the first successful build. First cold build may take 10-15 min; subsequent builds should hit cache.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package (no external test framework) |
| Config file | None — `go test ./...` in `receiver/tetragonreceiver/` |
| Quick run command | `cd receiver/tetragonreceiver && go test ./...` |
| Full suite command | `cd receiver/tetragonreceiver && go test -race ./...` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CICD-01 | Workflow file exists and is valid YAML | smoke | `yamllint .github/workflows/ci.yml` or `actionlint` | ❌ Wave 0 |
| CICD-02 | PR event does not push image | manual | Trigger PR, inspect workflow run logs | manual-only |
| CICD-03 | Main push produces `latest` + `sha-*` tags | manual | Push to main, run `docker buildx imagetools inspect ghcr.io/cilium/otelcol-tetragon:latest` | manual-only |
| CICD-04 | Semver tag push produces versioned tags | manual | Push `v0.1.0` tag, verify GHCR package tags | manual-only |
| CICD-05 | Published manifest has amd64 + arm64 | manual | `docker manifest inspect ghcr.io/cilium/otelcol-tetragon:latest` | manual-only |
| PROJ-04 | README exists with required sections | smoke | `test -f README.md` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `cd /workspaces/otel-collector-tetragon/receiver/tetragonreceiver && go test ./...`
- **Per wave merge:** Same — no additional automated suite for CI/CD workflow correctness
- **Phase gate:** README exists, workflow file is valid YAML, Go tests still pass

### Wave 0 Gaps
- [ ] `.github/workflows/ci.yml` — the CI workflow (main deliverable of CICD-01 through CICD-05)
- [ ] `README.md` — covers PROJ-04
- [ ] `.github/` directory itself (does not exist yet)

*(No new Go test files are needed — existing test infrastructure in `receiver/tetragonreceiver/` covers all receiver behavior. The CI/CD requirements are verified by running the workflow, not by unit tests.)*

---

## Sources

### Primary (HIGH confidence)
- [Docker multi-platform CI docs](https://docs.docker.com/build/ci/github-actions/multi-platform/) — QEMU + Buildx + build-push-action workflow pattern
- [Docker manage-tags-labels docs](https://docs.docker.com/build/ci/github-actions/manage-tags-labels/) — Complete workflow YAML with PR/branch/semver/sha tag patterns, verified action versions
- [GitHub Actions Go docs](https://docs.github.com/en/actions/tutorials/build-and-test-code/go) — `actions/setup-go@v5`, `go-version-file`, working-directory patterns
- [docker/metadata-action README](https://github.com/docker/metadata-action) — `type=sha` default prefix `sha-`, `flavor: latest=auto` behavior, semver pattern syntax
- [docker/build-push-action README](https://github.com/docker/build-push-action) — `file:` input for Containerfile, `platforms:`, `push:`, `cache-from/cache-to` inputs; v7 is latest
- [GitHub Container Registry docs](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry) — `packages: write` permission requirement

### Secondary (MEDIUM confidence)
- [Docker GHA cache docs](https://docs.docker.com/build/ci/github-actions/cache/) — `type=gha,mode=max` cache backend for BuildKit layer caching
- [GitHub Actions concurrency docs](https://docs.github.com/actions/writing-workflows/choosing-what-your-workflow-does/control-the-concurrency-of-workflows-and-jobs) — `cancel-in-progress` pattern for PR workflows
- [docker manifest CLI reference](https://docs.docker.com/reference/cli/docker/manifest/) — `docker manifest inspect` and `docker buildx imagetools inspect` for verification

### Tertiary (LOW confidence)
- Community discussion on GHCR lowercase image name requirement — verified by multiple sources including build-push-action issues; `cilium/otelcol-tetragon` is already lowercase so this is not a blocking issue

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all action versions confirmed from official Docker and GitHub docs
- Architecture: HIGH — workflow pattern verified against official Docker CI/CD documentation
- Pitfalls: HIGH — most pitfalls verified against official docs or action source; GHA layer cache behavior confirmed against Docker docs
- README requirements: MEDIUM — content requirements derived from PROJ-04 description and project context, no external source to verify exact content

**Research date:** 2026-03-18
**Valid until:** 2026-09-18 (Docker actions update frequently; re-verify action major versions before planning if >30 days elapsed)
