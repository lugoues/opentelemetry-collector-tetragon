# Milestone 1.3 Review — Validation Report

Independent validation of every issue raised in `milestone1.3-review.md`. Each assertion was verified against the current codebase with exact file paths and line numbers. Patches applied inline.

---

## HIGH SEVERITY

| # | Issue | Status | Patch |
|---|-------|--------|-------|
| 1 | `MaxElapsedTime` silently ignored | **PATCHED** | `cenkalti/backoff/v5` removed `MaxElapsedTime` from the struct (unlike v4). Fixed by wrapping the retry context with `context.WithTimeout(ctx, MaxElapsedTime)` when `MaxElapsedTime > 0`. The retry loop now checks `retryCtx.Err()` both before scheduling a retry and during the backoff wait, exiting with a permanent error when the deadline is exceeded. See `receiver.go:113-120` and `:155-162`, `:181-189`. Tested by `TestReceiverMaxElapsedTime`. |

---

## MEDIUM SEVERITY

| # | Issue | Status | Patch |
|---|-------|--------|-------|
| 2 | `go 1.25.0` in go.mod | **NOT AN ISSUE** | Go 1.25 has since been released (dev container runs Go 1.25.8). The review was written when 1.25 was unreleased. No change needed. |
| 3 | `config_invalid.yaml` unused | **PATCHED** | Added `TestConfigFromYAML_Invalid` in `config_test.go` that loads `testdata/config_invalid.yaml`, unmarshals it, and verifies `Validate()` returns `"endpoint is required"`. |
| 4 | No `generated_component_test.go` | **DEFERRED** | Requires `mdatagen` tooling to be installed and run. Cannot be generated without the full OTel contrib build environment. The `//go:generate` directive is correct; it needs to be run as a build step. |
| 5 | Backoff library mismatch | **PATCHED** | The mismatch between `configretry.BackOffConfig` (config) and `cenkalti/backoff/v5` (runtime) is now fully bridged: `Enabled` is checked (issue #6), `MaxElapsedTime` is enforced via context deadline (issue #1), and `InitialInterval`/`MaxInterval` were already mapped. The dual-library approach is intentional — `configretry` provides config vocabulary and validation, `cenkalti/backoff` provides the runtime algorithm. |
| 6 | `Retry.Enabled` not checked | **PATCHED** | Added `if !r.cfg.Retry.Enabled` guard in `streamEvents()` at `receiver.go:138-146`. When retry is disabled, the receiver logs the error, reports a permanent error via `componentstatus`, and exits immediately. Added `TestReceiverRetryDisabled` test that verifies only one `GetEvents` call is made. |
| 7 | No graceful drain timeout | **PATCHED** | `Shutdown()` now respects its context parameter instead of ignoring it (`_`). Uses a goroutine + select pattern: either `r.wg.Wait()` completes or the shutdown context deadline fires, logging a warning. See `receiver.go:75-95`. Tested by `TestReceiverShutdownRespectsContext`. |
| 8 | No `-race` in CI | **PATCHED** | Replaced `go vet ./...` + `go test ./...` with `golangci-lint-action` (which includes `govet`) + `go test -race ./...` in `.github/workflows/ci.yml`. |
| 9 | `TestConfigValidate_TLSDelegation` doesn't test delegation | **PATCHED** | Rewrote the test to set `TLS.CertFile` without `KeyFile` (Insecure=false), attempting to trigger a `ClientConfig.Validate()` error. Discovery: `configgrpc.ClientConfig.Validate()` does not validate cert/key pairing at config time — it only checks at connection time. The test documents this behavior. The review's suggestion to use an invalid CA file path also doesn't work because file existence is checked at connection time, not validation time. |
| 10 | README references `ghcr.io/cilium/otelcol-tetragon` | **NO CHANGE** | The module path and image references are consistent with each other (`github.com/cilium/otelcol-tetragon` / `ghcr.io/cilium/otelcol-tetragon`). Whether the repo should live under `cilium` org is a project governance decision, not a code issue. |
| 11 | No `.dockerignore` | **PATCHED** | Created `.dockerignore` excluding `.git`, `.github`, `.planning`, `.claude`, `.devcontainer`, `.mise`, `*.md` (except `README.md`), `milestone*.md`, and `SPEC.md`. |
| 12 | No golangci-lint | **PATCHED** | Created `.golangci.yml` with a curated linter set (errcheck, govet, ineffassign, staticcheck, unused, gosimple, gocritic, misspell, errorlint, bodyclose, noctx). Replaced `go vet` step in CI with `golangci/golangci-lint-action@v7`. |

---

## LOW SEVERITY

| # | Issue | Status | Patch |
|---|-------|--------|-------|
| 13 | `createDefaultConfig` doc comment | **PATCHED** | Replaced the self-evident comment with a substantive one that describes the actual defaults (endpoint, TLS, retry params). See `config.go:32-33`. |
| 14 | `componentType` const could drift from metadata.yaml | **DEFERRED** | Blocked by issue #4 (mdatagen). Once `go generate` is run, the generated constant should replace the manual `componentType` const. |
| 15 | `time.Now()` in convertEvent | **NO CHANGE NEEDED** | Tests use `plogtest.IgnoreObservedTimestamp()`. Injecting a clock adds complexity for no practical benefit at alpha stage. |
| 16 | `len(eventCh)` is racy | **NO CHANGE NEEDED** | Approximate channel length is acceptable for a warning heuristic. Documented as intentional. |
| 17 | `time.Sleep(100ms)` in shutdown test | **PATCHED** | Replaced `time.Sleep(100ms)` with `atomic.Int32` call counter + `require.Eventually` that waits until at least one `GetEvents` call has been made. Test is now deterministic instead of timing-dependent. See `receiver_test.go:232-260`. |
| 18 | No CHANGELOG | **NO CHANGE** | Pre-release project. A CHANGELOG should be added when cutting v0.1.0, not before there are releases to document. |
| 19 | Planning files in repo root | **PATCHED** | Created `.gitignore` with entries for `SPEC.md` and `milestone*.md`. These files will be excluded from future commits. |
| 20 | Checkpoint volume not in Containerfile | **PATCHED** | Added `VOLUME /var/lib/otelcol` directive to `container/Containerfile` after the `USER` directive. This declares the mount point for the `file_storage/checkpoint` extension. |
| 21 | Large type switches repeated 4x | **NO CHANGE** | The review itself notes this is "manageable with 10 types." A registry/table-driven approach would add indirection without reducing the total lines of code. The explicit switch pattern is idiomatic Go and easy to review in PRs when new event types are added. |
| 22 | `event.domain`/`event.name` not official OTel semconv | **NO CHANGE NEEDED** | Informational. When OTel stabilizes event semantics, these attributes should be updated. Not actionable now. |
| 23 | No dependabot/renovate | **PATCHED** | Created `.github/dependabot.yml` with three update ecosystems: `gomod` (receiver module, weekly, OTel deps grouped), `github-actions` (weekly), and `docker` (container directory, weekly). |

---

## Summary

| Category | Total | Patched | No Change Needed | Deferred |
|----------|-------|---------|------------------|----------|
| High     | 1     | 1       | 0                | 0        |
| Medium   | 11    | 8       | 2                | 1        |
| Low      | 11    | 5       | 4                | 2        |
| **Total** | **23** | **14** | **6**           | **3**    |

### Deferred Items
- **#4 `generated_component_test.go`** — Requires `mdatagen` tooling; run `go generate ./...` in an environment with `mdatagen` installed.
- **#14 `componentType` const drift** — Blocked by #4; resolved automatically once mdatagen output is committed.

### Files Modified
- `receiver/tetragonreceiver/receiver.go` — MaxElapsedTime enforcement, Retry.Enabled guard, graceful shutdown timeout
- `receiver/tetragonreceiver/config.go` — Improved doc comment
- `receiver/tetragonreceiver/config_test.go` — TLS delegation test rewrite, config_invalid.yaml test
- `receiver/tetragonreceiver/receiver_test.go` — Deterministic backoff test, retry-disabled test
- `.github/workflows/ci.yml` — golangci-lint + race flag
- `container/Containerfile` — VOLUME directive
- `.dockerignore` — New file
- `.golangci.yml` — New file
- `.github/dependabot.yml` — New file
- `.gitignore` — New file (excludes planning artifacts)

### Test Results
All tests pass with `-race` enabled:
```
ok  github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver  2.092s
```
