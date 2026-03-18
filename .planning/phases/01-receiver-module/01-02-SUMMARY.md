---
phase: 01-receiver-module
plan: "02"
subsystem: receiver
tags: [go, opentelemetry, tetragon, protojson, converter, golden-files, plog]

# Dependency graph
requires:
  - "01-01 (receiver module scaffold with go.mod and factory)"
provides:
  - "convertEvent() function mapping GetEventsResponse to plog.Logs"
  - "10 proto-schema-faithful JSON fixtures for all Tetragon event types"
  - "10 golden YAML files capturing expected plog.Logs output"
  - "Full golden file test suite with plogtest.CompareLogs validation"
affects:
  - 01-03 (gRPC stream loop will call convertEvent() to produce plog.Logs)
  - 02-distribution (converter determines body format for OpenObserve queries)

# Tech tracking
tech-stack:
  added:
    - "github.com/cilium/tetragon/api v1.6.0"
    - "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/golden v0.148.0"
    - "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest v0.148.0"
    - "go.opentelemetry.io/collector/pdata v1.54.0 (plog, pcommon)"
    - "google.golang.org/protobuf/encoding/protojson"
  patterns:
    - "protojson.MarshalOptions{UseProtoNames: true} for snake_case body — critical for OpenObserve compatibility"
    - "golden.WriteLogsToFile with UPDATE_GOLDEN=true env var guard for safe golden generation"
    - "plogtest.CompareLogs with plogtest.IgnoreObservedTimestamp() for deterministic golden comparison"
    - "Type switch on resp.GetEvent().(type) for all 10 GetEventsResponse variants"

key-files:
  created:
    - "receiver/tetragonreceiver/convert.go"
    - "receiver/tetragonreceiver/convert_test.go"
    - "receiver/tetragonreceiver/testdata/README.md"
    - "receiver/tetragonreceiver/testdata/events/process_exec.json"
    - "receiver/tetragonreceiver/testdata/events/process_exit.json"
    - "receiver/tetragonreceiver/testdata/events/process_kprobe.json"
    - "receiver/tetragonreceiver/testdata/events/process_tracepoint.json"
    - "receiver/tetragonreceiver/testdata/events/process_loader.json"
    - "receiver/tetragonreceiver/testdata/events/process_uprobe.json"
    - "receiver/tetragonreceiver/testdata/events/process_lsm.json"
    - "receiver/tetragonreceiver/testdata/events/process_usdt.json"
    - "receiver/tetragonreceiver/testdata/events/process_throttle.json"
    - "receiver/tetragonreceiver/testdata/events/rate_limit_info.json"
    - "receiver/tetragonreceiver/testdata/golden/process_exec.yaml"
    - "receiver/tetragonreceiver/testdata/golden/process_exit.yaml"
    - "receiver/tetragonreceiver/testdata/golden/process_kprobe.yaml"
    - "receiver/tetragonreceiver/testdata/golden/process_tracepoint.yaml"
    - "receiver/tetragonreceiver/testdata/golden/process_loader.yaml"
    - "receiver/tetragonreceiver/testdata/golden/process_uprobe.yaml"
    - "receiver/tetragonreceiver/testdata/golden/process_lsm.yaml"
    - "receiver/tetragonreceiver/testdata/golden/process_usdt.yaml"
    - "receiver/tetragonreceiver/testdata/golden/process_throttle.yaml"
    - "receiver/tetragonreceiver/testdata/golden/rate_limit_info.yaml"
  modified:
    - "receiver/tetragonreceiver/go.mod"
    - "receiver/tetragonreceiver/go.sum"

key-decisions:
  - "UInt32Value wrapper fields serialize as plain JSON numbers in protojson v2 (not {value: N} as plan documented)"
  - "ProcessUprobe has no GetFunctionName() — uses GetSymbol() instead; fixture uses symbol field"
  - "golden.WriteLogs always fails tests; used golden.WriteLogsToFile with UPDATE_GOLDEN env guard instead"

requirements-completed: [CONV-01, CONV-02, CONV-03, CONV-04, CONV-05, CONV-06, CONV-07, CONV-08, CONV-09, CONV-10]

# Metrics
duration: 30min
completed: 2026-03-18
---

# Phase 1 Plan 02: Event Converter Summary

**convertEvent() converting all 10 Tetragon event types to plog.Logs with protojson body, severity mapping, process/parent/k8s attribute extraction, and golden file validation via pkg/golden + plogtest.CompareLogs**

## Performance

- **Duration:** ~30 min
- **Started:** 2026-03-18T04:10:00Z
- **Completed:** 2026-03-18T04:44:00Z
- **Tasks:** 2 of 2
- **Files modified:** 24

## Accomplishments

- `convertEvent()` maps each `GetEventsResponse` to exactly one `plog.LogRecord` with scope name `tetragonreceiver`
- Body is `protojson.MarshalOptions{UseProtoNames: true}` output — snake_case fields matching Tetragon native JSON for OpenObserve query compatibility
- Severity: INFO for exec/exit/loader, WARN for kprobe/tracepoint/lsm/uprobe/usdt, ERROR for throttle/rate_limit
- Process attributes: binary, arguments, pid, uid, exec_id, cwd; Kubernetes: k8s.namespace.name, k8s.pod.name, k8s.container.name
- Parent attributes: binary, pid, exec_id
- Event-specific: policy_name, action, function_name (kprobe/lsm/uprobe), subsys/event (tracepoint), exit status/signal
- 10 proto-schema-faithful JSON fixtures verified to unmarshal via `TestFixtures_Unmarshal`
- 10 golden YAML files generated and validated by `TestConvertEvent_Golden` with `plogtest.IgnoreObservedTimestamp()`
- All 14 tests pass with race detector (`go test -race ./...`)

