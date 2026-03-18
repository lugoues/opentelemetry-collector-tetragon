# Phase 1: Review Fixes — Research

**Researched:** 2026-03-18
**Domain:** Go receiver code quality, backoff library semantics, OTel test conventions
**Confidence:** HIGH

## Summary

Milestone 1 code review (`Milestone1-review.md`) identified 17 issues across production receiver code, test code, CI, and project conventions. This research validates each finding against the actual codebase and authoritative sources (module cache, proto definitions) to establish which are genuine bugs, false positives, or enhancements, and identifies the exact fix for each.

The most critical finding is that the reviewer's Issue #1 (backoff library mismatch / `MaxElapsedTime=0` stops immediately) is based on v4 behavior. In `cenkalti/backoff/v5` the `ExponentialBackOff` struct has no `MaxElapsedTime` field at all, and `NextBackOff()` never returns `backoff.Stop`. The current "retry forever" behavior is correct. However, a consequence of this v5 change is that the `if wait == backoff.Stop` branch in `receiver.go:123-128` is dead code that can never execute — it should be removed. Issue #2 (backoff never resets on success) is a genuine production reliability bug.

Issue #11 (ProcessLoader parent extraction) is a false positive: the Tetragon API v1.6.0 `ProcessLoader` proto struct has no `Parent` field, so nothing is silently dropped.

