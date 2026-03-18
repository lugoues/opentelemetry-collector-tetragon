---
phase: 01-review-fixes-validate-and-fix-issues-from-milestone-1-code-review
plan: "01"
subsystem: receiver
tags: [go, backoff, retry, protojson, golden-files, race-detector]

# Dependency graph
requires:
  - phase: 01-receiver-module
    provides: receiver.go with cenkalti/backoff/v5 and runStream/streamEvents loop
provides:
  - "Dead backoff.Stop branch removed from streamEvents (RFX-01)"
  - "b.Reset() called after stream failure so retries start from InitialInterval (RFX-02)"
  - "Buffer warning rate-limited to 10s intervals (RFX-03)"
  - "Compact deterministic JSON body output via json.Compact post-processing"
  - "Golden files regenerated with compact JSON"
  - "TestReceiverStartShutdown unblocks on streamCtx cancellation (RFX-04 fix)"
affects:
  - 01-review-fixes-validate-and-fix-issues-from-milestone-1-code-review

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "json.Compact() post-processes protojson output for deterministic whitespace"
    - "mockGetEventsClient.streamCtx threads call context into mock Recv() for ctx-aware blocking"
    - "b.Reset() placement: after ctx.Err() check, before componentstatus.ReportStatus"

key-files:
  created: []
  modified:
    - receiver/tetragonreceiver/receiver.go
    - receiver/tetragonreceiver/convert.go
    - receiver/tetragonreceiver/receiver_test.go
    - receiver/tetragonreceiver/testdata/golden/ (10 golden files)

key-decisions:
  - "b.Reset() placed after ctx.Err() clean-shutdown check per research Pitfall 2 (not before)"
  - "bufferWarnInterval declared as local const in runStream (not package-level) to avoid expanding API"
  - "json.Compact() applied post-protojson to neutralize non-deterministic whitespace across test runs"
  - "streamCtx threaded into mockGetEventsClient so Recv() mirrors real gRPC context cancellation"

patterns-established:
  - "Golden file tests: always verify UPDATE_GOLDEN writes compact/deterministic output before committing"
  - "Mock gRPC streams: thread the call context so Recv() unblocks on cancellation like real gRPC"

requirements-completed: [RFX-01, RFX-02, RFX-03]

# Metrics
duration: 9min
completed: "2026-03-18"
---

# Phase 01 Plan 01: Review Fixes — Backoff and Buffer Warning Summary

**Removed dead backoff.Stop branch, added b.Reset() for reliable reconnect backoff, and rate-limited buffer warning to 10s intervals using cenkalti/backoff/v5 semantics**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-18T17:15:22Z
- **Completed:** 2026-03-18T17:24:50Z
- **Tasks:** 2
- **Files modified:** 13

## Accomplishments

- Removed dead code: `if wait == backoff.Stop` branch in streamEvents can never execute in backoff/v5 (v5 ExponentialBackOff.NextBackOff() always returns positive duration, never backoff.Stop)
- Fixed production reliability bug: `b.Reset()` now called after each runStream return so reconnect after a long-running stream always starts at InitialInterval (1s) instead of accumulated MaxInterval (30s)
- Rate-limited buffer-full warning: fires at most once per 10 seconds via local `lastBufferWarn`/`bufferWarnInterval` variables in runStream; prevents log flooding under sustained backpressure
- Fixed protojson non-determinism: `json.Compact()` post-processing ensures stable compact JSON body across all test runs; all 10 golden files regenerated
- Fixed TestReceiverStartShutdown 5s hang: mock Recv() now unblocks on streamCtx cancellation mirroring real gRPC behavior

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix backoff dead code and reset behavior** - `0819bd5` (fix)
2. **Task 2: Rate-limit buffer warning + auto-fixed pre-existing failures** - `f8ead9b` (fix)

## Files Created/Modified

