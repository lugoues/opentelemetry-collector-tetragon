---
phase: 03-cicd-and-release
plan: 01
subsystem: infra
tags: [github-actions, docker, ghcr, multi-arch, buildx, qemu, readme]

# Dependency graph
requires:
  - phase: 02-distribution
    provides: Containerfile for multi-arch Docker build and go.mod for Go version pinning
provides:
  - GitHub Actions CI/CD workflow with two-job pipeline (test + build-push)
  - Project README with usage, configuration reference, build instructions, and component listing
affects: [deployment, release process, project onboarding]

# Tech tracking
tech-stack:
  added:
    - docker/setup-qemu-action@v3 (QEMU for arm64 emulation)
    - docker/setup-buildx-action@v3 (Docker Buildx builder)
    - docker/login-action@v4 (GHCR authentication)
    - docker/metadata-action@v5 (OCI image tag generation)
    - docker/build-push-action@v6 (multi-arch build and push)
    - actions/checkout@v4
    - actions/setup-go@v5
  patterns:
    - PR push guard: push conditional on github.event_name != 'pull_request'
    - go-version-file pattern: reads Go version from go.mod, avoids hardcoded version
    - metadata-action semver pattern: type=semver with major/minor/patch patterns + latest=auto flavor
    - GHA layer cache: type=gha,mode=max for BuildKit layer caching across runs
    - Concurrency cancel-in-progress for PR deduplication

key-files:
  created:
    - .github/workflows/ci.yml
    - README.md
  modified: []

key-decisions:
  - "go-version-file: receiver/tetragonreceiver/go.mod rather than hardcoded version — auto-tracks module minimum"
  - "flavor: latest=auto prevents pre-release tags (v1.0.0-rc.1) from receiving the latest tag"
  - "QEMU registered before Buildx — required for arm64 emulation on amd64 runners (Pitfall 2)"
  - "Image name hardcoded as ghcr.io/cilium/otelcol-tetragon rather than constructed from github.repository — safer for lowercase guarantee"
  - "type=gha cache backend covers OCB module download layers after first cold build"

patterns-established:
  - "PR guard pattern: if: github.event_name != 'pull_request' on login step, push: ${{ github.event_name != 'pull_request' }} on build step"
  - "Defaults working-directory scoped to test job only, not workflow-level — build-push job uses repo root"

requirements-completed: [CICD-01, CICD-02, CICD-03, CICD-04, CICD-05, PROJ-04]

# Metrics
duration: 10min
completed: 2026-03-18
---

# Phase 3 Plan 01: CI/CD Workflow and README Summary

**Two-job GitHub Actions pipeline publishing multi-arch otelcol-tetragon image to GHCR on push/tag, with full project README covering usage, config reference, and component listing**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-03-18T14:00:00Z
- **Completed:** 2026-03-18T14:10:00Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- GitHub Actions CI workflow with test job (Go tests in receiver submodule using go-version-file) and build-push job (QEMU + Buildx multi-arch, GHCR push, metadata-action semver tags, GHA layer cache)
- README.md with docker pull/run usage, full configuration reference table (endpoint, tls, retry fields), build from source instructions, and OTel components table
- All 25 acceptance criteria satisfied across both files; existing Go tests still pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Create GitHub Actions CI workflow** - `dcd3ae4` (feat)
2. **Task 2: Create project README** - `330cf7f` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `.github/workflows/ci.yml` - Two-job CI/CD pipeline: test (Go) + build-push (multi-arch Docker to GHCR)
- `README.md` - Project documentation: overview, usage, configuration reference, build instructions, components

## Decisions Made

- Used `go-version-file: receiver/tetragonreceiver/go.mod` instead of hardcoded Go version to auto-track the module minimum (Go 1.25.0).
- Used `flavor: latest=auto` in metadata-action to prevent pre-release semver tags from receiving the `latest` tag.
- QEMU registered before Buildx per the research pitfall note — required for arm64 emulation on amd64 runners.
- Image name hardcoded as `ghcr.io/cilium/otelcol-tetragon` rather than constructed dynamically from `${{ github.repository }}` to guarantee lowercase compliance.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required beyond the GHCR package becoming public after first push (documented in RESEARCH.md Open Questions).

## Next Phase Readiness

All three phases complete:
1. Phase 01 (receiver-module): tetragonreceiver implementation with full test coverage
2. Phase 02 (distribution): OCB-built custom collector, multi-arch Containerfile
3. Phase 03 (cicd-and-release): GitHub Actions workflow + README

The project is fully ready for use. After the first push to `main` in the `cilium/otelcol-tetragon` repository, the workflow will publish `ghcr.io/cilium/otelcol-tetragon:latest` and `ghcr.io/cilium/otelcol-tetragon:sha-<hash>`. The GHCR package will need to be linked to the repository and made public via GitHub package settings.

---
*Phase: 03-cicd-and-release*
*Completed: 2026-03-18*
