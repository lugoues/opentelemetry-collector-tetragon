# Roadmap: OTel Collector Tetragon Receiver

## Milestones

- ✅ **v1.0 MVP** — Phases 1-3 (shipped 2026-03-18) — [archive](milestones/v1.0-ROADMAP.md)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1-3) — SHIPPED 2026-03-18</summary>

- [x] Phase 1: Receiver Module (3/3 plans) — completed 2026-03-18
- [x] Phase 2: Distribution (2/2 plans) — completed 2026-03-18
- [x] Phase 3: CI/CD and Release (1/1 plan) — completed 2026-03-18

</details>

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Receiver Module | v1.0 | 3/3 | Complete | 2026-03-18 |
| 2. Distribution | v1.0 | 2/2 | Complete | 2026-03-18 |
| 3. CI/CD and Release | v1.0 | 1/1 | Complete | 2026-03-18 |

### Phase 1: Review fixes — validate and fix issues from milestone 1 code review

**Goal:** Fix all confirmed bugs and code quality issues from the milestone 1 code review (14 requirements: 3 production bugs, 6 test/config improvements, 5 convention/doc fixes)
**Requirements**: RFX-01, RFX-02, RFX-03, RFX-04, RFX-05, RFX-06, RFX-07, RFX-08, RFX-09, RFX-10, RFX-11, RFX-12, RFX-13, RFX-14
**Depends on:** v1.0 MVP
**Plans:** 3 plans

Plans:
- [ ] 01-01-PLAN.md — Fix production bugs: backoff dead code, backoff reset, buffer warning rate-limit
- [ ] 01-02-PLAN.md — Fix test mocks, adopt OTel test helpers, add config validation
- [ ] 01-03-PLAN.md — Add CI lint, LICENSE, go:generate directive, documentation
