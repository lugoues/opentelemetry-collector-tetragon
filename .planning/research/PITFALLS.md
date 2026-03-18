# Pitfalls Research

**Domain:** Custom OTel Collector receiver — gRPC streaming, OCB distribution build, Tetragon integration
**Researched:** 2026-03-18
**Confidence:** HIGH (most pitfalls verified via official OTel docs, grpc-go issue tracker, and confirmed OCB build issues)

---

## Critical Pitfalls

### Pitfall 1: Reusing the Start() context in a long-running gRPC stream goroutine

**What goes wrong:**
The `Start(ctx context.Context, host component.Host)` context is scoped to the startup operation, not the receiver's lifetime. If the streaming loop uses this context directly, the gRPC `Recv()` call will be cancelled as soon as the startup phase completes — effectively immediately. The receiver appears to start successfully but silently stops receiving events.

**Why it happens:**
The OTel Collector SDK documentation explicitly warns against this, but the method signature makes it natural to pass `ctx` into the goroutine. Developers coming from HTTP handler patterns assume the context represents "while the component is alive."

**How to avoid:**
In `Start()`, create a new background context and store its cancel function on the receiver struct:
```go
r.ctx, r.cancel = context.WithCancel(context.Background())
```
Launch the stream loop goroutine using `r.ctx`. In `Shutdown()`, call `r.cancel()` to terminate the loop. Never pass the `Start()` ctx into any goroutine that outlives the method call.

**Warning signs:**
- `GetEvents` stream connects successfully but zero log records arrive in OpenObserve
- Receiver logs show "stream started" immediately followed by context-related errors
- The issue reproduces consistently on first start, not intermittently

**Phase to address:** Receiver implementation (Phase 1/core receiver scaffold)

---

### Pitfall 2: OCB otelcol_version misalignment causing silent or cryptic build failures

**What goes wrong:**
The `dist.otelcol_version` field in `builder-config.yaml` must match the major+minor version that Go's module graph resolves from the declared components. When any component's transitive dependency pulls in a newer collector core version than `otelcol_version` declares, OCB either emits a warning and continues (producing a broken binary) or fails with an opaque error like "mismatch in go.mod and builder configuration versions."

**Why it happens:**
The OTel Collector releases on a two-week cadence. contrib components and core collector are versioned independently. Specifying `journaldreceiver v0.120.0` and `filelogreceiver v0.120.0` appears consistent, but if either has a transitive dep on `v0.121.0` of a core package, the entire build is poisoned. The `--skip-strict-versioning` workaround exists but is explicitly marked for removal.

**How to avoid:**
- Pin every component in `builder-config.yaml` to the same target version (e.g. all at `v0.120.0`)
- Run `ocb --config builder-config.yaml` locally and inspect the generated `go.mod` — the resolved versions must align
- Never use `--skip-strict-versioning` except as a last resort with a tracking issue to remove it
- When upgrading, bump ALL components together in a single commit, not piecemeal

**Warning signs:**
- OCB prints "You're building a distribution with non-aligned version of the builder"
- `go build` in the generated output dir fails with version constraint errors
- Generated `go.sum` is missing entries for collector core packages

**Phase to address:** OCB distribution setup (Phase 2/OCB build scaffold)

---

### Pitfall 3: OCB-generated go.mod with unpinned Go toolchain version failing in CI

**What goes wrong:**
OCB generates a `go.mod` that specifies `go 1.22` (minor only, no patch). Go 1.21+ introduced toolchain management: when the minor-only string `go1.22` is used, the Go toolchain attempts to download a generic `go1.22` toolchain, which does not exist as a downloadable artifact. CI environments fail with: `toolchain not available`.

