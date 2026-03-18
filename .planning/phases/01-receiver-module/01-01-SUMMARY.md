---
phase: 01-receiver-module
plan: "01"
subsystem: receiver
tags: [go, opentelemetry, grpc, tetragon, configgrpc, receiver]

# Dependency graph
requires: []
provides:
  - "receiver/tetragonreceiver Go module with go.mod, go.sum"
  - "Config struct with configgrpc.ClientConfig squash pattern and Validate()"
  - "Factory registering 'tetragon' as logs receiver with alpha stability"
  - "Stub tetragonReceiver with no-op cancel guard for Shutdown-before-Start safety"
  - "mise.toml with Go 1.25 and test/lint/build/tidy tasks"
affects:
  - 01-02 (receiver implementation will extend factory/config)
  - 01-03 (event converter will extend factory/config)
  - 02-distribution (OCB distribution consumes this module)

# Tech tracking
tech-stack:
  added:
    - "go.opentelemetry.io/collector/component v1.54.0"
    - "go.opentelemetry.io/collector/config/configgrpc v0.148.0"
    - "go.opentelemetry.io/collector/config/configretry v1.54.0"
    - "go.opentelemetry.io/collector/config/configtls v1.54.0"
    - "go.opentelemetry.io/collector/consumer v1.54.0"
    - "go.opentelemetry.io/collector/receiver v1.54.0"
    - "go.opentelemetry.io/collector/consumer/consumertest v0.148.0"
    - "go.opentelemetry.io/collector/receiver/receivertest v0.148.0"
    - "go.uber.org/zap v1.27.1"
    - "google.golang.org/grpc v1.79.2"
    - "go 1.25 toolchain via mise"
  patterns:
    - "configgrpc.ClientConfig embedded with mapstructure:\",squash\" for standard OTel gRPC config"
    - "Config.Validate() pattern: check receiver-specific fields first, then delegate to embedded ClientConfig.Validate()"
    - "Factory no-op cancel guard: r.cancel = func(){} initialized in factory for Shutdown-before-Start safety"
    - "component.MustNewType() for factory type registration"
    - "receiver.WithLogs() with component.StabilityLevelAlpha for logs receiver registration"

key-files:
  created:
    - "receiver/tetragonreceiver/go.mod"
    - "receiver/tetragonreceiver/go.sum"
    - "receiver/tetragonreceiver/doc.go"
    - "receiver/tetragonreceiver/metadata.yaml"
    - "receiver/tetragonreceiver/config.go"
    - "receiver/tetragonreceiver/config_test.go"
    - "receiver/tetragonreceiver/factory.go"
    - "receiver/tetragonreceiver/factory_test.go"
    - "receiver/tetragonreceiver/testdata/config.yaml"
    - "receiver/tetragonreceiver/testdata/config_invalid.yaml"
  modified:
    - ".mise/config.toml"

key-decisions:
  - "Go 1.25 required (not 1.24 as planned) — tetragon/api v1.6.0 and OTel v1.54.0 both declare go 1.25.0 minimum"
  - "consumer/consumertest is at v0.148.0 not v1.54.0 — unstable track, plan had incorrect version"
  - "confmap.WithIgnoreUnused() used in TestConfigFromYAML to handle squash-embedded struct field mapping"

patterns-established:
  - "Config embed: configgrpc.ClientConfig with squash tag, Validate() checks endpoint then delegates"
  - "Factory init: no-op cancel func guard, stub Start/Shutdown returning nil"
  - "Test approach: internal package tests (package tetragonreceiver) with confmaptest for YAML loading"

requirements-completed: [PROJ-01, PROJ-03, RECV-01, CONF-01, CONF-02, CONF-03, CONF-04]

# Metrics
duration: 35min
completed: 2026-03-18
---

# Phase 1 Plan 01: Receiver Module Scaffold Summary

**tetragonreceiver Go module with configgrpc squash config, factory registration as 'tetragon' logs receiver, and mise.toml with Go 1.25 tooling**

## Performance

- **Duration:** ~35 min
- **Started:** 2026-03-18T03:57:34Z
- **Completed:** 2026-03-18T04:32:00Z
- **Tasks:** 2 of 2
- **Files modified:** 11

## Accomplishments

- Standalone Go module at `receiver/tetragonreceiver/` compiles and all 9 tests pass
- Config struct uses `configgrpc.ClientConfig` squash pattern with `Validate()` rejecting empty endpoint and delegating TLS validation to embedded ClientConfig (CONF-01, CONF-02)
- Factory registers as "tetragon" type with `receiver.WithLogs` at alpha stability; stub receiver has no-op cancel guard for Shutdown-before-Start safety
- mise.toml updated to Go 1.25 with test/lint/build/tidy tasks targeting the receiver module

## Task Commits

Each task was committed atomically:

1. **Task 1: Create Go module, config, and factory with tests** - `856c2f6` (feat)
2. **Task 2: Update mise.toml with Go tooling and project tasks** - `3092d3b` (chore)

**Plan metadata:** (pending final commit)

## Files Created/Modified

