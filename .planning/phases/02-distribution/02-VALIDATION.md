---
phase: 2
slug: distribution
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-18
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go test + OCB build + Docker smoke |
| **Config file** | none — OCB outputs to /tmp/dist |
| **Quick run command** | `builder --config distribution/builder-config.yaml --output-path /tmp/dist --skip-strict-versioning` |
| **Full suite command** | `mise run ocb && docker build -t otelcol-tetragon:local -f Containerfile .` |
| **Estimated runtime** | ~120 seconds |

---

## Sampling Rate

- **After every task commit:** Run `builder --config distribution/builder-config.yaml --output-path /tmp/dist --skip-strict-versioning`
- **After every plan wave:** Run `mise run ocb && docker build -t otelcol-tetragon:local -f Containerfile .`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | DIST-01 | smoke | `./dist/otelcol-tetragon components` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | DIST-02 | build | `go build ./...` from repo root | ❌ W0 | ⬜ pending |
| 02-01-03 | 01 | 1 | DIST-03 | build | `docker build -f Containerfile .` | ❌ W0 | ⬜ pending |
| 02-01-04 | 01 | 1 | DIST-04 | smoke | `docker run --rm otelcol-tetragon:local id` | ❌ W0 | ⬜ pending |
| 02-01-05 | 01 | 1 | DIST-05 | smoke | `docker run -d ... && curl -sf localhost:13133/` | ❌ W0 | ⬜ pending |
| 02-01-06 | 01 | 1 | PROJ-02 | smoke | `otelcol-tetragon validate --config rootfs/etc/otelcol/config.yaml` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `distribution/builder-config.yaml` — OCB manifest
- [ ] `Containerfile` — Multi-stage build
- [ ] `go.mod` (repo root) — Top-level module with replace
- [ ] `rootfs/etc/otelcol/config.yaml` — Example config
- [ ] `mise.toml` additions — `ocb`, `container`, `smoke` tasks

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| journald reads journal when volume mounted | DIST-01 | Requires host journal volume | Mount `/var/log/journal` from host, verify logs appear in exporter |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 120s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