## Task Commits

Each task was committed atomically:

1. **Task 1: Create proto-schema-faithful JSON fixtures** - `0da4ed9` (feat)
2. **Task 2: Implement convert.go and golden file tests** - `011e283` (feat)

## Files Created/Modified

- `receiver/tetragonreceiver/convert.go` — convertEvent() with helpers (setSeverity, eventTypeName, extractProcess, extractParent, extractEventAttrs)
- `receiver/tetragonreceiver/convert_test.go` — TestConvertEvent_Golden, TestConvertEvent_BodySnakeCase, TestConvertEvent_ThrottleNoProcess, TestConvertEvent_RateLimitNoProcess, TestFixtures_Unmarshal
- `receiver/tetragonreceiver/testdata/README.md` — Documents fixture provenance and golden regeneration
- `receiver/tetragonreceiver/testdata/events/*.json` — 10 proto-schema-faithful fixtures
- `receiver/tetragonreceiver/testdata/golden/*.yaml` — 10 golden expected plog.Logs YAML files
- `receiver/tetragonreceiver/go.mod` — Added tetragon/api v1.6.0, pkg/golden v0.148.0, pdatatest v0.148.0

## Decisions Made

- **UInt32Value format correction:** Plan documented `{"value": N}` format for proto wrapper types, but `google.golang.org/protobuf` v2 marshals `google.protobuf.UInt32Value` as a plain JSON number. Discovered and fixed during TestFixtures_Unmarshal debugging.
- **ProcessUprobe symbol field:** ProcessUprobe has no `GetFunctionName()` — uses `GetSymbol()` for the probe symbol name. Fixture uses `symbol` field, converter maps to `tetragon.function_name` attribute for query consistency with kprobe/lsm.
- **golden.WriteLogs API:** `golden.WriteLogs` always fails tests with a "must be removed" message (it's a write-time signal). Used `golden.WriteLogsToFile` with `UPDATE_GOLDEN=true` env var guard instead.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] UInt32Value JSON format is plain number, not {"value": N}**
- **Found during:** Task 1 (TestFixtures_Unmarshal)
- **Issue:** Plan documented pid/uid as `{"value": 1234}` format, but protojson v2 encodes `google.protobuf.UInt32Value` as a plain JSON number (`1234`). All fixtures with pid/uid fields failed to unmarshal.
- **Fix:** Updated all 8 fixtures with process pid/uid fields to use plain integer values.
- **Files modified:** All process_*.json fixtures (except throttle and rate_limit_info which have no process)
- **Committed in:** `0da4ed9` (Task 1 commit)

**2. [Rule 1 - Bug] ProcessUprobe has no GetFunctionName() method**
- **Found during:** Task 2 (go build)
- **Issue:** Plan specified `u.GetFunctionName()` for ProcessUprobe event-specific attributes, but ProcessUprobe proto has no `function_name` field — it uses `symbol` for the probe symbol.
- **Fix:** Changed `u.GetFunctionName()` to `u.GetSymbol()` in convert.go; updated process_uprobe.json fixture to use `symbol` field.
- **Files modified:** `convert.go`, `testdata/events/process_uprobe.json`
- **Committed in:** `011e283` (Task 2 commit)

**3. [Rule 1 - Bug] golden.WriteLogs always fails tests**
- **Found during:** Task 2 design (reviewing golden API)
- **Issue:** Plan cited `golden.WriteLogs(t, path, logs)` as "no-op when UPDATE_GOLDEN not set" — but the actual implementation always calls `tb.Fail()` as a test-time signal that the call must be removed.
- **Fix:** Used `golden.WriteLogsToFile(path, logs)` wrapped in `if os.Getenv("UPDATE_GOLDEN") == "true"` guard.
- **Files modified:** `convert_test.go`
- **Committed in:** `011e283` (Task 2 commit)

---

**Total deviations:** 3 auto-fixed (all Rule 1 — incorrect proto/API assumptions in plan)
**Impact on plan:** All fixes necessary for compilation and test correctness. No scope creep.

## Issues Encountered

- Plan's UInt32Value JSON format assumption was based on proto2 or custom JSON behavior, not protojson v2 semantics
- ProcessUprobe is structurally different from kprobe/lsm (symbol not function_name) — proto schema difference
- pkg/golden WriteLogs API changed from a conditional write to an always-fail mechanism in contrib v0.148.0

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `convertEvent()` is ready for Plan 03 (gRPC stream loop) to call
- Function signature: `convertEvent(resp *tetragonv1.GetEventsResponse) plog.Logs`
- Golden files protect against accidental converter changes
- mise tasks operational: `mise run test`, `mise run build`

## Self-Check: PASSED

All files exist on disk:
- `receiver/tetragonreceiver/convert.go` — exists (verified by build passing)
- `receiver/tetragonreceiver/convert_test.go` — exists (verified by tests passing)
- `receiver/tetragonreceiver/testdata/golden/*.yaml` — 10 files (verified by `ls | wc -l`)
- `receiver/tetragonreceiver/testdata/events/*.json` — 10 files (verified by TestFixtures_Unmarshal)

Commits verified in git log:
- `0da4ed9` feat(01-02): add proto-schema-faithful synthetic Tetragon JSON test fixtures
- `011e283` feat(01-02): implement convertEvent() with golden file validation for all 10 event types

---
*Phase: 01-receiver-module*
*Completed: 2026-03-18*