**Why it happens:**
This is a documented OCB bug (issue #11844). The template was written before Go's toolchain directive semantics changed. Local dev machines with `go1.22.x` already installed never hit the download path and don't see the failure.

**How to avoid:**
After OCB generates its output, add a post-generation step that rewrites the `go` directive in the generated `go.mod` to include the patch version (e.g. `go 1.22.0`). Better: pin the `GOVERSION` in the Containerfile to a specific patch release and ensure mise's Go version matches.

**Warning signs:**
- CI fails at `go mod tidy` inside the OCB output directory with "toolchain not available"
- Works fine locally on developer machines
- Only reproduces in clean CI environments without a cached Go toolchain

**Phase to address:** CI pipeline setup (Phase 3/CI scaffold)

---

### Pitfall 4: gRPC stream error discrimination — treating all errors as reconnect triggers

**What goes wrong:**
`stream.Recv()` returns multiple error types that require different handling:
- `io.EOF` — server closed stream gracefully (reconnect is appropriate)
- `codes.Canceled` — the local context was cancelled (do NOT reconnect — initiates a reconnect storm during shutdown)
- `codes.Unavailable` — Tetragon is not reachable (reconnect with backoff)
- Race: approximately 50% of the time a context cancellation manifests as `codes.Canceled`, the other 50% as `io.EOF`

If the shutdown path cancels `r.ctx` and the reconnect loop treats `codes.Canceled` as a retriable error, the goroutine reconnects immediately and loops forever until forcibly killed. This prevents clean shutdown and can leave dangling gRPC connections.

**How to avoid:**
Check `r.ctx.Err()` before deciding to reconnect:
```go
if err := r.ctx.Err(); err != nil {
    return // context was cancelled — shutdown path, stop reconnecting
}
```
Then discriminate the gRPC status code:
```go
st, _ := status.FromError(err)
switch st.Code() {
case codes.Canceled, codes.OK:
    // deliberate close, check ctx above
case codes.Unavailable, codes.DeadlineExceeded:
    // retriable — apply exponential backoff
default:
    // log and apply backoff
}
```

**Warning signs:**
- Collector takes unusually long to shut down (30+ seconds)
- Logs show repeated "connecting to Tetragon" messages during shutdown sequence
- `Shutdown()` call blocks until the 5s deadline is hit

**Phase to address:** Receiver implementation — reconnection logic (Phase 1)

---

### Pitfall 5: Protobuf dependency conflict between Tetragon API and OTel Collector SDK

**What goes wrong:**
`github.com/cilium/tetragon/api` depends on `google.golang.org/protobuf` and `google.golang.org/grpc`. The OTel Collector SDK and its contrib components also depend on these packages, but potentially at different versions. Starting from `google.golang.org/protobuf v1.26.0`, the runtime enforces a hard error when multiple conflicting protobuf names are linked into the same binary — the program panics at startup with a message about duplicate proto registration.

**Why it happens:**
Both `github.com/golang/protobuf` (deprecated) and `google.golang.org/protobuf` (current) can coexist in the module graph via replace/compatibility shims. Tetragon 2024 updates moved to `google.golang.org/protobuf v1.34.1`, but transitive deps in contrib components may still pull the legacy module. The bridging wrapper handles most cases, but name collisions can still trigger panics.

**How to avoid:**
- In the receiver's `go.mod`, explicitly require `google.golang.org/protobuf` at the same version or newer than what Tetragon requires
- Run `go mod graph | grep golang/protobuf` after initial `go get` to identify split-dependency situations
- Do NOT use `replace` to downgrade `google.golang.org/protobuf` — this un-fixes the deduplication
- Use `GOTRACEBACK=all` on startup panics to identify the conflicting registration site

**Warning signs:**
- Binary panics at startup with "proto: duplicate proto type registered" or "panic: proto: file ... is already registered"
- `go mod graph` shows both `github.com/golang/protobuf` and `google.golang.org/protobuf` at multiple versions
- Build succeeds but runtime fails immediately

**Phase to address:** Receiver implementation — initial module setup (Phase 1)

---

### Pitfall 6: Shutdown() called before Start() — receiver panics on nil state

**What goes wrong:**
The OTel Collector framework guarantees that `Shutdown()` may be called without `Start()` ever having been called — this happens when another component fails to start and the collector aborts initialization, calling Shutdown on all registered components for cleanup. If `Shutdown()` dereferences a cancel function or channel that was only initialized in `Start()`, the receiver panics.

**Why it happens:**
The component specification (issue #9682) requires this defensiveness. Auto-generated lifecycle tests in the collector SDK's `receivertest` package explicitly test this scenario, but many developers skip running the full test suite.

**How to avoid:**
Initialize `r.cancel` to a no-op function and `r.doneCh` to a closed channel in the factory's `createLogsReceiver` function (not in `Start()`), so `Shutdown()` is always safe to call:
```go
r := &tetragonReceiver{
    cancel: func() {}, // safe no-op
    ...
}
```
Guard with `if r.cancel != nil` in `Shutdown()` as additional defense.

**Warning signs:**
- `receivertest.ComponentFactory` lifecycle tests fail or are not present
- Panic traces originating from `Shutdown()` mentioning nil pointer
- Only reproducible when the collector fails to fully initialize (e.g. bad config)

**Phase to address:** Receiver implementation — factory and lifecycle (Phase 1)

---

### Pitfall 7: nextConsumer.ConsumeLogs() blocking the stream receive loop

**What goes wrong:**
`nextConsumer.ConsumeLogs()` propagates backpressure from the pipeline. If the downstream exporter (OpenObserve via OTLP/HTTP) is slow or unavailable, `ConsumeLogs()` blocks. While blocked, `stream.Recv()` is not being called. The Tetragon server accumulates events in its send buffer; once that fills, the gRPC flow control window closes and Tetragon's event dispatch stalls. If Tetragon's internal buffer then fills, events are dropped server-side — silently.

**Why it happens:**
This is the intended OTel backpressure mechanism working as designed, but it surprises receiver authors who think they're just reading from a stream. The receiver is not a passive reader — it is the head of a pipeline, and its Recv() rate is coupled to the pipeline's export rate.

**How to avoid:**
- Always wrap `ConsumeLogs()` in the receive loop with a select on `r.ctx.Done()` so shutdown can interrupt a blocked consumer
- Consider decoupling with an internal buffered channel (capacity ~100 log records) and a separate consumer goroutine — this prevents flow-control stall on the gRPC side at the cost of potential in-memory loss on crash
- Include `memory_limiter` processor in the recommended pipeline config to prevent OOM when buffer fills

**Warning signs:**
- Events arrive at Tetragon but stop appearing in OpenObserve during exporter outages
- Receiver logs show no errors but `stream.Recv()` call count drops to zero
- CPU drops to near-zero during what should be active event ingestion

**Phase to address:** Receiver implementation + pipeline config (Phase 1, verified in Phase 3)

---

### Pitfall 8: protojson.Marshal produces different output than expected for Tetragon events

**What goes wrong:**
`protojson.Marshal` with default options uses field names from the proto descriptor (which may differ from Tetragon's own JSON export). Specifically:
- Enum values are marshalled as string names by default in protojson (e.g. `"PROCESS_EXEC"`) but OTLP/JSON requires integer values for OTLP-native fields. For the log body (which is freeform JSON), string enums are fine — but the format must exactly match what Tetragon's `tetra getevents --output json` produces.
- Fields with zero/default values are omitted by default in `protojson` — Tetragon's own export may include them
- `protojson.MarshalOptions{EmitUnpopulated: true}` changes the output significantly

**Why it happens:**
The project constraint is "JSON body must match Tetragon's own JSON format for query compatibility." Tetragon's own JSON export uses `protojson` but with specific `MarshalOptions`. Getting the options wrong produces JSON that existing OpenObserve queries break on (field names shift, values differ, nested fields vanish).

**How to avoid:**
- Before writing any marshaling code, capture reference JSON output from `tetra getevents --output json` for several event types
- Write a table-driven test that marshals the same proto message and asserts byte-level JSON equality against the reference
- Use `protojson.MarshalOptions{UseProtoNames: true}` to match proto field names (not camelCase JSON names) if that matches Tetragon's output
- Pin the exact `MarshalOptions` in a package-level var, not inline

**Warning signs:**
- OpenObserve queries that worked against filelog-sourced events return zero results after switching to the gRPC receiver
- JSON diff between reference output and marshaled output shows camelCase vs snake_case field names
- Nested message fields appear/disappear depending on whether they hold default values

**Phase to address:** Event mapping implementation (Phase 1 — event-to-log-record mapping)

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| `--skip-strict-versioning` in OCB | Unblocks build when version mismatch | Masks real misalignment; removed in future OCB versions; can produce broken binary | Never in CI; only in throwaway local debug session |
| Hardcode Tetragon address as `localhost:54321` | Simpler code | Breaks any non-localhost deployment; prevents TLS testing | Never — put in config from day one |
| Single goroutine for receive + consume | Less code | Blocks receive when consumer is slow; causes flow-control stall | Never for a production receiver |
| Skip `consumertest` / `receivertest` unit tests | Faster initial dev | Lifecycle bugs surface in integration, not unit tests | Never — these tests are cheap to write |
| Copy Tetragon proto files locally instead of depending on `cilium/tetragon/api` | Avoids module dependency complexity | Gets out of sync; misses upstream proto changes; breaks on Tetragon upgrades | Never — import the canonical module |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Tetragon gRPC `FineGuidanceSensors.GetEvents` | Calling `GetEvents` with an empty `GetEventsRequest` and assuming all event types are returned | Confirm server behavior — the stream returns all events by default but `allow_list`/`deny_list` fields exist; test with a live Tetragon instance, not just mock |
| Tetragon gRPC connection | Assuming `grpc.Dial` (deprecated) or `grpc.NewClient` returns error if Tetragon is down | Both are non-blocking by default; errors only appear when the first RPC is made; detect connection state via `conn.GetState()` or treat initial `GetEvents` error as the "is Tetragon available?" check |
| OCB + local receiver module | Using an absolute path in the `replaces` directive | Use a path relative to the `builder-config.yaml` location; absolute paths break in CI where workspace paths differ |
| GHCR multi-arch push | Pushing individual arch images and expecting a manifest list automatically | Must use `docker buildx build --push --platform linux/amd64,linux/arm64` or `podman manifest` workflow explicitly; single-arch push to a tag silently replaces any existing manifest |
| OpenObserve OTLP/HTTP endpoint | Using wrong content-type or path — `/v1/logs` vs `/api/default/v1/logs` | Verify the exact endpoint path OpenObserve expects; it differs from the OTLP spec default path |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Allocating a new `plog.LogRecord` per event without batching | High GC pressure visible in Go pprof; CPU spikes every few seconds | Use `plog.NewLogs()` + `ResourceLogs` + `ScopeLogs` and batch events received in a tight loop window | At high Tetragon event rates (>1000 events/sec) |
| Unmarshaling the full proto `GetEventsResponse` to extract just the event type for routing | Extra CPU for unpacking nested oneof fields that aren't used | Unmarshal once, pass the full struct; do attribute extraction in one pass | Constant overhead; noticeable at >500 events/sec |
| Logging every received event at `DEBUG` level in the receive loop | Log output saturates disk or logging infrastructure; receiver spends more time logging than processing | Use `zap.Logger.Check(zap.DebugLevel, ...)` to skip allocation when debug is disabled | Immediately when debug logging is enabled in production |
| `ConsumeLogs()` called synchronously in receive loop without internal buffer | Backpressure from exporter stalls gRPC flow control; Tetragon drops server-side | Decouple with internal channel | When OpenObserve has any latency spike (>200ms response time) |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Defaulting to insecure gRPC (`grpc.WithInsecure()` / `grpc.WithTransportCredentials(insecure.NewCredentials())`) without documenting the assumption | Acceptable for localhost, but config option is never added; future deployments use insecure over network | Add `tls` config block from day one even if `insecure: true` is the default; makes the option visible |
| Passing Tetragon's raw event data (including process arguments, environment variables) directly into log body without scrubbing | Secrets in env vars (e.g. `AWS_SECRET_ACCESS_KEY`) flow into OpenObserve and any downstream storage | Document that the receiver does not scrub; note that OTel `redaction` processor should be added to the pipeline for sensitive environments |
| Including full file paths and binary names from kprobe/uprobe events in unprotected attributes | Reveals internal service topology to anyone with OpenObserve read access | Access control is OpenObserve's responsibility, but document the sensitivity of the data this receiver emits |

---

## "Looks Done But Isn't" Checklist

- [ ] **Reconnection:** Reconnect loop exists but does not implement exponential backoff with jitter — verify the sleep duration increases between attempts and includes `rand.Int63n` jitter
- [ ] **Shutdown:** `Shutdown()` returns before the stream goroutine has actually stopped — verify `Shutdown()` waits on a `doneCh` or `WaitGroup` before returning
- [ ] **All event types:** Only `ProcessExec` and `ProcessExit` events are mapped — verify `ProcessKprobe`, `ProcessTracepoint`, `ProcessLoader`, `ProcessUprobe`, `ProcessLsm`, `ProcessUsdt`, `RateLimitInfo`, and `ThrottleType` are all handled (even if as a fallback)
- [ ] **protojson options:** `protojson.Marshal` called with default options — verify options exactly match Tetragon's own JSON export by running a comparison test against `tetra getevents` output
- [ ] **OCB replace directive:** Works locally via `replaces` pointing to `../` — verify CI uses published module path and `replaces` is removed or gated behind a dev flag
- [ ] **Multi-arch image:** Image built and pushed for `linux/amd64` only — verify `docker manifest inspect ghcr.io/your/image:tag` shows both `amd64` and `arm64` entries
- [ ] **Pipeline registration:** Receiver implemented and compiled but not wired into `service.pipelines.logs` in the default config — verify the sample `config.yaml` includes the receiver

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Wrong Start() context in goroutine | LOW | Add `r.ctx, r.cancel = context.WithCancel(context.Background())` in factory; update goroutine to use `r.ctx` |
| OCB version misalignment | MEDIUM | Audit all components in `builder-config.yaml`; align all to same minor version; regenerate and verify `go.mod` |
| Proto registration panic at startup | HIGH | Run `go mod graph | grep protobuf` to find conflicting versions; add explicit `require` for correct version; may require forking or replacing a transitive dep |
| gRPC reconnect storm on shutdown | LOW | Add `r.ctx.Err()` check at top of reconnect loop; test with `go test -run TestShutdown` |
| protojson output mismatch | MEDIUM | Capture reference output from live Tetragon; write diff test; adjust `MarshalOptions` to match |
| `nextConsumer` blocking receive loop | MEDIUM | Add internal buffered channel between `Recv()` loop and `ConsumeLogs()` call; add `ctx.Done()` select case |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Reusing Start() context | Phase 1 — receiver core implementation | `receivertest` lifecycle tests pass; manual test shows events flow after startup |
| OCB version misalignment | Phase 2 — OCB build setup | `ocb --config builder-config.yaml` produces clean output with no version warnings |
| OCB go.mod toolchain version | Phase 2 — OCB build setup | CI `go mod tidy` succeeds in a clean Docker environment |
| gRPC error discrimination | Phase 1 — reconnection logic | Unit test covers Canceled, EOF, Unavailable error codes; shutdown test completes in <2s |
| Protobuf dependency conflict | Phase 1 — initial module setup | Binary starts without panics; `go mod graph` shows single protobuf version |
| Shutdown before Start | Phase 1 — factory and lifecycle | `receivertest.NewFactory(...)` auto-generated lifecycle test passes |
| nextConsumer blocking | Phase 1 — receive loop design | Integration test with slow consumer shows no gRPC stall within 30s |
| protojson output mismatch | Phase 1 — event mapping | Table-driven test compares marshaled output against captured Tetragon reference JSON |
| Multi-arch image gaps | Phase 3 — container image build | `docker manifest inspect` confirms both amd64 and arm64 layers present |
| OCB replace in CI | Phase 2/3 boundary | CI build uses published module path, not local replace path |

---

## Sources

- [Build a receiver — OpenTelemetry official docs](https://opentelemetry.io/docs/collector/extend/custom-component/receiver/) — lifecycle context reuse warning, Shutdown/Start requirements
- [OCB otelcol_version mismatch issue #9626](https://github.com/open-telemetry/opentelemetry-collector/issues/9626) — version string alignment requirement
- [OCB go.mod toolchain issue #11844](https://github.com/open-telemetry/opentelemetry-collector/issues/11844) — unpinned Go version in generated go.mod
- [OCB contrib version mismatch issue #35171](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/35171) — component version calculated by deps does not match configured version
- [Shutdown must be safe without Start issue #9682](https://github.com/open-telemetry/opentelemetry-collector/issues/9682) — lifecycle contract
- [grpc-go streaming error handling issue #8190](https://github.com/grpc/grpc-go/issues/8190) — error discrimination in server-streaming Recv()
- [grpc-go context canceled race issue #3039](https://github.com/grpc/grpc-go/issues/3039) — context.Canceled vs io.EOF race on stream close
- [gRPC streaming backpressure issue #2159](https://github.com/grpc/grpc-go/issues/2159) — buffer behavior and data loss
- [OTel Collector backpressure — Axoflow blog](https://axoflow.com/blog/opentelemetry-controller-outages-pipelines-backpressure) — pipeline blocking behavior, cascading failures
- [OTel Collector receiver backpressure issue #29410](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/29410) — limiting data production to avoid backpressure
- [google.golang.org/protobuf conflict detection](https://protobuf.dev/reference/go/faq/) — duplicate proto registration panic from v1.26.0+
- [Tetragon issue #1419](https://github.com/cilium/tetragon/issues/1419) — OTel support remains unimplemented upstream; no prior art to copy
- [consumertest package docs](https://pkg.go.dev/go.opentelemetry.io/collector/consumer/consumertest) — testing helpers for receiver unit tests
- [OCB build at scale — Bindplane blog](https://bindplane.com/blog/custom-opentelemetry-collectors-build-run-and-manage-at-scale) — maintenance burden of two-week release cadence

---
*Pitfalls research for: Custom OTel Collector Tetragon gRPC receiver*
*Researched: 2026-03-18*
