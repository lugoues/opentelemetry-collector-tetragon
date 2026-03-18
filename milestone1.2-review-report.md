# Milestone 1.2 Review — Patch Report

**Date:** 2026-03-18
**Commit:** a698b44
**Scope:** All 10 findings from `milestone1.2-review.md` validated against source, 6 patched.

---

## Findings Summary

| # | Title | Severity | Verdict | Action |
|---|-------|----------|---------|--------|
| 1 | Backoff reset defeats exponential backoff | Bug | Valid | **Patched** |
| 2 | `MaxElapsedTime: 0` semantics | Low | Valid | Skipped — comment-only, intent is correct |
| 3 | `consumeChannel` ignores consumer errors | Medium | Valid | **Patched** |
| 4 | Context propagation to `ConsumeLogs` during shutdown | Medium | Valid | **Patched** |
| 5 | Repeated type-switch blocks across convert.go | Low | Valid | Skipped — structural refactor, debatable trade-off |
| 6 | `time.Now()` in `convertEvent` not injectable | Low | Valid | Skipped — golden tests handle via `IgnoreObservedTimestamp` |
| 7 | Unnecessary `configtls` import | Trivial | Valid | Skipped — single usage is fine |
| 8 | `waitForLogs` polling pattern | Low | Valid | **Patched** |
| 9 | `atomic.AddInt32` vs `atomic.Int32` type | Trivial | Valid | **Patched** |
| 10 | Missing test for unknown event type | Low | Valid | **Patched** |

---

## Patches Applied

### #1 — Backoff reset defeats exponential backoff

**File:** `receiver/tetragonreceiver/receiver.go`

The `b.Reset()` call ran unconditionally after every `runStream` return, before `b.NextBackOff()`. This meant the backoff always restarted from `InitialInterval` — exponential growth never kicked in.

**Fix:** Only reset backoff after a stream that lasted longer than 30 seconds, indicating a real connection was established. Short-lived failures now accumulate backoff correctly (1s → 2s → 4s → … → 30s).

### #3 — Consumer error handling

**File:** `receiver/tetragonreceiver/receiver.go`

Consumer errors were logged at ERROR level but otherwise ignored — no distinction between permanent and transient failures.

**Fix:** Added `consumererror.IsPermanent()` check. Permanent errors log at ERROR ("dropping logs"), transient errors log at WARN. This follows OTel conventions and gives operators better signal.

### #4 — Context propagation during shutdown

**File:** `receiver/tetragonreceiver/receiver.go`

`ConsumeLogs` received the streaming context, which is cancelled on shutdown. This could short-circuit in-flight event processing before the consumer finishes.

**Fix:** Switched to `context.Background()` for `StartLogsOp`/`ConsumeLogs` so already-buffered events can complete gracefully during shutdown. The channel close (after `consumeWg.Wait()`) still bounds the drain.

### #8 — waitForLogs polling pattern

**File:** `receiver/tetragonreceiver/receiver_test.go`

Replaced the custom deadline-based polling loop with `assert.Eventually`, which is more idiomatic and produces better failure messages under CI load.

### #9 — atomic.Int32 type

**File:** `receiver/tetragonreceiver/receiver_test.go`

Replaced function-based `atomic.AddInt32(&callCount, 1)` / `atomic.LoadInt32(&callCount)` with the `atomic.Int32` type (`callCount.Add(1)` / `callCount.Load()`). Clearer intent, available since Go 1.19.

### #10 — Unknown event type test

**File:** `receiver/tetragonreceiver/convert_test.go`

Added `TestConvertEvent_UnknownEventType` covering the `default: return "unknown"` branch in `eventTypeName`, plus nil returns from `extractProcess` and `extractParent`. Confirms the converter handles unrecognized event types without panic.

---

## Skipped Findings

| # | Reason |
|---|--------|
| 2 | `MaxElapsedTime: 0` means "retry forever" in cenkalti/backoff/v5. Intent is correct; a code comment would be nice but isn't worth a patch. |
| 5 | The 5 type-switches are a known Go trade-off with proto oneofs. A table-driven refactor adds complexity without clear win for 10 event types. |
| 6 | `time.Now()` is handled by `plogtest.IgnoreObservedTimestamp()` in golden tests. Injecting a clock adds indirection for no current test need. |
| 7 | `configtls` import for a single struct literal is normal Go — not worth removing. |

---

## Verification

- All tests pass: `go test -race -count=1 ./...` — OK
- `go vet ./...` — clean