**Primary recommendation:** Fix P0 bugs first (Issues #2, #3), then clean up the dead code from the backoff v5 migration (#1 dead branch), then address P1-P2 test/convention issues in priority order. Skip #11 (false positive) and defer #17.

<phase_requirements>
## Phase Requirements

Derived from code review findings in `Milestone1-review.md`. Prioritized from the reviewer's Priority Summary table.

| ID | Description | Research Support |
|----|-------------|-----------------|
| RFX-01 | Fix backoff dead-code branch — `if wait == backoff.Stop` can never execute in v5 | Verified: v5 `ExponentialBackOff.NextBackOff()` never returns `Stop` |
| RFX-02 | Fix backoff never resets on success — call `b.Reset()` after `runStream` connects | Verified: production reliability bug, backoff accumulates across reconnects |
| RFX-03 | Rate-limit or deduplicate buffer-full warning log | Verified: fires on every `Recv()` under backpressure |
| RFX-04 | Fix `mockGetEventsClient.blockOnce` — use channel/context instead of `time.Sleep(30s)` | Verified: test hangs 30s if context not cancelled fast enough |
| RFX-05 | Fix `mockTetragonClient` — remove `atomic.AddInt32` while mutex is held | Verified: redundant synchronization |
| RFX-06 | Fix `makeExecResponse` — set pid on Process or remove unused parameter | Verified: `pid` parameter silently ignored, `Process.Pid` stays nil |
| RFX-07 | Add CI lint job — run `go vet ./...` in CI (extends current `go test ./...` job) | Verified: `.github/workflows/ci.yml` only runs `go test` |
| RFX-08 | Replace custom `nopHost` with `componenttest.NewNopHost()` | Verified: `componenttest.NewNopHost()` exists in v0.148.0 module cache |
| RFX-09 | Add `componenttest.CheckConfigStruct` test to `config_test.go` | Verified: `CheckConfigStruct` exists in v0.148.0, validates mapstructure tags |
| RFX-10 | Add retry validation in `Config.Validate()` — call `c.Retry.Validate()` | Verified: `configretry.BackOffConfig.Validate()` exists and checks intervals |
| RFX-11 | Add `//go:generate mdatagen metadata.yaml` directive in `doc.go` | Verified: `doc.go` exists, no generate directive present |
| RFX-12 | Add LICENSE file to repo root | Verified: `ls` shows no LICENSE file; README badge references the repo |
| RFX-13 | Document `event.domain` and `event.name` as receiver-specific (non-semconv) attributes | Verified: attributes set in `convert.go:45-46` with no semconv note |
| RFX-14 | Verify container health check works in distroless — document finding | Verified: distroless has no wget/curl; health check uses OTel healthcheck extension on :13133 |
</phase_requirements>

## Issue Analysis

### Confirmed Bugs (P0/P1)

#### RFX-02: Backoff never resets on reconnect success (P0)
**Location:** `receiver/tetragonreceiver/receiver.go:99-141`

The exponential backoff `b` is created once per `streamEvents()` call. After `runStream` successfully connects and then eventually fails (stream EOF, network hiccup), `b.NextBackOff()` returns an interval accumulated from all previous attempts. A stream that ran for hours and then failed would reconnect with `MaxInterval` (30s) instead of `InitialInterval` (1s).

Fix: call `b.Reset()` after `runStream` returns `nil` or returns a non-context-cancellation error (i.e., after a successful connect that later failed). The correct place is after `runStream` returns and before checking `ctx.Err()`:

```go
err := r.runStream(ctx, eventCh)
if err == nil || !errors.Is(err, ctx.Err()) {
    b.Reset() // stream connected; restart backoff from InitialInterval on next retry
}
```

**Source:** Verified against `cenkalti/backoff/v5@v5.0.3/exponential.go` — `Reset()` sets `currentInterval = InitialInterval`.

#### RFX-03: Buffer warning floods under sustained backpressure (P0)
**Location:** `receiver/tetragonreceiver/receiver.go:165-172`

The warning fires on every `Recv()` call while `len(eventCh) >= 800`. At 1000 events/sec this generates 200+ identical log lines per second.

Fix: use a time-based rate limiter. Standard approach in Go: track `lastWarnTime` and emit only if `time.Since(lastWarnTime) > 10*time.Second`. This is a field on `tetragonReceiver` or a local variable in `runStream`.

#### RFX-01: Dead code — `backoff.Stop` check can never trigger in v5 (P1)
**Location:** `receiver/tetragonreceiver/receiver.go:122-128`

The reviewer's concern about `MaxElapsedTime=0` causing immediate stop is based on `cenkalti/backoff/v4` behavior. In v5, `ExponentialBackOff` has no `MaxElapsedTime` field. The struct only has `InitialInterval`, `RandomizationFactor`, `Multiplier`, `MaxInterval`. `NextBackOff()` always returns a positive duration — it never returns `backoff.Stop`. The `if wait == backoff.Stop` branch is dead code from the v4→v5 migration.

**Verified against:** `/home/vscode/go/pkg/mod/github.com/cenkalti/backoff/v5@v5.0.3/exponential.go` — no `MaxElapsedTime` field, `NextBackOff()` always computes a positive interval.

Fix: remove the dead `if wait == backoff.Stop` block entirely (lines 122-128). The `r.logger.Error("max backoff elapsed, stopping stream")` log line will never appear in production anyway.

Note: `configretry.BackOffConfig.MaxElapsedTime` field exists and `MaxElapsedTime: 0` is documented as "retries never stopped" — but this field is on `configretry`, not on `cenkalti/backoff/v5`. The code never passes `MaxElapsedTime` to the cenkalti struct, which is correct for v5 (v5 doesn't have the field).

### Confirmed Test Issues (P2)

#### RFX-04: `time.Sleep(30s)` in blocking mock
**Location:** `receiver/tetragonreceiver/receiver_test.go:49-52`

```go
if m.blockOnce {
    time.Sleep(30 * time.Second)
}
```

`TestReceiverStartShutdown` uses this mock with `blockOnce: true`. If `Shutdown()` is called, the goroutine stays sleeping for 30s until the sleep expires, then returns `io.EOF` — but by then the test's 5s timeout has either passed or the WaitGroup is stuck. Confirmed it passes currently because Shutdown cancels the stream context which unblocks `stream.Recv()` → the goroutine is in `runStream`'s `stream.Recv()` → context cancellation returns immediately. The sleep is in `Recv()` before returning, though, so it does block.

Fix: replace `time.Sleep(30 * time.Second)` with `<-ctx.Done(); return nil, ctx.Err()` — the mock needs a `blockCtx context.Context` field set at construction time.

#### RFX-05: Mutex+atomic redundancy in `mockTetragonClient`
**Location:** `receiver/tetragonreceiver/receiver_test.go:76-77`

```go
m.mu.Lock()
defer m.mu.Unlock()
atomic.AddInt32(&m.callCount, 1)  // mutex already held; atomic is redundant
```

Fix: change `callCount` to `int` (not `int32`) and use `m.callCount++` inside the lock. Remove `atomic` import if unused elsewhere.

#### RFX-06: `makeExecResponse` ignores `pid` parameter
**Location:** `receiver/tetragonreceiver/receiver_test.go:270-280`

```go
func makeExecResponse(binary string, pid uint32) *tetragonv1.GetEventsResponse {
    // pid parameter is never used — Process.Pid stays nil
    Process: &tetragonv1.Process{Binary: binary},
```

Fix: either set `Pid: wrapperspb.UInt32(pid)` on the Process, or drop the parameter and update all call sites to not pass `pid`. Since the tests don't assert on pid values, dropping the parameter is cleaner.

### False Positive

#### Issue #11: ProcessLoader parent extraction — FALSE POSITIVE
**Location:** `receiver/tetragonreceiver/convert.go:157-177`

The reviewer states ProcessLoader events may have a parent field that is silently dropped. Verified against the actual proto definition:

```
type ProcessLoader struct {
    Process  *Process
    Path     string
    Buildid  []byte
    // NO Parent field
}
```

**Source:** `/home/vscode/go/pkg/mod/github.com/cilium/tetragon/api@v1.6.0/v1/tetragon/tetragon.pb.go` — `ProcessLoader` has `Process`, `Path`, `Buildid` only. No `GetParent()` method. Nothing to fix.

### Convention/Project Issues (P2-P3)

#### RFX-08: Custom `nopHost` vs `componenttest.NewNopHost()`
`componenttest.NewNopHost()` exists in `go.opentelemetry.io/collector/component/componenttest@v0.148.0`. It implements `component.Host` with empty-map `GetExtensions()`. The local `nopHost` returns `nil` for `GetExtensions()` while the OTel version returns an empty (non-nil) map — which matters if extension lookup code assumes non-nil map.

Fix: replace `newNopHost()` calls with `componenttest.NewNopHost()`, remove local `nopHost` struct and `newNopHost()` function.

Note: `componenttest` is already in `go.mod` as an indirect dependency (`go.opentelemetry.io/collector/component/componenttest v0.148.0 // indirect`). It needs to be promoted to a direct test dependency.

#### RFX-09: `componenttest.CheckConfigStruct`
`CheckConfigStruct(config any) error` in `componenttest@v0.148.0/configtest.go` validates that all public struct fields have `mapstructure` tags. Call it in `TestCreateDefaultConfig` or as a standalone test:

```go
func TestConfigStruct(t *testing.T) {
    require.NoError(t, componenttest.CheckConfigStruct(createDefaultConfig()))
}
```

#### RFX-10: Add `c.Retry.Validate()` call
`configretry.BackOffConfig.Validate()` (verified in module cache) checks:
- `InitialInterval >= 0`
- `RandomizationFactor in [0, 1]`
- `Multiplier >= 0`
- `MaxInterval >= 0`
- `MaxElapsedTime >= 0` and `>= InitialInterval` and `>= MaxInterval` if non-zero

`Config.Validate()` currently only checks `Endpoint != ""` and delegates TLS via `c.ClientConfig.Validate()`. Add `c.Retry.Validate()`:

```go
func (c *Config) Validate() error {
    if c.Endpoint == "" {
        return errors.New("endpoint is required")
    }
    if err := c.ClientConfig.Validate(); err != nil {
        return err
    }
    return c.Retry.Validate()
}
```

#### RFX-07: Add CI lint job
Current `.github/workflows/ci.yml` `test` job only runs `go test ./...`. Add a `lint` job (or extend `test`) with `go vet ./...`:

```yaml
- run: go vet ./...
```

Ideally add `golangci-lint` as a separate job. The `mise run lint` task already runs `go vet` locally.

#### RFX-11: Add `//go:generate` directive
`doc.go` exists but has no `//go:generate` comment. Convention for OTel receivers:

```go
//go:generate mdatagen metadata.yaml
```

This is a P3 developer workflow enhancement only — `mdatagen` is not in `go.mod` and generating component tests requires extending `metadata.yaml`.

#### RFX-12: Add LICENSE file
The `README.md` includes a CI badge and references the GitHub repository `github.com/cilium/otelcol-tetragon`, but no LICENSE file is present in the repo root. The Cilium project uses Apache 2.0. Add `LICENSE` with Apache-2.0 text.

#### RFX-13: Document receiver-specific attributes
`event.domain` and `event.name` are set in `convert.go` but are not OTel semantic conventions. This is a documentation fix (comment in `convert.go` or README section), not a code change.

#### RFX-14: Container health check — distroless verification
`container/Containerfile` exposes port 13133 and the config enables the `health_check` extension. Distroless images have no shell, wget, or curl. The `HEALTHCHECK` Dockerfile instruction cannot work in distroless. The current `Containerfile` has no `HEALTHCHECK` instruction (verified) — port 13133 exposure is for Kubernetes readiness/liveness probes to hit directly. This is acceptable. The smoke test in `.mise/config.toml` uses `curl` from the host, not inside the container — it works. No code change needed; add a comment to `Containerfile` clarifying this.

## Standard Stack

### Core (already in use)
| Library | Version | Purpose | Role in fixes |
|---------|---------|---------|---------------|
| `github.com/cenkalti/backoff/v5` | v5.0.3 | Exponential backoff | Dead-code removal (RFX-01), reset fix (RFX-02) |
| `go.opentelemetry.io/collector/component/componenttest` | v0.148.0 | OTel test helpers | `NewNopHost()` (RFX-08), `CheckConfigStruct` (RFX-09) |
| `go.opentelemetry.io/collector/config/configretry` | v1.54.0 | Retry config + validation | `Validate()` call in `Config.Validate()` (RFX-10) |
| `google.golang.org/protobuf/types/known/wrapperspb` | (via tetragon dep) | Proto wrappers | For `makeExecResponse` pid fix (RFX-06) |

All dependencies are already in `go.mod`. `componenttest` needs to be promoted from `// indirect` to direct.

### No New Dependencies Required

All fixes use existing dependencies already present in `go.mod`. No new imports at module level.

## Architecture Patterns

### Rate-limiting the buffer warning (RFX-03)

The warning rate-limiting pattern for `runStream` — add a field to the receiver or use a local variable:

```go
// Option A: local variable in runStream (simpler, no receiver field needed)
var lastBufferWarn time.Time
const bufferWarnInterval = 10 * time.Second

// In the Recv loop:
bufLen := len(eventCh)
if bufLen >= int(float64(bufferSize)*bufferWarnPct) {
    if time.Since(lastBufferWarn) >= bufferWarnInterval {
        r.logger.Warn("event buffer nearing capacity",
            zap.Int("buffer_len", bufLen),
            zap.Int("buffer_cap", bufferSize))
        lastBufferWarn = time.Now()
    }
}
```

This is local to `runStream` and doesn't require a struct field or sync primitives.

### Backoff reset placement (RFX-02)

```go
// In streamEvents(), after runStream returns:
err := r.runStream(ctx, eventCh)
if ctx.Err() != nil {
    // Clean shutdown — do not reconnect.
    close(eventCh)
    consumeWg.Wait()
    return
}

// Transient error — reset backoff (stream had connected) and schedule retry.
b.Reset()
componentstatus.ReportStatus(r.host,
    componentstatus.NewRecoverableErrorEvent(err))

wait := b.NextBackOff()
// ... rest of reconnect logic
```

`b.Reset()` before `b.NextBackOff()` ensures the next retry starts at `InitialInterval` regardless of how many previous retries occurred.

### Context-based blocking mock (RFX-04)

```go
type mockGetEventsClient struct {
    mu        sync.Mutex
    responses []*tetragonv1.GetEventsResponse
    idx       int
    err       error
    blockCtx  context.Context // non-nil means block until Done
}

func (m *mockGetEventsClient) Recv() (*tetragonv1.GetEventsResponse, error) {
    m.mu.Lock()
    // ... existing response/error handling ...
    m.mu.Unlock()

    if m.blockCtx != nil {
        <-m.blockCtx.Done()
        return nil, m.blockCtx.Err()
    }
    return nil, io.EOF
}
```

`TestReceiverStartShutdown` creates a `context.WithCancel` and passes it as `blockCtx`. Shutdown cancels it, unblocking `Recv()` immediately.

### `componenttest.NewNopHost()` — returns non-nil map (RFX-08)

The official `nopHost.GetExtensions()` returns `map[component.ID]component.Component{}` (non-nil empty map). The local `nopHost` returns `nil`. `configgrpc.ClientConfig.ToClientConn()` calls `host.GetExtensions()` to look up auth extensions. With a nil map, map iteration is safe in Go, but explicit nil checks in the OTel SDK may differ. Using the official helper avoids this subtlety.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Retry config validation | Custom interval checks | `configretry.BackOffConfig.Validate()` | Already validates all fields including cross-field constraints |
| Config struct tag validation | Custom reflection | `componenttest.CheckConfigStruct()` | OTel standard, maintained with SDK |
| Test host mock | Custom `nopHost` struct | `componenttest.NewNopHost()` | Tracks interface changes automatically |

## Common Pitfalls

### Pitfall 1: Removing `backoff.Stop` check without understanding the v5 API
When removing the `if wait == backoff.Stop` dead branch (RFX-01), confirm that no other backoff types are used that DO return `Stop`. In this codebase only `backoff.NewExponentialBackOff()` is used — safe to remove. If `backoff.StopBackOff` were ever used, the check would be needed.

### Pitfall 2: Backoff reset placement breaks clean shutdown detection
Calling `b.Reset()` must happen AFTER the `ctx.Err()` clean-shutdown check. Resetting before the check is harmless but structurally confusing. The correct ordering: `runStream` returns → check `ctx.Err()` → if shutdown, exit → else `b.Reset()` → report error → sleep.

### Pitfall 3: `componenttest` is an indirect dependency
`go.opentelemetry.io/collector/component/componenttest v0.148.0` is in `go.mod` as `// indirect`. Adding a direct import in `receiver_test.go` will require `go mod tidy` to promote it to direct. Run `go mod tidy` after making the import.

### Pitfall 4: `makeExecResponse` pid fix — `wrapperspb.UInt32` wrapper
`Process.Pid` is `*wrapperspb.UInt32Value`, not a plain `uint32`. Setting it requires:
```go
import "google.golang.org/protobuf/types/known/wrapperspb"
Pid: wrapperspb.UInt32(pid),
```
`wrapperspb` is already a transitive dependency via tetragon/api.

### Pitfall 5: Issue #11 (ProcessLoader parent) is a false positive — don't add code
Do NOT add `ProcessLoader` to `extractParent()`. The Tetragon v1.6.0 proto for `ProcessLoader` has no `Parent` field. Adding `e.ProcessLoader.GetParent()` would not compile.

## Code Examples

### Verified: backoff/v5 ExponentialBackOff never returns Stop
```go
// Source: /home/vscode/go/pkg/mod/github.com/cenkalti/backoff/v5@v5.0.3/exponential.go
func (b *ExponentialBackOff) NextBackOff() time.Duration {
    if b.currentInterval == 0 {
        b.currentInterval = b.InitialInterval
    }
    next := getRandomValueFromInterval(b.RandomizationFactor, rand.Float64(), b.currentInterval)
    b.incrementCurrentInterval()
    return next  // always positive; never returns backoff.Stop
}
// No MaxElapsedTime field on ExponentialBackOff in v5.
// backoff.Stop is only returned by StopBackOff.NextBackOff().
```

### Verified: componenttest.NewNopHost signature
```go
// Source: /home/vscode/go/pkg/mod/go.opentelemetry.io/collector/component/componenttest@v0.148.0/nop_host.go
func NewNopHost() component.Host {
    return &nopHost{}
}
func (nh *nopHost) GetExtensions() map[component.ID]component.Component {
    return map[component.ID]component.Component{}  // non-nil empty map
}
```

### Verified: configretry.BackOffConfig.Validate()
```go
// Source: /home/vscode/go/pkg/mod/go.opentelemetry.io/collector/config/configretry@v1.54.0/backoff.go
func (bs *BackOffConfig) Validate() error {
    if !bs.Enabled { return nil }
    // validates InitialInterval >= 0, RandomizationFactor in [0,1],
    // Multiplier >= 0, MaxInterval >= 0, MaxElapsedTime >= 0,
    // and cross-field: MaxElapsedTime >= InitialInterval and MaxInterval
    ...
}
```

### Verified: ProcessLoader has no Parent field
```go
// Source: /home/vscode/go/pkg/mod/github.com/cilium/tetragon/api@v1.6.0/v1/tetragon/tetragon.pb.go
type ProcessLoader struct {
    Process  *Process  // field 1
    Path     string    // field 2
    Buildid  []byte    // field 3
    // No Parent field — Issue #11 is a false positive
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `backoff/v4` with `MaxElapsedTime` | `backoff/v5` without `MaxElapsedTime` on `ExponentialBackOff` | v5.0.0 released 2024-12-19 | Dead-code `backoff.Stop` check in receiver.go |
| Custom `nopHost` | `componenttest.NewNopHost()` | Always available in OTel SDK | Test forward-compatibility |

## Open Questions

1. **Should `mdatagen`-generated tests be added (Issue #7)?**
   - What we know: `metadata.yaml` is minimal (no metrics/attributes), `mdatagen` generates `generated_component_test.go` with lifecycle tests
   - What's unclear: Whether adding `mdatagen` as a dev tool via `mise` is desired for this project given it's a standalone receiver module
   - Recommendation: Add `//go:generate` directive (RFX-11, P3) but defer running `mdatagen` to a future enhancement phase — it requires extending `metadata.yaml` meaningfully

2. **golangci-lint in CI vs just `go vet`?**
   - What we know: reviewer recommends `golangci-lint` as "ideally"
   - What's unclear: golangci-lint version compatibility with Go 1.25 and OTel v0.148.0/v1.54.0 mix
   - Recommendation: Start with `go vet ./...` in CI (RFX-07, confirmed works), defer golangci-lint to a future phase

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + testify v1.11.1 |
| Config file | none — standard `go test` |
| Quick run command | `cd receiver/tetragonreceiver && go test -race ./...` |
| Full suite command | `cd receiver/tetragonreceiver && go test -race -count=1 ./...` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RFX-01 | Dead code removed, `backoff.Stop` branch gone | code review | n/a — compile + vet | existing |
| RFX-02 | Backoff resets after successful stream | unit | `go test -race -run TestReceiverReconnectsOnStreamError ./...` | existing (extend) |
| RFX-03 | Buffer warning rate-limited | unit | `go test -race -run TestReceiver ./...` | existing |
| RFX-04 | Mock blocks on context, not sleep | unit | `go test -race -run TestReceiverStartShutdown ./...` | existing |
| RFX-05 | Mutex-only in mock | code review | `go vet ./...` | existing |
| RFX-06 | pid set or parameter removed | code review | `go test -race -run TestReceiverConsumesLogs ./...` | existing |
| RFX-07 | CI lint job present | config review | CI run | ❌ Wave 0 |
| RFX-08 | `componenttest.NewNopHost()` used | compile | `go build ./...` | existing |
| RFX-09 | `CheckConfigStruct` test passes | unit | `go test -race -run TestConfigStruct ./...` | ❌ Wave 0 |
| RFX-10 | `Config.Validate()` calls `Retry.Validate()` | unit | `go test -race -run TestConfigValidate ./...` | existing (extend) |
| RFX-11 | `//go:generate` in doc.go | code review | n/a | existing |
| RFX-12 | LICENSE file exists | existence check | `ls LICENSE` | ❌ Wave 0 |
| RFX-13 | Attribute docs updated | review | n/a | existing |
| RFX-14 | Containerfile comment added | review | n/a | existing |

### Sampling Rate
- **Per task commit:** `cd receiver/tetragonreceiver && go test -race ./...`
- **Per wave merge:** `cd receiver/tetragonreceiver && go test -race -count=1 ./... && go vet ./...`
- **Phase gate:** Full suite green + new CI lint job green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `receiver/tetragonreceiver/config_test.go` — add `TestConfigStruct` test (RFX-09)
- [ ] `LICENSE` — Apache-2.0 file in repo root (RFX-12)
- [ ] `.github/workflows/ci.yml` — add `go vet ./...` step (RFX-07)

## Sources

### Primary (HIGH confidence)
- `/home/vscode/go/pkg/mod/github.com/cenkalti/backoff/v5@v5.0.3/exponential.go` — v5 `ExponentialBackOff` struct, `NextBackOff()`, no `MaxElapsedTime` field, never returns `Stop`
- `/home/vscode/go/pkg/mod/github.com/cenkalti/backoff/v5@v5.0.3/backoff.go` — `Stop` constant definition, only returned by `StopBackOff`
- `/home/vscode/go/pkg/mod/go.opentelemetry.io/collector/component/componenttest@v0.148.0/nop_host.go` — `NewNopHost()` API
- `/home/vscode/go/pkg/mod/go.opentelemetry.io/collector/component/componenttest@v0.148.0/configtest.go` — `CheckConfigStruct()` API
- `/home/vscode/go/pkg/mod/go.opentelemetry.io/collector/config/configretry@v1.54.0/backoff.go` — `BackOffConfig.Validate()` API and semantics (`MaxElapsedTime: 0` = unlimited)
- `/home/vscode/go/pkg/mod/github.com/cilium/tetragon/api@v1.6.0/v1/tetragon/tetragon.pb.go` — `ProcessLoader` struct has no `Parent` field (Issue #11 false positive)
- `/workspaces/otel-collector-tetragon/Milestone1-review.md` — authoritative review findings with priority table
- `/workspaces/otel-collector-tetragon/receiver/tetragonreceiver/receiver.go` — current production code
- `/workspaces/otel-collector-tetragon/receiver/tetragonreceiver/receiver_test.go` — current test code

## Metadata

**Confidence breakdown:**
- Issue analysis: HIGH — verified against module cache source files
- Fix patterns: HIGH — standard Go/OTel patterns from module source
- False positive (Issue #11): HIGH — verified against proto definition

**Research date:** 2026-03-18
**Valid until:** 2026-04-17 (stable ecosystem, 30-day window)
