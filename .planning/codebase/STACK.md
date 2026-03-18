# Technology Stack

**Analysis Date:** 2026-03-18

## Languages

**Primary:**
- JavaScript/TypeScript - Primary application language (as indicated by Bun runtime selection)

## Runtime

**Environment:**
- Bun (latest) - JavaScript runtime and package manager
  - Configured in: `.mise/config.toml`
  - Base image: Debian Trixie (mcr.microsoft.com/devcontainers/base:trixie)

**Package Manager:**
- Bun - Unified JavaScript runtime and package manager
- Lock file handling: Bun manages dependencies (uses bun.lock)

## Frameworks

**Build/Dev:**
- Mise (latest) - Tool version management for development environment
  - Configured in: `.mise/config.toml`

## Key Dependencies

**Infrastructure:**
- OpenTelemetry Collector components - Project is intended to integrate with OTEL ecosystem (based on project name "otel-collector-tetragon")
- Tetragon integration - Network security observability (implied by project name)

## Configuration

**Environment:**
- Development environment configured via DevContainer:
  - Image: `mcr.microsoft.com/devcontainers/base:trixie`
  - Features: Mise for version management
  - Config file: `.devcontainer/devcontainer.json`

**Build:**
- No build configuration files currently present
- Bun will handle build/run commands when configured

## Platform Requirements

**Development:**
- DevContainer support required
- Mise for tool version management
- Docker/Container runtime for DevContainer

**Production:**
- Debian-compatible Linux distribution (based on Trixie/Debian base)
- Bun runtime environment

---

*Stack analysis: 2026-03-18*
