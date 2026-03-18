# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.0 — MVP

**Shipped:** 2026-03-18
**Phases:** 3 | **Plans:** 6 | **Sessions:** 1

### What Was Built
- Standalone tetragonreceiver Go module with gRPC streaming, exponential backoff, and golden file tests for all 10 Tetragon event types
- OCB-built custom collector distribution with distroless container image
- GitHub Actions CI/CD pipeline publishing multi-arch images to GHCR
- Full project documentation with config reference, env var substitution, and mise dev workflow

### What Worked
- Research-first approach: deep phase research before planning caught API mismatches early (backoff v5, ToClientConn signature, protojson UInt32Value format)
- Golden file testing pattern: 10 fixtures × 10 golden YAMLs gave high confidence in converter correctness with minimal test code
- OCB path: field for local module: eliminated complex replace directive choreography in go.mod
- Phase 2 and 3 executed extremely fast (3 min and 10 min) because Phase 1 established all patterns

### What Was Inefficient
- Plan version specifications were frequently wrong (Go 1.24→1.25, consumertest v1.54→v0.148, backoff v4→v5, golden.WriteLogs API) — 9 auto-fixes across 6 plans
- journaldreceiver was included through all phases then removed post-audit — should have been scoped out during requirements
- Audit found smoke task was broken (missing volume mount) — should have been caught during Phase 2 verification

### Patterns Established
- configgrpc.ClientConfig embedded with `mapstructure:",squash"` for standard OTel gRPC config
- `r.client != nil` guard in Start() for test injection without real gRPC
- `context.WithCancel(context.Background())` in Start() — never pass Start's ctx to goroutine
- Two-goroutine design: streamEvents owns channel, consumeChannel drains it
- UPDATE_GOLDEN env var guard for golden.WriteLogsToFile

### Key Lessons
1. Research phase API versions against actual go.mod/go.sum — don't trust documentation or memory for exact version numbers
2. Question every component in the distribution: journaldreceiver had no business in a Tetragon gRPC receiver
3. Smoke tests should be run (or at least dry-run validated) during phase verification, not discovered during milestone audit

### Cost Observations
- Model mix: ~60% sonnet (agents), ~40% opus (orchestration)
- Sessions: 1 (single-day build)
- Notable: Phase 2 (3 min) and Phase 3 (10 min) were near-instant because Phase 1 (90 min) established all patterns and dependencies

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.0 | 1 | 3 | Initial build — research-first approach with golden file testing |

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v1.0 | 20 | n/a | 0 |

### Top Lessons (Verified Across Milestones)

1. Research actual library APIs before planning — version mismatches are the #1 source of plan deviations