- `receiver/tetragonreceiver/go.mod` — Module declaration with go 1.25 and pinned OTel/Tetragon deps
- `receiver/tetragonreceiver/go.sum` — Generated checksum file
- `receiver/tetragonreceiver/doc.go` — Package comment
- `receiver/tetragonreceiver/metadata.yaml` — OTel component metadata (type: tetragon, alpha logs)
- `receiver/tetragonreceiver/config.go` — Config struct with configgrpc squash, Validate(), createDefaultConfig()
- `receiver/tetragonreceiver/config_test.go` — 5 config tests including TLS delegation and YAML loading
- `receiver/tetragonreceiver/factory.go` — NewFactory(), tetragonReceiver stub with no-op cancel
- `receiver/tetragonreceiver/factory_test.go` — 4 factory tests including Shutdown-before-Start
- `receiver/tetragonreceiver/testdata/config.yaml` — Valid config fixture
- `receiver/tetragonreceiver/testdata/config_invalid.yaml` — Invalid config fixture (empty endpoint)
- `.mise/config.toml` — Updated from bun+go:1.20 to go:1.25 with project tasks

## Decisions Made

- **Go 1.25 over 1.24:** Plan specified go 1.24 but `github.com/cilium/tetragon/api v1.6.0` and `go.opentelemetry.io/collector/component v1.54.0` both declare `go 1.25.0` minimum. Updated go.mod and mise.toml to 1.25 (auto-fix, Rule 1).
- **consumer/consumertest at v0.148.0:** Plan listed `v1.54.0` which doesn't exist — consumertest is on the unstable track (v0.x). go mod tidy resolved to correct version.
- **confmap.WithIgnoreUnused() in YAML test:** The `cm.Unmarshal(cfg)` without options failed with "invalid keys" error because confmap strict mode doesn't recognize squash-flattened keys at the top level. WithIgnoreUnused() fixes this.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Go version updated from 1.24 to 1.25**
- **Found during:** Task 1 (go mod tidy)
- **Issue:** Plan specified `go 1.24` in go.mod and `go = "1.24"` in mise.toml, but `github.com/cilium/tetragon/api v1.6.0` and `go.opentelemetry.io/collector/component v1.54.0` both require `go 1.25.0`. `go mod tidy` failed with "requires go >= 1.25.0".
- **Fix:** Updated go.mod to `go 1.25` and mise.toml to `go = "1.25"`. Installed Go 1.25.8 via mise.
- **Files modified:** `receiver/tetragonreceiver/go.mod`, `.mise/config.toml`
- **Verification:** `go mod tidy` succeeded; `go build ./...` exits 0; all tests pass
- **Committed in:** `856c2f6` (Task 1 commit) and `3092d3b` (Task 2 commit)

**2. [Rule 1 - Bug] Fixed incorrect consumertest version in go.mod**
- **Found during:** Task 1 (go mod tidy)
- **Issue:** Plan specified `go.opentelemetry.io/collector/consumer/consumertest v1.54.0` which doesn't exist — latest is `v0.148.0` (unstable track).
- **Fix:** Updated go.mod to use `consumertest v0.148.0`. go mod tidy confirmed and resolved.
- **Files modified:** `receiver/tetragonreceiver/go.mod`
- **Verification:** `go mod tidy` succeeded without errors
- **Committed in:** `856c2f6` (Task 1 commit)

**3. [Rule 1 - Bug] Fixed TestConfigFromYAML unmarshal failure**
- **Found during:** Task 1 (go test)
- **Issue:** `cm.Unmarshal(cfg)` failed with `'internal.Conf' has invalid keys: endpoint, retry, tls` because confmap strict mode doesn't recognize squash-embedded fields at the top level.
- **Fix:** Changed to `cm.Unmarshal(cfg, confmap.WithIgnoreUnused())` to allow top-level keys from the squashed ClientConfig.
- **Files modified:** `receiver/tetragonreceiver/config_test.go`
- **Verification:** Test passes: `TestConfigFromYAML` asserts all fields correctly loaded
- **Committed in:** `856c2f6` (Task 1 commit)

---

**Total deviations:** 3 auto-fixed (all Rule 1 - incorrect version specs and unmarshal bug)
**Impact on plan:** All fixes necessary for compilation and test correctness. No scope creep.

## Issues Encountered

- Plan's OTel version matrix was partially incorrect: `consumer/consumertest` is on unstable track (v0.x) not stable (v1.x), and all OTel v1.54.0 packages require go 1.25.0 minimum. `go mod tidy` resolved all transitive dependencies correctly after fixing the declared versions.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Module skeleton complete; ready for Plan 02 (gRPC stream loop implementation)
- Config and factory patterns established for Plans 02-03 to extend
- mise tasks operational: `mise run test`, `mise run build`, `mise run lint`, `mise run tidy`

## Self-Check: PASSED

All files exist on disk, all commits verified in git log:
- `856c2f6` feat(01-01): scaffold tetragonreceiver Go module with config and factory
- `3092d3b` chore(01-01): update mise.toml with Go 1.25 and project tasks
- `6d44cb5` docs(01-01): complete receiver module scaffold plan summary

---
*Phase: 01-receiver-module*
*Completed: 2026-03-18*
