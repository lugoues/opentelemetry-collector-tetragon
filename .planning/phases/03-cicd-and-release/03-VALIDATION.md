---
phase: 3
slug: cicd-and-release
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-18
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package |
| **Config file** | None — `go test ./...` in `receiver/tetragonreceiver/` |
| **Quick run command** | `cd receiver/tetragonreceiver && go test ./...` |
| **Full suite command** | `cd receiver/tetragonreceiver && go test -race ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd receiver/tetragonreceiver && go test ./...`
- **After every plan wave:** Run `cd receiver/tetragonreceiver && go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 3-01-01 | 01 | 1 | CICD-01 | smoke | `yamllint .github/workflows/ci.yml` | ❌ W0 | ⬜ pending |
| 3-01-02 | 01 | 1 | CICD-02 | structural | `grep -q 'pull_request' .github/workflows/ci.yml` | ❌ W0 | ⬜ pending |
| 3-01-03 | 01 | 1 | CICD-03 | structural | `grep -q 'type=sha' .github/workflows/ci.yml` | ❌ W0 | ⬜ pending |
| 3-01-04 | 01 | 1 | CICD-04 | structural | `grep -q 'type=semver' .github/workflows/ci.yml` | ❌ W0 | ⬜ pending |
| 3-01-05 | 01 | 1 | CICD-05 | structural | `grep -q 'linux/amd64,linux/arm64' .github/workflows/ci.yml` | ❌ W0 | ⬜ pending |
| 3-02-01 | 02 | 1 | PROJ-04 | smoke | `test -f README.md && grep -q '## Usage' README.md` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `.github/workflows/ci.yml` — CI/CD workflow (CICD-01 through CICD-05)
- [ ] `README.md` — project documentation (PROJ-04)

*These are the primary deliverables of this phase, not test infrastructure gaps.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| PR build does not push image | CICD-02 | Requires actual GitHub Actions run on a PR event | Open PR, verify workflow run has `push: false` in build-push step logs |
| Main push produces `latest` + `sha-*` tags | CICD-03 | Requires actual push to main and GHCR inspection | Merge to main, run `docker buildx imagetools inspect ghcr.io/cilium/otelcol-tetragon:latest` |
| Semver tag produces versioned tags | CICD-04 | Requires actual tag push and GHCR inspection | Push `v0.1.0` tag, verify `0.1.0`, `0.1`, `0` tags exist in GHCR |
| Multi-arch manifest has amd64 + arm64 | CICD-05 | Requires actual image push and manifest inspection | `docker manifest inspect ghcr.io/cilium/otelcol-tetragon:latest` shows both platforms |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
