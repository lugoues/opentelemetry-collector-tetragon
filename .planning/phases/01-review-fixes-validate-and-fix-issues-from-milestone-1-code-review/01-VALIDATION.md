---
phase: 1
slug: review-fixes
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-18
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing + testify v1.11.1 |
| **Config file** | none — standard `go test` |
| **Quick run command** | `cd receiver/tetragonreceiver && go test -race ./...` |
| **Full suite command** | `cd receiver/tetragonreceiver && go test -race -count=1 ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd receiver/tetragonreceiver && go test -race ./...`
- **After every plan wave:** Run `cd receiver/tetragonreceiver && go test -race -count=1 ./... && go vet ./...`
- **Before `/gsd:verify-work`:** Full suite must be green + new CI lint job green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | RFX-01 | code review | `go vet ./...` | ✅ | ⬜ pending |
| 01-01-02 | 01 | 1 | RFX-02 | unit | `go test -race -run TestReceiverReconnectsOnStreamError ./...` | ✅ (extend) | ⬜ pending |
| 01-01-03 | 01 | 1 | RFX-03 | unit | `go test -race -run TestReceiver ./...` | ✅ | ⬜ pending |
| 01-01-04 | 01 | 1 | RFX-04 | unit | `go test -race -run TestReceiverStartShutdown ./...` | ✅ | ⬜ pending |
| 01-01-05 | 01 | 1 | RFX-05 | code review | `go vet ./...` | ✅ | ⬜ pending |
| 01-01-06 | 01 | 1 | RFX-06 | unit | `go test -race -run TestReceiverConsumesLogs ./...` | ✅ | ⬜ pending |
| 01-02-01 | 02 | 2 | RFX-08 | compile | `go build ./...` | ✅ | ⬜ pending |
| 01-02-02 | 02 | 2 | RFX-09 | unit | `go test -race -run TestConfigStruct ./...` | ❌ W0 | ⬜ pending |
| 01-02-03 | 02 | 2 | RFX-10 | unit | `go test -race -run TestConfigValidate ./...` | ✅ (extend) | ⬜ pending |
| 01-02-04 | 02 | 2 | RFX-11 | code review | n/a | ✅ | ⬜ pending |
| 01-03-01 | 03 | 3 | RFX-07 | config review | CI run | ❌ W0 | ⬜ pending |
| 01-03-02 | 03 | 3 | RFX-12 | existence | `ls LICENSE` | ❌ W0 | ⬜ pending |
| 01-03-03 | 03 | 3 | RFX-13 | review | n/a | ✅ | ⬜ pending |
| 01-03-04 | 03 | 3 | RFX-14 | review | n/a | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `receiver/tetragonreceiver/config_test.go` — add `TestConfigStruct` stub (RFX-09)
- [ ] `LICENSE` — Apache-2.0 file in repo root (RFX-12)
- [ ] `.github/workflows/ci.yml` — add `go vet ./...` step (RFX-07)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Dead code removed | RFX-01 | Code deletion — visual review | Confirm `backoff.Stop` branch removed from `receiver.go` |
| `//go:generate` directive | RFX-11 | Convention compliance | Confirm directive in `doc.go` |
| Attribute docs updated | RFX-13 | Documentation quality | Review attribute table in README |
| Containerfile comment | RFX-14 | Documentation quality | Verify comment in Containerfile |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
