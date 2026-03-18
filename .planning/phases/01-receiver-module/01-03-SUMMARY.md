---
phase: 01-receiver-module
plan: "03"
subsystem: receiver
tags: [go, opentelemetry, tetragon, grpc, backoff, obsreport, componentstatus, lifecycle, tests]

# Dependency graph
requires:
  - "01-01 (receiver module scaffold with factory, config, go.mod)"
  - "01-02 (convertEvent() converting GetEventsResponse to plog.Logs)"
provides:
  - "tetragonReceiver.Start(): non-blocking gRPC dial + stream goroutine spawn"
  - "tetragonReceiver.Shutdown(): cancel + wait + conn.Close() with no-op guard"
  - "streamEvents() with exponential backoff reconnection via cenkalti/backoff v5"
  - "runStream() reading events into 1000-event buffered channel with 80% capacity warning"
  - "consumeChannel() decoupling Recv from ConsumeLogs via buffered channel"
  - "obsReport tracking accepted/refused log records via StartLogsOp/EndLogsOp"
  - "componentstatus.StatusOK on connect, RecoverableError on disconnect"
  - "6 receiver lifecycle tests: start/shutdown, reconnection, clean shutdown during backoff, log consumption"
affects:
  - "02-distribution (this receiver is the component being distributed)"

# Tech tracking
tech-stack:
  added:
    - "go.opentelemetry.io/collector/receiver/receiverhelper v0.148.0 — ObsReport for telemetry"
    - "go.opentelemetry.io/collector/component/componentstatus v0.148.0 — health status reporting"
    - "github.com/cenkalti/backoff/v5 v5.0.3 — exponential backoff for reconnection"
  patterns:
    - "Start() dials gRPC only when r.client == nil, enabling test injection without real gRPC"
    - "context.WithCancel(context.Background()) in Start() — never pass Start's ctx to goroutine (Pitfall 1)"
    - "cancel initialized to no-op func(){} in factory — Shutdown-before-Start safety (Pitfall 6)"
    - "Two-goroutine design: streamEvents owns the channel, consumeChannel drains it"
    - "mockTetragonClientFn pattern: function-based mock for dynamic GetEvents behavior per call"
    - "nopHost inline struct satisfying component.Host without importing componenttest"

key-files:
  created:
    - "receiver/tetragonreceiver/receiver.go"
    - "receiver/tetragonreceiver/receiver_test.go"
  modified:
    - "receiver/tetragonreceiver/factory.go"
    - "receiver/tetragonreceiver/go.mod"
    - "receiver/tetragonreceiver/go.sum"
    - "receiver/tetragonreceiver/testdata/golden/*.yaml (10 files refreshed)"

key-decisions:
  - "cenkalti/backoff v5 (not v4): go.mod already had v5 — InitialInterval/MaxInterval set directly, MaxElapsedTime removed (v5 struct change)"
  - "ToClientConn takes map[component.ID]component.Component not component.Host — plan had incorrect signature; use host.GetExtensions()"
  - "r.client != nil guard in Start() enables test injection without changing production gRPC dial path"
  - "protojson v2 emits spaces after commas — golden files regenerated to match current output"

requirements-completed: [RECV-02, RECV-03, RECV-04, RECV-05, RECV-06, RECV-07, RECV-08]

# Metrics
duration: 25min
completed: 2026-03-18
---

# Phase 1 Plan 03: Receiver Lifecycle Summary

**Full gRPC streaming receiver lifecycle with Start/Shutdown, exponential backoff reconnection via backoff v5, 1000-event buffered channel with 80% capacity warning, obsreport telemetry, componentstatus health reporting, and 6 race-clean lifecycle tests**

## Performance

- **Duration:** ~25 min
- **Started:** 2026-03-18T05:00:00Z
- **Completed:** 2026-03-18T05:25:00Z
- **Tasks:** 2 of 2
- **Files modified:** 6 (plus 10 golden files refreshed)

## Accomplishments

- `receiver.go` implements the full operational core: Start/Shutdown lifecycle, two-goroutine design (streamEvents + consumeChannel), 1000-event buffered channel, 80% buffer capacity warning, exponential backoff reconnection, obsreport accepted/refused tracking, componentstatus health reporting
- `factory.go` updated to create ObsReport via receiverhelper.NewObsReport and store full `settings receiver.Settings` for ToClientConn telemetry
- `receiver_test.go` with 6 passing tests: start/shutdown non-blocking, shutdown-before-start safety, stream events forwarding, reconnection on error, clean shutdown during backoff, log attribute verification
- All 27 tests pass with race detector (`go test -race ./...`)

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement receiver.go with stream loop, backpressure channel, buffer warning, and telemetry** - `73fb293` (feat)
2. **Task 2: Write receiver_test.go with lifecycle, reconnection, and shutdown tests** - `e49ef64` (feat)

## Files Created/Modified

