# Milestones

## v1.0 MVP (Shipped: 2026-03-18)

**Phases completed:** 3 phases, 6 plans, 11 tasks
**Timeline:** 2026-03-18 (46 commits, 78 files, 1,073 lines of Go)
**Git range:** 5efb10b..f4da507

**Key accomplishments:**
- Standalone tetragonreceiver Go module with gRPC streaming, exponential backoff reconnection, and 20 tests covering all 10 Tetragon event types
- Event-to-LogRecord converter with protojson body (snake_case), severity mapping, process/parent/k8s attribute extraction, and golden file validation
- OCB-built custom collector distribution with distroless container image
- GitHub Actions CI/CD pipeline publishing multi-arch (amd64/arm64) images to GHCR
- Full project documentation with configuration reference, env var substitution, and mise-based dev workflow

**Archives:**
- [v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- [v1.0-REQUIREMENTS.md](milestones/v1.0-REQUIREMENTS.md)
- [v1.0-MILESTONE-AUDIT.md](milestones/v1.0-MILESTONE-AUDIT.md)

---