- `receiver/tetragonreceiver/receiver.go` - streamEvents: removed backoff.Stop branch, added b.Reset(); runStream: added lastBufferWarn/bufferWarnInterval rate limiting
- `receiver/tetragonreceiver/convert.go` - Added json.Compact() post-processing of protojson output for deterministic whitespace
- `receiver/tetragonreceiver/receiver_test.go` - mockGetEventsClient: added streamCtx field; mockTetragonClient.GetEvents: threads ctx into stream; mock Recv() unblocks on either blockCtx or streamCtx
- `receiver/tetragonreceiver/testdata/golden/*.yaml` (10 files) - Regenerated with compact JSON body format

## Decisions Made

- `b.Reset()` placed after `ctx.Err()` check, not before — preserves clean-shutdown detection flow per research Pitfall 2
- `bufferWarnInterval` declared as local const inside `runStream` rather than package-level const — keeps package API clean, avoids scope creep
- Applied `json.Compact()` in convert.go rather than changing test comparison — compact JSON is valid and correct, fixing the source is preferable to tolerating non-determinism in tests
- Threaded `streamCtx` into mockGetEventsClient rather than adding a separate cancel parameter to test — matches how real gRPC streams work

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed protojson non-deterministic whitespace in golden file tests**
- **Found during:** Task 2 (verifying go test -race -count=1 ./...)
- **Issue:** protojson.MarshalOptions produces space-separated JSON on some runs and compact JSON on others; golden files stored spaces; test comparison (exact string match on body) failed non-deterministically
- **Fix:** Added `json.Compact(&buf, raw)` after protojson.Marshal in convert.go; regenerated all 10 golden files with compact format
- **Files modified:** receiver/tetragonreceiver/convert.go, receiver/tetragonreceiver/testdata/golden/*.yaml
- **Verification:** All golden tests pass; TestConvertEvent_BodySnakeCase still passes; compact JSON is semantically identical
- **Committed in:** f8ead9b (Task 2 commit)

**2. [Rule 1 - Bug] Fixed TestReceiverStartShutdown hanging 5 seconds**
- **Found during:** Task 2 (go test -race -count=1 ./...)
- **Issue:** mockGetEventsClient.Recv() blocked on blockCtx.Done() but Shutdown() cancels streamCtx (different context); Recv() never unblocked on shutdown, causing 5s timeout
- **Fix:** Added streamCtx field to mockGetEventsClient; mockTetragonClient.GetEvents threads ctx into stream; Recv() now select-waits on both blockCtx.Done() and streamCtx.Done()
- **Files modified:** receiver/tetragonreceiver/receiver_test.go
- **Verification:** TestReceiverStartShutdown completes in <2s with race detector
- **Committed in:** f8ead9b (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (2 Rule 1 bugs)
**Impact on plan:** Both auto-fixes required for test suite correctness. No scope creep — these are pre-existing bugs in tests committed from a prior session.

## Issues Encountered

- protojson whitespace non-determinism: discovered that `go run` and `go test` can produce different whitespace in the same protojson call; `json.Compact()` is the canonical fix
- Pre-existing test modifications in working tree (go.mod, receiver_test.go, STATE.md) were left from a prior incomplete session; these had RFX-04/05/06/08 fixes partially applied but the StreamCtx threading was missing

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 01-01 complete: RFX-01, RFX-02, RFX-03 fixed, all tests passing with race detector
- Ready for Plan 01-02 (test quality fixes: RFX-07 CI lint, RFX-08/09/10 config validation)
- No blockers

## Self-Check: PASSED

All files exist and all commits verified:
- receiver/tetragonreceiver/receiver.go: FOUND
- receiver/tetragonreceiver/convert.go: FOUND
- receiver/tetragonreceiver/receiver_test.go: FOUND
- .planning/phases/.../01-01-SUMMARY.md: FOUND
- commit 0819bd5 (Task 1): FOUND
- commit f8ead9b (Task 2): FOUND

---
*Phase: 01-review-fixes-validate-and-fix-issues-from-milestone-1-code-review*
*Completed: 2026-03-18*