- `receiver/tetragonreceiver/receiver.go` — tetragonClient interface, tetragonReceiver struct, Start/Shutdown/streamEvents/runStream/consumeChannel
- `receiver/tetragonreceiver/factory.go` — createLogsReceiver now creates ObsReport, stores settings field
- `receiver/tetragonreceiver/receiver_test.go` — mockGetEventsClient, mockTetragonClient, mockTetragonClientFn, nopHost, 6 TestReceiver* tests
- `receiver/tetragonreceiver/go.mod` — added receiverhelper v0.148.0, componentstatus v0.148.0
- `receiver/tetragonreceiver/testdata/golden/*.yaml` — 10 golden files refreshed for protojson v2 spacing

## Decisions Made

- **backoff v5 API:** Plan referenced `github.com/cenkalti/backoff/v4` but go.mod already pinned v5. The v5 `ExponentialBackOff` struct dropped `MaxElapsedTime` from the exported struct (it's now set via `NewExponentialBackOff()` with defaults). Set `InitialInterval` and `MaxInterval` from config; omit MaxElapsedTime (retry forever, matching the pre-phase "retry forever" decision from CONTEXT.md).
- **ToClientConn API mismatch:** Plan specified `ToClientConn(ctx, host, settings.TelemetrySettings)` but actual v0.148.0 signature is `ToClientConn(ctx, extensions map[component.ID]component.Component, settings.TelemetrySettings)`. Fixed to use `host.GetExtensions()`.
- **Test injection pattern:** Added `if r.client == nil` guard in Start() to allow tests to bypass gRPC dial by pre-setting `r.client`. Clean one-line change, zero impact on production path.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] backoff v5 struct has no MaxElapsedTime field**
- **Found during:** Task 1 (receiver.go implementation)
- **Issue:** Plan's code used `b.MaxElapsedTime = r.cfg.Retry.MaxElapsedTime` but cenkalti/backoff v5 removed MaxElapsedTime from ExponentialBackOff struct exports
- **Fix:** Removed MaxElapsedTime assignment; kept InitialInterval and MaxInterval from config. Retry forever (aligns with CONTEXT.md "retry forever" decision)
- **Files modified:** `receiver/tetragonreceiver/receiver.go`
- **Verification:** Compiles clean, backoff still functional
- **Committed in:** `73fb293` (Task 1 commit)

**2. [Rule 1 - Bug] ToClientConn takes extensions map, not host**
- **Found during:** Task 1 (checking actual configgrpc v0.148.0 source)
- **Issue:** Plan specified `ToClientConn(ctx, host, r.settings.TelemetrySettings)` but actual signature is `ToClientConn(ctx context.Context, extensions map[component.ID]component.Component, settings component.TelemetrySettings, ...)`
- **Fix:** Changed call to `r.cfg.ClientConfig.ToClientConn(ctx, host.GetExtensions(), r.settings.TelemetrySettings)`
- **Files modified:** `receiver/tetragonreceiver/receiver.go`
- **Verification:** Builds clean
- **Committed in:** `73fb293` (Task 1 commit)

**3. [Rule 1 - Bug] Golden files stale against protojson v2 output**
- **Found during:** Task 2 (full test run after writing receiver_test.go)
- **Issue:** protojson v2 now emits spaces after commas in JSON output (`, ` not `,`). All 10 golden YAML files had the compact format from the previous plan's generation; they failed body comparison in TestConvertEvent_Golden
- **Fix:** Ran `UPDATE_GOLDEN=true go test -run TestConvertEvent_Golden` to regenerate all 10 golden files with current protojson output
- **Files modified:** `testdata/golden/*.yaml` (all 10)
- **Verification:** All golden tests pass
- **Committed in:** `e49ef64` (Task 2 commit)

---

**Total deviations:** 3 auto-fixed (all Rule 1 — API version mismatches between plan and actual library)
**Impact on plan:** All fixes necessary for compilation and test correctness. No scope creep.

## Issues Encountered

- Plan's backoff import path (`github.com/cenkalti/backoff/v4`) didn't match go.mod (`v5`) — caught immediately at import time
- protojson v2 output format changed between plan generation and execution — gold files needed one-command refresh
- `FineGuidanceSensors_GetEventsClient` is `grpc.ServerStreamingClient[GetEventsResponse]` (generic type alias) — mock required implementing full `grpc.ClientStream` interface (Header, Trailer, CloseSend, Context, SendMsg, RecvMsg) plus Recv

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Full receiver module is complete: factory, config, converter, and lifecycle all implemented and tested
- `convertEvent()` and receiver lifecycle tested end-to-end via mock gRPC client
- Ready for Phase 2 (OCB distribution): `go.mod` module path `github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver` can be referenced via OCB `replaces` directive

## Self-Check: PASSED

Files verified:
- `receiver/tetragonreceiver/receiver.go` — exists, builds clean
- `receiver/tetragonreceiver/receiver_test.go` — exists, all 6 TestReceiver* tests pass
- `receiver/tetragonreceiver/factory.go` — updated with obsReport and settings
- `receiver/tetragonreceiver/testdata/golden/` — 10 refreshed YAML files

Commits verified in git log:
- `73fb293` feat(01-03): implement receiver lifecycle with stream loop, backpressure channel, and telemetry
- `e49ef64` feat(01-03): add receiver lifecycle tests and refresh golden files

Full test suite: 27 tests, 0 failures, race detector clean.

---
*Phase: 01-receiver-module*
*Completed: 2026-03-18*
