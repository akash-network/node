# Reporter PR #2095 Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the CodeRabbit findings on PR #2095 (review of d60d094e) that still hold at 29c14c35, plus the failing `reporter` CI job, with one commit per finding.

**Architecture:** Each task is an independent, test-first fix. It targets the Go reporter (`operator/reporter/...`, `cmd/akt-reporter`), its TypeScript counterpart or tooling (`contracts/solana/...`), the docs, or CI. The tasks run in severity order. Several touch the same files (run.go, broadcaster.go, the spec), so they run one after another. Once earlier tasks have shifted lines, locate code by function name.

**Tech Stack:** Go 1.26.2 (module `pkg.akt.dev/node/v2`), testify, cosmos-sdk fork v0.53.8-akash.1, CometBFT fork v0.38.26-akash.1, grpc v1.84.0, prometheus client_golang v1.23.2, gogoproto. TypeScript (tsx, node:test) in `contracts/solana`.

**Spec:** `docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md`. Section numbers such as §5.4 refer to it. The findings come from https://github.com/akash-network/node/pull/2095 (CodeRabbit review 5941817762); IDs M1-M6 are its major comments and m1-m19 its minor ones.

## Global Constraints

- Before any `go`, `make` or `golangci-lint` command, load the environment in that shell: `export PATH="$(brew --prefix)/opt/make/libexec/gnubin:$PATH"; eval "$(direnv export bash 2>/dev/null)"`.
- Run Go tests the way `make test-reporter` does: `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 …`. Tasks 14-19 and O2-O3 abbreviate this as `gt`, so define it first: `gt() { GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 "$@"; }`.
- Line numbers are at 29c14c35. Once an earlier task has landed, find the code by function name.
- Do not add, remove or upgrade Go modules or npm packages. `go.mod`, `go.sum`, `package.json` and the lockfiles stay unchanged.
- Tests use `github.com/stretchr/testify/require`. Use `assert` inside goroutines.
- Change behaviour only as far as the finding requires. Edit any spec or operator-doc passage that describes the changed behaviour in the same commit.
- Keep error, alert and event texts verbatim. Do not reword existing ones.
- Make one commit per task, using Conventional Commits with scope `reporter`, at most 10 lines in total. Never push.
- golangci-lint (repo `.golangci.yaml`, v2.3.0 from `.cache/bin`) must report no new finding on changed lines.
- Do not touch or stage the four untracked files at the repository root (`Akash Mainnet 19 Upgrade Proposal.md`, `mainnet-19.md`, `params-prop.json`, `upgrade-prop-v2.2.0.json`). Stage files by path, never with `git add -A` or `git add .`.

## Review Focus

- Task 5: `emitReview` holds `abortMu` while it emits. If `Emit` can block (a full writer), the aborting pass's `setAborted` must not deadlock behind it.
- Task 2: endpoints that fail to parse, have no host, or use IPv6 literals (`http://[::1]:26657/x`) are logged as `scheme://host`, never as the raw string. `redactURL` leaves errors without a `*url.Error` unchanged.
- Task 9: for a `reporter.toml` symlinked into another directory, the temporary file is created next to the target. No temporary file survives any failure path, including the in-place fallbacks.
- Task 4: a `metrics.listen` set to a hostname or to port 0 binds before the pass. `stopMetrics` never blocks when the bind failed.
- Task 1: a `Log truncated` marker inside an AKT frame that has not closed yet, or a second marker, must not yield that frame's event.

## Decisions

The user asked for every finding to be fixed. These rulings were made while drafting:
- M3 (Task 5): the abort is enforced in code through an abort generation, so the doc and spec keep their promise.
- M3 (Task 5): when a follow-up stops mid-chunk, `followDone` can report "not sent" for transfers it reviewed that had no messages. This is accepted, because fixing it would need a change to the scan API.
- m3 (Task 9): config writes use a temporary file and a rename.
  - Fall back to the in-place write when `Rename` fails with EBUSY or EXDEV (single-file bind mount), or when `CreateTemp` fails with a permission error (EACCES, EPERM or EROFS).
  - Every other error leaves the file untouched.
  - A failed write is tested through an unexported write seam, exposed to tests by `export_test.go`.
- m18 (Task 8): use a per-transaction retry flag that the refresh tick clears. Up to one RefreshInterval of delay after a failed lookup is accepted.
- m11 (Task 7): if `Latest` fails just before signing, sign with the last chain time seen. That is no worse than today, and safety does not depend on the cutoff (spec §6.2).
- m14 (Task 15): detach replies with a gogoproto marshal/unmarshal round trip while the lock is held.
- m8 (Task 17): detect an existing key with `keys list --output json`.
- Task 10 changes only the test. It still takes about 24 s, because Comet's decode is not bound by the deadline.
- Task 18 updates every statement of the formula: operator-configuration.md:85, spec :242, resolve.go:51 and config.go:167.
- Tasks 16, 18, 19 and 20 check with a command that fails before the change, not with a unit test.
- Task 17 has no failing check, because `stack.ts` has no seam. Its check pins the CLI contract that the fix relies on.

Not planned:
- M5 is already fixed by 8006e019; resolve the thread.
- m17's suggested change would let a late event pass the idle assertions.

Declined by the user, so the threads get replies and no code change: m10 (cached evidence ignores a cancelled ctx), m13 (simnode account reads around AfterCalls), m15 (simnode first block deadline) and m4 (keyring OS-backend fallback). None has practical impact today.

---

### Task 1: M4 stop at the first `Log truncated` and keep the frames closed before it
**Finding:** `operator/reporter/solana/logs.go:49-50` rejects any line after the marker. Agave (v1.18 to master, `LogCollector::log`) writes the marker once, does not count the dropped message, and keeps appending messages that fit. A later oversized log therefore voids an AKT event whose frame already closed.
**Files:** Modify: `operator/reporter/solana/logs.go:20,36-37,48-54`, `contracts/solana/reporter/solana.ts:94-114`, `docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md:246` / Test: `operator/reporter/solana/logs_test.go:15,123-125`, `contracts/solana/reporter/solana.test.ts:577-589`
- [ ] Step 1: Write the failing tests. In `TestProgramEvents`, subtest "log truncation keeps only top-level AKT frames that already succeeded": change the "lines after the truncation marker" case (123-125) to `false, ""`, delete the now unused `errTruncated` const (15), and add after that case:
```go
{"lines after the marker keep a closed AKT frame", []string{
	inv(budget, 1), ok(budget), inv(p, 1), e, "Program " + p + " consumed 1 of 2 compute units", ok(p),
	inv(other, 1), truncatedLine, "Program " + other + " consumed 3 of 4 compute units", ok(other),
}, true, ""},
```
In `solana.test.ts`, same-named test, add after "lines after the truncation marker". Keep that case `false`, because its AKT frame comes after the marker. Run prettier afterwards:
```ts
["lines after the marker keep a closed AKT frame", (p, e) => [
  `Program ${budget} invoke [1]`, `Program ${budget} success`, `Program ${p} invoke [1]`, e,
  `Program ${p} consumed 1 of 2 compute units`, `Program ${p} success`, `Program ${other} invoke [1]`,
  "Log truncated", `Program ${other} consumed 3 of 4 compute units`, `Program ${other} success`,
], true],
```
- [ ] Step 2: Run and confirm failure.
  `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestProgramEvents$/^log_truncation' ./operator/reporter/solana/`
  Both cases fail with `Received unexpected error: inconsistent truncated logs` (logs_test.go:140).
  `cd contracts/solana && npx tsx --test --test-name-pattern="log truncation keeps only top-level AKT frames" reporter/solana.test.ts`
  The new case rejects with `resolution evidence unavailable in its block`.
- [ ] Step 3: Implement.
  - **logs.go:48-54:** replace the block with `if line == logTruncated { return flatten(closed, id), nil }`. Delete `errTruncatedLogs` (20). Change the doc (36-37) to: "A later instruction can exhaust the log limit. The runtime then writes `Log truncated` once and keeps logging the messages that still fit, so parsing stops at the first marker: only top-level program frames that closed with success before it count."
  - **solana.ts:108-114:** loop with `for (const line of logs)`, drop the assert, and return the same `closed.filter(...).flatMap(...)` at the marker. Use the same wording in the comment at 96-98.
  - **spec:246:** replace "When `Log truncated` is the last line, only top-level AKT frames that closed with `success` count." with "Agave writes `Log truncated` once and keeps logging later messages that still fit, so parsing stops at the first `Log truncated`: only top-level AKT frames that closed with `success` before it count, and the lines after it are ignored."
  - **Why this is safe:** everything before the first marker is complete, since the marker takes the place of the first dropped message. Programs cannot write invoke or exit lines. The lines after the marker are a filtered subset and are never parsed.
- [ ] Step 4: Run and confirm pass. Rerun both Step 2 commands, then:
  `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/solana/ ./operator/reporter/observe/`
  In `contracts/solana`: `npm run test:reporter && npm run typecheck && npx prettier --check reporter/solana.ts reporter/solana.test.ts`
- [ ] Step 5: Commit. `git add operator/reporter/solana/logs.go operator/reporter/solana/logs_test.go contracts/solana/reporter/solana.ts contracts/solana/reporter/solana.test.ts docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md && git commit -m "fix(reporter): keep evidence logged before a log truncation marker" -m "Agave writes \"Log truncated\" once and keeps logging messages that still fit, so a later instruction's oversized log voided the event of an AKT frame that had already closed. Stop at the first marker in Go and TS and ignore the lines after it."`

### Task 2: M1 Redact RPC endpoints in every mode
**Finding:** `legacyLogEndpoint` (cmd/akt-reporter/cmd/legacy.go:82-91) redacts only legacy JSON. Canonical runs log the full `node` and `solana.rpc` URLs (run.go:173, 530, 532), and canonical CometBFT transport errors keep the URL path (`comet`, operator/reporter/akash/client.go:338-350).
**Files:** Modify: `cmd/akt-reporter/cmd/run.go:173,513-540`, `cmd/akt-reporter/cmd/legacy.go:6,81-91`, `operator/reporter/akash/client.go:5-16,337-350`, `contracts/solana/docs/reporter.md:64,268`, `docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md:639` / Test: `operator/reporter/clitest/preview_test.go`
- [ ] Step 1: Write the failing test `TestCanonicalEndpointsRedacted`. `reporterArgs` puts overrides last, and simnode serves Solana on any path:
```go
func TestCanonicalEndpointsRedacted(t *testing.T) {
	n := startNode(t, "clean")
	t.Run("startup logs", func(t *testing.T) {
		r := runToEnd(t, reporterArgs(n, t.TempDir(), "scan",
			"--node", n.CometURL+"?token=SOURCE", "--solana.rpc", n.SolanaURL+"/SECRET?api-key=SECRET")...)
		require.Equal(t, 0, r.code, r.stderr)
		require.Contains(t, r.stderr, "solana.rpc="+n.SolanaURL)
		for _, out := range []string{r.stdout, r.stderr} {
			require.NotContains(t, out, "SOURCE")
			require.NotContains(t, out, "SECRET")
		}
	})
	t.Run("CometBFT transport error", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		origin := "http://" + l.Addr().String()
		require.NoError(t, l.Close()) // the dial is refused
		r := runToEnd(t, reporterArgs(n, t.TempDir(), "scan", "--node", origin+"/SOURCE")...)
		require.Equal(t, 1, r.code, r.stderr)
		require.Contains(t, r.stderr, `connect to Akash: post failed: Post "`+origin+`": dial tcp`)
		require.NotContains(t, r.stderr, "SOURCE")
	})
}
```
- [ ] Step 2: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestCanonicalEndpointsRedacted$' ./operator/reporter/clitest/`. Expected: `startup logs` fails with `should not contain "SOURCE"` (stderr shows `node=…?token=SOURCE` and `solana.rpc=…/SECRET?api-key=SECRET`). `CometBFT transport error` fails with `does not contain "connect to Akash: post failed: Post \"http://127.0.0.1:…\": dial tcp"`, because the URL still ends in `/SOURCE`.
- [ ] Step 3: Implement.
  - run.go: add `logEndpoint(raw string) string` beside `logStartup`. It returns `u.Scheme+"://"+u.Host`, or `"invalid URL"` when parsing fails or the host is empty. Use it at :173, :530 and :532. Delete `legacyLogEndpoint` and legacy.go's `net/url` import.
  - client.go `comet` (:344-346): keep `sanitizeLegacyHTTP` for legacy. Otherwise call `err = redactURL(err)`, a new function next to `comet`.
    - `redactURL` finds the `*url.Error` with `errors.As`. It returns `&redactedError{msg, cause}`, whose `msg` is `strings.ReplaceAll(err.Error(), strconv.Quote(ue.URL), strconv.Quote(origin))`, where origin is `scheme://host` from `url.Parse(ue.URL)`, and whose `Unwrap()` returns `ue.Err`.
    - Mutating `ue.URL` in place has no effect: CometBFT's `fmt.Errorf("post failed: %w")` has already rendered the URL into the outer text. `ue.Err` holds no URL and keeps cancellation detectable.
  - Docs:
    - reporter.md:64 becomes "Keep credentials out of the URL and use a local authenticated proxy: `user:password@` is refused, and a key in the path or query is not detected".
    - reporter.md:268 and spec :639 say startup logs show RPC endpoints as `scheme://host`.
- [ ] Step 4: Re-run Step 2's command, then `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/clitest/ ./operator/reporter/akash/ ./cmd/akt-reporter/cmd/`.
- [ ] Step 5: `git add cmd/akt-reporter/cmd/run.go cmd/akt-reporter/cmd/legacy.go operator/reporter/akash/client.go operator/reporter/clitest/preview_test.go contracts/solana/docs/reporter.md docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md`, then
  `git commit -m "fix(reporter): redact RPC endpoints in logs and errors" -m "Providers put API keys in URL paths and queries, which configuration accepts. Log node and solana.rpc as scheme://host in every mode, and cut CometBFT transport errors down to the origin."`

### Task 3: M2 Metrics server timeouts
**Finding:** `metrics.Serve` sets only `ReadHeaderTimeout` (operator/reporter/metrics/metrics.go:229-232). A request whose body never arrives, or an idle keep-alive connection, holds a goroutine and a socket forever (go1.26.2 net/http/server.go:1049-1050, 2098-2111).
**Files:** Modify: `operator/reporter/metrics/metrics.go:15-21,229-232` / Test: `operator/reporter/metrics/timeouts_test.go` (new file, `package metrics`)
- [ ] Step 1: Write the failing test `TestServeClosesStalledConnections` (imports: context, io, net, testing, time, prometheus, require):
```go
func TestServeClosesStalledConnections(t *testing.T) {
	saved := serverTimeouts
	serverTimeouts = timeouts{header: 200 * time.Millisecond, read: 200 * time.Millisecond, write: time.Second, idle: 200 * time.Millisecond}
	t.Cleanup(func() { serverTimeouts = saved })
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, addr, prometheus.NewRegistry()) }()
	t.Cleanup(func() { cancel(); require.NoError(t, <-done) })
	for name, request := range map[string]string{
		"withheld body":   "POST /metrics HTTP/1.1\r\nHost: m\r\nContent-Length: 1\r\n\r\n",
		"idle keep-alive": "GET /metrics HTTP/1.1\r\nHost: m\r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			var c net.Conn
			require.Eventually(t, func() bool { var derr error; c, derr = net.Dial("tcp", addr); return derr == nil }, 5*time.Second, 10*time.Millisecond)
			defer c.Close()
			_, err := io.WriteString(c, request)
			require.NoError(t, err)
			require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
			_, err = io.Copy(io.Discard, c) // the server may answer, but must then close
			require.NoError(t, err, "the connection stayed open")
		})
	}
}
```
- [ ] Step 2: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestServeClosesStalledConnections$' ./operator/reporter/metrics/`. Expected: the build fails with `undefined: serverTimeouts` and `undefined: timeouts`, because the seam is new. If the var is added but not wired into `Serve`, both subtests fail after 5s with `i/o timeout`.
- [ ] Step 3: Implement in metrics.go.
  - Replace the `headerTimeout` const (:18) with `type timeouts struct{ header, read, write, idle time.Duration }`.
  - Add `var serverTimeouts = timeouts{header: 10 * time.Second, read: 10 * time.Second, write: 30 * time.Second, idle: 2 * time.Minute}`, commented "keeps a client from holding a connection; tests shorten it".
  - In `Serve` (:229-232), set `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` and `IdleTimeout` from it.
- [ ] Step 4: Re-run Step 2's command, then `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/metrics/`.
- [ ] Step 5: `git add operator/reporter/metrics/metrics.go operator/reporter/metrics/timeouts_test.go`, then
  `git commit -m "fix(reporter): bound metrics connection lifetimes" -m "With only ReadHeaderTimeout, a withheld request body or an idle keep-alive connection held a goroutine and a socket forever. Add read, write and idle timeouts."`

### Task 4: m6 Bind the metrics listener before scanning
**Finding:** `serveMetrics` binds inside its goroutine (`lc.Listen` at metrics.go:236, run from run.go:427-433) and logs "serving metrics" (run.go:435) before the bind. The scan therefore starts before the bind, and a bind failure that arrives after `scanOnce` is ignored (run.go:124-128).
**Files:** Modify: `operator/reporter/metrics/metrics.go:224-239`, `operator/reporter/metrics/metrics_test.go:139-192`, `operator/reporter/metrics/timeouts_test.go`, `cmd/akt-reporter/cmd/run.go:113-118,415-441`, spec `:558` / Test: `operator/reporter/clitest/preview_test.go`
**Interfaces:**
- `func Serve(ctx context.Context, lis net.Listener, g prometheus.Gatherer) error` serves g at /metrics on `lis` until ctx is cancelled, then shuts down and closes `lis`.
- cmd: `func (r *reporter) serveMetrics(ctx context.Context, fail context.CancelCauseFunc) (stop func(), err error)` binds `cfg.Metrics.Listen` with `net.ListenConfig.Listen` before it returns. A bind error comes back as `&listenerError{err: err}`, and `runReporter` returns it before `startBroadcaster`.
- After the bind, `serveMetrics` runs `metrics.Serve(ctx, lis, r.gather)` in its goroutine. An error from it still goes to `fail` and ends the run.
- [ ] Step 1: Write the failing test `TestMetricsBindFailureStopsBeforeScanning`:
```go
func TestMetricsBindFailureStopsBeforeScanning(t *testing.T) {
	n := startNode(t, "clean")
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = taken.Close() })
	r := runToEnd(t, command(t, n, "scan", "--metrics.listen", taken.Addr().String())...)
	require.Equal(t, 1, r.code, r.stderr)
	require.Contains(t, r.stderr, "metrics listener: listen tcp "+taken.Addr().String())
	require.NotContains(t, r.stderr, "serving metrics")
	require.Empty(t, r.stdout, "no pass started")
	require.Zero(t, n.Calls("getGenesisHash"), "no Solana read before the bind")
}
```
- [ ] Step 2: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestMetricsBindFailureStopsBeforeScanning$' ./operator/reporter/clitest/`. Expected: `should not contain "serving metrics"`. Past that check, stdout also holds the summary of a pass.
- [ ] Step 3: Implement.
  - metrics.go: `Serve` drops `lc.Listen` (:234-239) and serves `lis`.
  - run.go:417-441: return `func() {}, nil` for an empty address. Otherwise bind with `var lc net.ListenConfig; lis, err := lc.Listen(ctx, "tcp", addr)`, start the goroutine, and log `Stringer("listen", lis.Addr())` after the bind.
  - run.go:117-118: `stopMetrics, err := r.serveMetrics(runCtx, cancel)`, then `if err != nil { return err }`, then `defer stopMetrics()`.
  - Tests:
    - metrics_test.go: `TestServe` keeps `lis` open and passes it to `Serve`.
    - Replace `TestServeListenError` with `TestServeAcceptError`: `Serve` on a closed listener returns an error.
    - timeouts_test.go: pass `l` to `Serve`. Drop `l.Close()` and the dial `Eventually`.
  - Spec :558: "Finally they bind the metrics listener, if configured; a bind failure ends the command before any pass."
- [ ] Step 4: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^(TestMetricsBindFailureStopsBeforeScanning|TestMetricsInPreview)$' ./operator/reporter/clitest/`, then `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/metrics/ ./cmd/akt-reporter/cmd/ ./operator/reporter/clitest/`.
- [ ] Step 5: `git add operator/reporter/metrics/ cmd/akt-reporter/cmd/run.go operator/reporter/clitest/preview_test.go docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md`, then
  `git commit -m "fix(reporter): bind the metrics listener before scanning" -m "The listener bound inside a goroutine, so a pass started first and a bind failure after scan's pass was dropped. Bind synchronously and fail startup before any pass."`

### Task 5: M3 follow-ups honour the pass abort
**Finding:** `operator/reporter/scan/scan.go:617-618` gives a running follow-up a request context and a per-transfer loop that never see the pass's abort. In `start`, it then writes reviews and queues votes after the pass's mismatch alert and error. This was demonstrated with a temp test.
**Files:** Modify: `operator/reporter/scan/scan.go:49-52,96-109,139-146,358-421,470-477,562-637`, spec `:301` / Test: `operator/reporter/scan/scan_test.go` (after `TestOneMismatchAlertPerPass`, 926), `operator/reporter/tx/broadcaster_test.go` (after `TestAbortedScanReportsDeferred`, 1771)
**Interfaces:** no exported change. Internally, `followUp(ctx, st, gen uint64, chunk)` gains `gen`, and `reviewRun` gains a `gen uint64` field.
- [ ] Step 1: Write the failing tests. In scan_test.go:
```go
func TestFollowUpStopsOnPassMismatch(t *testing.T) {
	pin := simnode.LightClient
	for name, later := range map[string]bool{"no review or queue after the pass's error": false, "a later pass does not resume it": true} {
		t.Run(name, func(t *testing.T) {
			sc := simnode.Scenarios()["votes"]()
			ids := idsOf(t, sc)
			h := newHarness(t, sc, setup{pin: &pin, queue: &recorder{}})
			// The follow-up pauses in its first Queue call: its snapshot passed the pin and its first review is written.
			own, entered, release, done := &recorder{}, make(chan struct{}), make(chan struct{}), make(chan error, 1)
			var once sync.Once
			own.onQueue = func(policy.Planned) {
				once.Do(func() {
					close(entered)
					select {
					case <-release:
					case <-time.After(10 * time.Second):
					}
				})
			}
			go func() { done <- h.scanner.Review(scan.WithQueuer(t.Context(), own), ids[:2], 0) }()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("the follow-up never queued")
			}

			setBridge(h.node, other)
			require.Equal(t, 1, h.scanner.Pass(t.Context(), nil), "as in start, settle does not wait for follow-ups")
			lines := h.lines()
			require.Contains(t, decode(t, lines[len(lines)-1])["error"], "differs from pinned")
			if later {
				setBridge(h.node, pin)
				h.clock.Advance(time.Second)
				require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
				h.lines()
			}

			close(release)
			require.ErrorIs(t, <-done, scan.ErrPassAborted)
			require.Empty(t, h.lines(), "nothing follows")
			require.Len(t, own.all(), 1, "only the Queue call that began before the mismatch")
		})
	}
}
```
In broadcaster_test.go, a guard against double reports:
```go
func TestAbortedFollowUpReportsOnlyWhatItSkipped(t *testing.T) {
	h := newHarness(t, setup{})
	p := h.start(h.options(), h.signer, nil)
	st := h.snapshot()
	w := WindowOf(st.Time)
	a, b := h.plan(st, 0), h.plan(st, 1)
	until := func(pl policy.Planned) time.Time { return SlotTimeout(w, slotOf(pl)) }
	p.state(func(s *ownerState) {
		for _, pl := range []policy.Planned{a, b} {
			s.deferred[pl.Key] = &deferral{planned: pl, reason: reasonSlotBusy, window: w, until: until(pl)}
		}
	})

	// In window w+1 the follow-up queues a again, then a pass's pin mismatch stops it before b.
	h.blockAt(WindowStart(w + 1).Add(time.Second))
	h.tick()
	job := followJob{id: 9, window: w + 1, ids: []sinkid.ID{a.ID, b.ID}, keys: []policy.Key{a.Key, b.Key}}
	require.True(t, followQueuer{b: p.b, job: job.id}.Queue(h.plan(h.snapshot(), 0)))
	require.True(t, p.b.call(func() { p.b.followDone(job, scan.ErrPassAborted) }))

	_, errs := outcomes(t, p.lines())
	require.Equal(t, map[string]string{b.ID.String(): "sink message not sent: deferred (slot_busy) until " + rfc3339(until(b))}, errs)
	require.Equal(t, []string{a.ID.String()}, h.waitBroadcasts(1)[0].ids)
}
```
- [ ] Step 2: Run and confirm failure.
  `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestFollowUpStopsOnPassMismatch$' ./operator/reporter/scan/`
  Both subtests fail with `Expected error with "follow-up review skipped: the pass ended on a light-client pin mismatch" in chain but got nil.`
  `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestAbortedFollowUpReportsOnlyWhatItSkipped$' ./operator/reporter/tx/`
  This already passes, and it stays as a guard: `queue()` deletes a deferral it queues again (table.go:198), and a new deferral carries the current window, so `followDone` does not cover it.
- [ ] Step 3: Implement in scan.go. Add the `Scanner` field `abortGen atomic.Uint64` after `aborted`. Its comment: "counts the pin mismatches that aborted a pass; a follow-up stops once it moves past the value it began with, so the next pass, which clears aborted, cannot resume it". In `setAborted`, under `abortMu`, add `if v { s.abortGen.Add(1) }` before `s.aborted.Store(v)`. Then add:
```go
// beginFollowUp returns the abort generation a follow-up runs under; false means it is skipped.
func (s *Scanner) beginFollowUp() (uint64, bool) {
	s.abortMu.Lock()
	defer s.abortMu.Unlock()
	return s.abortGen.Load(), !s.aborted.Load()
}

// abortedSince reports a pin mismatch that a pass found after the follow-up of r began.
func (s *Scanner) abortedSince(r reviewRun) bool { return r.followUp && s.abortGen.Load() != r.gen }

// emitReview writes ev unless abortedSince(r). Holding abortMu puts the event before the aborting pass's alert;
// its messages are then queued as already planned (spec §5.6).
func (s *Scanner) emitReview(r reviewRun, ev events.Review) bool {
	if r.followUp {
		s.abortMu.Lock()
		defer s.abortMu.Unlock()
		if s.abortGen.Load() != r.gen {
			return false
		}
	}
	s.events.Emit(ev)
	return true
}
```
  - **Review (586), ReviewRefundHead (564):** make the first statement `gen, ok := s.beginFollowUp(); if !ok { return ErrPassAborted }`. Replace the later `s.aborted.Load()` checks (576, 593, 601) with `s.abortGen.Load() != gen`, and pass `gen` to `followUp`.
  - **followUp (615):** set `gen: gen` in `reviewRun`. After `followUpFailures.Add`, add `if errors.Is(err, ErrPassAborted) { return err }`. In the mismatch branch (627), test `s.abortGen.Load() != gen` instead of `s.aborted.Load()`.
  - **reviewAt:** after the stop/req check (401-403), add `if s.abortedSince(r) { return ErrPassAborted }`. Before `s.fail` (415), add `if errors.Is(err, ErrPassAborted) { return err }`. Update the doc (359) to say that a follow-up may also return ErrPassAborted.
  - **review (470-477):** replace the `Emit` with `if !s.emitReview(r, events.Review{...}) { return ErrPassAborted }`. The Queue loop runs only after this check passes.
  - **Docs:** `ErrPassAborted` (49-51): "…once a pass has found a light-client pin mismatch: at once until the next pass begins, and before the next review event of a follow-up already running, which never resumes." Spec:301: "…are skipped. One already running stops before its next review event and never resumes, even after the next pass begins."
  - **followDone/reportDeferred:** no change (see the Step 2 guard).
- [ ] Step 4: Run and confirm pass. Rerun both Step 2 commands, then:
  `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/scan/ ./operator/reporter/tx/`
  This also covers `TestNoFollowUpAfterPinMismatch`, `TestOneMismatchAlertPerPass`, `TestFollowUpFailuresCount` and `TestAbortedScanReportsDeferred`.
- [ ] Step 5: Commit. `git add operator/reporter/scan/scan.go operator/reporter/scan/scan_test.go operator/reporter/tx/broadcaster_test.go docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md && git commit -m "fix(reporter): stop running follow-up reviews on a pass's pin mismatch" -m "A follow-up checked the abort only when it began, so in start it kept writing reviews and queuing votes after the pass's mismatch alert and error. It now runs under the abort generation it began with and stops before its next review; the next pass cannot resume it."`

### Task 6: m5 Join follow-up reviews before scan's summary
**Finding:** After a signal, scan's settle step (run.go:391) returns from `b.Wait` as soon as `Run` closes `done`. `Run` closes it before `b.wg.Wait()` joins the follow-up goroutine (operator/reporter/tx/broadcaster.go:351-355). A follow-up review still running then writes after the summary, and its failure misses `followUpFailures` (scan.go:175-179). The settle step of `start` already joins (run.go:452-458).
**Files:** Modify: `cmd/akt-reporter/cmd/run.go:121-125,379-392` / Test: `operator/reporter/clitest/broadcast_test.go`
**Interfaces:** unexported only: `func (r *reporter) scanOnce(ctx context.Context, stopBroadcaster func()) int`.
- [ ] Step 1: Write the failing test `TestScanSigtermJoinsFollowUp`. In refund-queue, R1's inclusion starts a refund-head review of R2. The pass reviews R2 using its stored vote, so it calls no `getBlock`. After the test drops that vote, the follow-up's evidence read is slow and fails.
```go
func TestScanSigtermJoinsFollowUp(t *testing.T) {
	sc := simnode.Scenarios()["refund-queue"]()
	sc.Solana.Failures = map[string]simnode.Failure{"getBlock": {Code: -32004, Message: "block not available"}}
	rec, err := sinkid.DecodeRecord(sc.Akash.Transfers[1].Transfer)
	require.NoError(t, err)
	// Blocks come only when the test commits one, so R1's refund is included after the pass.
	n, err := simnode.Start(t.Context(), sc, simnode.Options{AlignWindow: true, BlockFetchLatency: 3 * time.Second})
	require.NoError(t, err)
	t.Cleanup(n.Close)
	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() { done <- run(t.Context(), &stdout, &stderr, reporterArgs(n, withKey(t), "scan", keyringBroadcast...)...) }()
	require.Eventually(t, func() bool { return strings.Count(stdout.String(), `"observed"`) >= 3 }, 60*time.Second, 5*time.Millisecond, stderr.String())
	n.Mutate(func(sc *simnode.Scenario) { sc.Akash.Transfers[1].Transfer.Settlement.Votes = nil })
	require.Eventually(t, func() bool { n.CommitBlock(); return n.Calls("getBlock") > 0 }, 60*time.Second, 200*time.Millisecond, stderr.String())
	sigterm(t) // the follow-up now waits on getBlock, and no transaction is in flight

	require.Equal(t, 1, exitCodeOf(t, done), stderr.String())
	events := lines(t, stdout.String())
	last := len(events) - 1
	require.Equal(t, []int{last}, summaryIndexes(events), "the summary is the last line")
	require.EqualValues(t, 1, events[last]["failures"], "the follow-up's failure counts")
	failed := -1
	for i, ev := range events {
		if ev["id"] == rec.ID.String() && ev["error"] != nil {
			failed = i
		}
	}
	require.True(t, failed >= 0 && failed < last, "R2's follow-up error precedes the summary")
}
```
- [ ] Step 2: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestScanSigtermJoinsFollowUp$' ./operator/reporter/clitest/`. Expected: `expected: 1 actual: 0`. Today the summary counts 0 failures and R2's error is written after it.
- [ ] Step 3: Implement.
  - run.go:125: `failures := r.scanOnce(runCtx, stopBroadcaster)`.
  - `scanOnce` (:391): in the settle closure, add `if ctx.Err() != nil { stopBroadcaster(); failures += b.TakeFailures() }` after `failures := b.Wait(settleCtx)`, mirroring start's settle. Comment that `Wait` returns once `Run` closes `done`, which is before `Run` joins its follow-up review.
  - Update the `scanOnce` doc comment: on a signal, settle joins the broadcaster, so a follow-up review writes before the summary and its failures count (spec §6.6).
- [ ] Step 4: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^(TestScanSigtermJoinsFollowUp|TestSigtermOrdering|TestScanBroadcastEndToEnd)$' ./operator/reporter/clitest/`, then `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/clitest/ ./cmd/akt-reporter/cmd/`.
- [ ] Step 5: `git add cmd/akt-reporter/cmd/run.go operator/reporter/clitest/broadcast_test.go`, then
  `git commit -m "fix(reporter): join follow-up reviews before scan's summary" -m "After a signal, Wait returned when the broadcaster closed done, before its follow-up review ended. The review then wrote after the summary, and its failure was not counted."`

### Task 7: m11 Re-check the send cutoff before signing
**Finding:** `operator/reporter/tx/broadcaster.go:841` signs after Prune (:792), but the cutoff was last checked at :764, before simulation. A slow simulation can therefore end in signing past the cutoff. Spec design.md:411 already says "Before signing … the send cutoff must not have passed", so no spec edit is needed.
**Files:** Modify: `operator/reporter/tx/broadcaster.go:741-762, 839-841` / Test: `operator/reporter/tx/broadcaster_test.go` (new test after `TestCutoff`, :1016)
- [ ] Step 1: Write the failing test `TestCutoffRecheckedBeforeSigning`. It is a table of `{name string; sim time.Duration; sent bool}`: `{"cutoff passes during simulation", 12 * time.Second, false}` and `{"cutoff still holds after simulation", time.Second, true}`. For each case, build `s`, `vote` and `b` as `TestManualGasKnownCaps` does (manual_gas_test.go:160-201), with `Options{Reporter: s.Address().String(), GasAdjustment: 1.3, GasPrices: testPrices, MaxGas: 20_000_000}` and this chain:
```go
timeout := start.Add(time.Minute)
st := &akash.State{Height: 42, Time: timeout.Add(-21 * time.Second), Policy: sinkv1.ReporterPolicy{Members: []string{s.Address().String()}}}
chain := &capBuildChain{state: st}
chain.sim = func([]byte) (uint64, error) {
	later := *chain.state
	later.Time = later.Time.Add(c.sim) // the chain moves on while simulation runs
	chain.state = &later
	return 100_000, nil
}
res := b.build(buildJob{msgs: []policy.Planned{vote}, timeout: timeout, chainTime: st.Time})
if c.sent {
	require.NotNil(t, res.sent, "20 s left: still signed")
	return
}
require.Nil(t, res.sent, "not signed past the cutoff")
require.Empty(t, s.got)
require.Empty(t, chain.broadcasts)
require.Len(t, res.deferred, 1)
require.Equal(t, reasonCutoff, res.deferred[0].reason)
require.True(t, res.seen.Time.Equal(timeout.Add(-9*time.Second)), "the owner sees the later block")
```
- [ ] Step 2: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestCutoffRecheckedBeforeSigning$' ./operator/reporter/tx/`. It should fail with `--- FAIL: TestCutoffRecheckedBeforeSigning/cutoff_passes_during_simulation`, `Expected nil, but got: &tx.firstSend{` and `Messages: not signed past the cutoff`. The "still holds" case passes.
- [ ] Step 3: Implement in `func (b *Broadcaster) build(job buildJob) (res buildResult)`: declare the closure `refresh` below after `retry` (:748), and replace the block at :756-762 with `st, err := refresh()`. Between the sign-bytes block (:839) and `b.b.Build` (:841), call it again and check the cutoff. Latest is cached, so the extra call is cheap; if it fails, the last chain time seen stands.
```go
refresh := func() (*akash.State, error) {
	st, err := b.chain.Latest(b.work)
	if err == nil {
		res.see(st)
		if st.Time.After(chainTime) {
			chainTime = st.Time
		}
	}
	return st, err
}
// … right before Build:
_, _ = refresh()
if job.timeout.Sub(chainTime) < cutoff {
	res.deferAll(msgs, reasonCutoff)
	return res
}
```
- [ ] Step 4: Run the Step 2 command again; it passes. Then run the whole package, `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/tx/` (about 2.5 min); it passes, and `TestCutoff` and `TestManualGasKnownCaps` are unchanged.
- [ ] Step 5: Commit:
```sh
git add operator/reporter/tx/broadcaster.go operator/reporter/tx/broadcaster_test.go
git commit -m "fix(reporter): re-check the send cutoff before signing

A simulation can outlast the slot's send cutoff after the first check, and the
transaction was then signed and sent only to expire. Measure chain time again
right before Build and defer with reason cutoff (spec §6.2)."
```

### Task 8: m18 Retry a failed lookup at the next refresh
**Finding:** `operator/reporter/tx/table.go:386` records `looked = h` before the `err` check. Pump (`broadcaster.go:451`) only looks a transaction up again when `looked < s.height`, so after a failed lookup nothing retries until a new height arrives.
**Files:** Modify: `operator/reporter/tx/table.go:73-74, 384-389`, `operator/reporter/tx/broadcaster.go:428-432, 451`, `docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md:460` / Test: `operator/reporter/tx/broadcaster_test.go` (after `TestConfirmationEvents`, :1552)
- [ ] Step 1: Write the failing test `TestFailedLookupRetriedAtSameHeight` with a wrapper in the style of `unavailableSim` (:1803-1815):
```go
// failingLookups fails every transaction lookup while down is set.
type failingLookups struct {
	Chain
	down  *atomic.Bool
	calls atomic.Int64
}

func (c *failingLookups) Tx(ctx context.Context, hash []byte) (*akash.TxResult, error) {
	c.calls.Add(1)
	if c.down.Load() {
		return nil, status.Error(codes.Unavailable, "connection refused")
	}
	return c.Chain.Tx(ctx, hash)
}
// TestFailedLookupRetriedAtSameHeight:
h := newHarness(t, setup{})
var down atomic.Bool
down.Store(true)
lookups := &failingLookups{down: &down}
p := h.start(h.options(), h.signer, nil, func(c Chain) Chain { lookups.Chain = c; return lookups })
veto := h.plan(h.snapshot(), firstVeto)
require.True(t, p.b.Queue(veto))
h.waitBroadcasts(1)
h.commit() // includes the veto; no later block follows
height := h.snapshot().Height
h.eventually(func() bool { return lookups.calls.Load() >= 2 }, "looked up at the inclusion height")
time.Sleep(50 * time.Millisecond)
n := lookups.calls.Load()
time.Sleep(50 * time.Millisecond)
require.Equal(t, n, lookups.calls.Load(), "no retry without a refresh tick")
down.Store(false)
h.eventually(func() bool { return p.count() == 1 }, "confirmed at the same height")
require.Equal(t, height, h.snapshot().Height, "no new block")
hashes, errs := outcomes(t, p.lines())
require.Contains(t, hashes, veto.ID.String())
require.Empty(t, errs)
```
- [ ] Step 2: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestFailedLookupRetriedAtSameHeight$' ./operator/reporter/tx/`. After 60 ticks it should fail with `Error: Should be true` and `Messages: confirmed at the same height`.
- [ ] Step 3: Implement. Never re-dispatch from `applyLookup`: pump runs after every op, so that would loop against a failing RPC (the Step 1 sleep check catches this).
  - table.go `liveTx`: after `looked` (:74), add `lookupFailed bool` with the comment `// lookupFailed is set after a failed lookup: the next one waits for a refresh tick, not a new height.`
  - table.go `applyLookup` (:385-389): after `tx.busy = false`, add `if err != nil { tx.lookupFailed = true; return }`. Then set `tx.looked = max(tx.looked, h)`, and drop `|| err != nil` from the guard that follows.
  - broadcaster.go `pump` (:451): add `!tx.lookupFailed &&` before `tx.looked < s.height`.
  - broadcaster.go `tick` (:428-432): in the entries loop, set `e.tx.lookupFailed = false` for every non-nil `e.tx`, before the resend check.
  - design.md:460: change "On every new height, each unconfirmed transaction" to "On every new height, and at the next refresh after a failed lookup, each unconfirmed transaction".
- [ ] Step 4: Run the Step 2 command again; it passes. Then run the whole package, `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/tx/` (about 2.5 min); it passes.
- [ ] Step 5: Commit:
```sh
git add operator/reporter/tx/table.go operator/reporter/tx/broadcaster.go operator/reporter/tx/broadcaster_test.go docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md
git commit -m "fix(reporter): retry a failed transaction lookup at the next refresh

A failed lookup still recorded its height, so a node that recovered at the
same height, as on a halted chain, could not confirm the transaction. Retry
on the next refresh tick rather than from the lookup's completion."
```

### Task 9: m3 Replace reporter.toml safely on init --overwrite
**Finding:** `writeConfig` truncates with `O_TRUNC` before it writes (`cmd/akt-reporter/cmd/init.go:83-107`), so a failed write leaves an empty or partial file. The 0600 mode (:93) applies only when the file is created, so an existing 0644 file stays 0644.
**Files:** Modify: `cmd/akt-reporter/cmd/init.go:83-107`, spec `:551`, `contracts/solana/docs/reporter.md:55` / Create: `cmd/akt-reporter/cmd/export_test.go` (`package cmd`) / Test: `cmd/akt-reporter/cmd/cmd_test.go` (`package cmd_test`)
**Interfaces:**
- init.go: `var writeFileData = (*os.File).Write`. Every reporter.toml write goes through it; only tests replace it.
- export_test.go: `func SetConfigWriter(w func(*os.File, []byte) (int, error)) (restore func())`.
- [ ] Step 1: Write the failing test.
  - First route the existing write through the seam, with no behaviour change: init.go:101 `f.Write(data)` becomes `writeFileData(f, data)`.
  - Add `export_test.go` with `SetConfigWriter`.
  - Then add `TestInitOverwriteReplacesSafely` to cmd_test.go:
```go
func TestInitOverwriteReplacesSafely(t *testing.T) {
	seed := func(t *testing.T, mode os.FileMode) (home, path string) { // an old reporter.toml in a fresh home
		home = t.TempDir()
		path = config.FilePath(home)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("old = 1\n"), 0o600))
		require.NoError(t, os.Chmod(path, mode))
		return home, path
	}
	only := func(t *testing.T, dir string) { // no temporary file is left behind
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
	}
	t.Run("a failed write keeps the old file", func(t *testing.T) {
		home, path := seed(t, 0o600)
		t.Cleanup(cmd.SetConfigWriter(func(f *os.File, b []byte) (int, error) {
			n, _ := f.Write(b[:len(b)/2])
			return n, errors.New("disk full")
		}))
		code, _, stderr := execute(t, "", "init", "--home", home, "--overwrite")
		require.Equal(t, 1, code)
		require.Contains(t, stderr, "disk full")
		data, err := os.ReadFile(path) //nolint:gosec // a test directory
		require.NoError(t, err)
		require.Equal(t, "old = 1\n", string(data))
		only(t, filepath.Dir(path))
	})
	t.Run("a looser mode becomes 0600", func(t *testing.T) {
		home, path := seed(t, 0o644)
		code, _, stderr := execute(t, "", "init", "--home", home, "--overwrite")
		require.Equal(t, 0, code, stderr)
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		only(t, filepath.Dir(path))
	})
	t.Run("a symlink stays and its target is replaced", func(t *testing.T) {
		_, target := seed(t, 0o600)
		home, link := seed(t, 0o600)
		require.NoError(t, os.Remove(link))
		require.NoError(t, os.Symlink(target, link))
		code, _, stderr := execute(t, "", "init", "--home", home, "--overwrite", "--chain-id", "other-1")
		require.Equal(t, 0, code, stderr)
		got, err := os.Readlink(link)
		require.NoError(t, err)
		require.Equal(t, target, got)
		data, err := os.ReadFile(target) //nolint:gosec // a test directory
		require.NoError(t, err)
		require.Contains(t, string(data), `chain-id = "other-1"`)
		only(t, filepath.Dir(target))
	})
	t.Run("a directory without write access falls back to an in-place write", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory modes")
		}
		home, path := seed(t, 0o600)
		require.NoError(t, os.Chmod(filepath.Dir(path), 0o500))       //nolint:gosec // no temporary file can be created
		t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0o700) }) //nolint:gosec // restored for removal
		code, _, stderr := execute(t, "", "init", "--home", home, "--overwrite", "--chain-id", "other-1")
		require.Equal(t, 0, code, stderr)
		data, err := os.ReadFile(path) //nolint:gosec // a test directory
		require.NoError(t, err)
		require.Contains(t, string(data), `chain-id = "other-1"`)
	})
}
```
- [ ] Step 2: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestInitOverwriteReplacesSafely$' ./cmd/akt-reporter/cmd/`. Expected:
  - "a failed write keeps the old file" fails on the content check: expected `"old = 1\n"`, actual the first half of the new config.
  - "a looser mode becomes 0600" fails with `expected: 0x180 actual: 0x1a4`.
  - The symlink and read-only-directory subtests already pass. They guard the rename and fallback paths.
- [ ] Step 3: Implement in init.go.
  - `writeConfig`: after `MkdirAll`, add `if overwrite { return replaceConfig(path, data) }`. The `O_EXCL` branch keeps its behaviour and writes through `writeSynced`.
  - `writeSynced(f *os.File, data []byte) error`: `f.Chmod(0o600)`, `writeFileData`, `f.Sync()`, then `errors.Join(err, f.Close())`.
  - `writeInPlace(path string, data []byte) error`: `os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)`, then `writeSynced`.
  - `replaceConfig(path string, data []byte) (err error)`:
    1. Call `filepath.EvalSymlinks(path)`. An `fs.ErrNotExist` error keeps `path`; any other error is returned.
    2. Call `os.CreateTemp(filepath.Dir(path), ".reporter.toml.*")`. If it fails with `fs.ErrPermission` or `syscall.EROFS`, return `writeInPlace(path, data)`. Any other error is returned with the file untouched.
    3. A `defer` removes the temporary file when `err != nil`.
    4. Call `writeSynced(tmp, data)`, then `os.Rename(tmp.Name(), path)`. If `Rename` fails with `syscall.EBUSY` or `syscall.EXDEV`, remove the temporary file and return `writeInPlace(path, data)`.
  - The EBUSY/EXDEV fallback has no unit test, because it needs a bind mount.
  - Docs (spec :551, reporter.md:55) should say:
    - With `--overwrite`, init writes a synced 0600 temporary file and renames it over the target; through a symlink, it replaces the link's target.
    - A failed write keeps the old file.
    - Where no temporary file can be created or renamed into place (a single-file bind mount), init writes in place.
- [ ] Step 4: Re-run Step 2's command, then `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./cmd/akt-reporter/cmd/`.
- [ ] Step 5: `git add cmd/akt-reporter/cmd/init.go cmd/akt-reporter/cmd/export_test.go cmd/akt-reporter/cmd/cmd_test.go contracts/solana/docs/reporter.md docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md`, then
  `git commit -m "fix(reporter): replace reporter.toml atomically on init --overwrite" -m "O_TRUNC emptied the old file before the write and kept a looser existing mode. Rename a synced 0600 temporary file over the target, and write in place only where a bind mount or directory permissions rule out a rename."`

### Task 10: CI TestLegacySimulationBounds deadline covers mock marshalling

**Finding:** `legacy_simulation_test.go:322` sets a 10 s `c.timeout`, but `mockABCI` marshals the 64 MiB+1 reply (~89 MB JSON) inside it, before any headers (`abci_test.go:86-89`). Under `-race` on slow cores that takes 12–15 s, so `Simulate` returns `context.DeadlineExceeded` (`abci.go:397-398`) and `:324` sees `codes.Unknown` (0x2) instead of `ResourceExhausted` (0x8).

**Files:**
- Modify: `operator/reporter/akash/legacy_simulation_test.go:15-16` (import) and `:310-326` (`TestLegacySimulationBounds` only; `mockABCI` and its other callers stay unchanged)

**Sufficiency (measured at 29c14c35 with the Step 2 code as a scratch `-overlay` test, `-race`):** with the reply pre-built, the deadline covers only headers and body read. Throttled (`taskpolicy -b`): first byte 1 ms, body written 182–248 ms; passed 3/3, one run with a 1 s deadline. Pre-build (7.6–7.8 s) and Comet's post-read decode (~52 s) run outside the deadline. Normal cores: 3.1 s pre-build, 68 ms under the deadline, 24 s total. Pre-building is enough.

- [ ] **Step 1: Reproduce the failure (RED).** macOS only: `taskpolicy -b` runs the test on slower background-QoS cores.

```bash
export PATH="$(brew --prefix)/opt/make/libexec/gnubin:$PATH"; eval "$(direnv export bash 2>/dev/null)"
GOWORK=off CGO_ENABLED=1 GO111MODULE=on taskpolicy -b go test -mod=readonly -race -count=3 -v -run '^TestLegacySimulationBounds$' ./operator/reporter/akash/
```
Expected: 3/3 `--- FAIL: TestLegacySimulationBounds` (~12 s each) at `legacy_simulation_test.go:324` with `expected: 0x8` / `actual  : 0x2`.

- [ ] **Step 2: Implement.** Replace `:311-318` (`calls` + `mockABCI`) with the block below, a test-local server modelled on `TestLegacySimulationMalformedEnvelopeAndPrivacy` (`:331-371`). `cmtjson.Marshal` is what `rpctypes.NewRPCSuccessResponse` uses, so the reply bytes match the old mock's. `raw` gets its own `Write`; concatenating it would copy ~89 MB inside the deadline. At `:319` change `_, err :=` to `_, err =`; keep the rest of `:319-325` unchanged, including `c.timeout = 10 * time.Second`. Add `cmtjson "github.com/cometbft/cometbft/libs/json"` after the `abci` import (`:15`).

```go
	// Marshal once, outside the request deadline: under -race this takes seconds.
	raw, err := cmtjson.Marshal(&ctypes.ResultABCIQuery{Response: abci.ResponseQuery{
		Height: 42,
		Value:  []byte(strings.Repeat(" ", maxRecvBytes+1)),
	}})
	require.NoError(t, err)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpctypes.RPCRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if req.Method == "status" {
			data, err := json.Marshal(rpctypes.NewRPCSuccessResponse(req.ID, &ctypes.ResultStatus{}))
			require.NoError(t, err)
			_, _ = w.Write(data)
			return
		}
		calls.Add(1)
		id, err := json.Marshal(req.ID)
		require.NoError(t, err)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":`))
		_, _ = w.Write(raw)
		_, _ = w.Write([]byte(`}`))
	}))
	defer server.Close()
	c, err := Dial(t.Context(), Options{ABCI: true, Node: server.URL, RequestTimeout: time.Second})
	require.NoError(t, err)
	defer c.Close()
```

- [ ] **Step 3: Verify GREEN.** Rerun the Step 1 command. Expected: 3/3 `--- PASS` (~60 s each, ~3 min total). Then run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/akash/`. Expected: `ok` (this test ≈ 24 s).

- [ ] **Step 4: Commit.**

```bash
git add operator/reporter/akash/legacy_simulation_test.go
git commit -m "test(reporter): marshal oversized ABCI reply outside the deadline" -m "The mock marshalled the ~89 MB reply inside the 10 s simulate deadline, so
-race CI runs timed out with codes.Unknown instead of ResourceExhausted.
Build it once beforehand so the deadline covers only the transfer."
```

Note: the test stays slow (~24 s local, ~60 s throttled) because Comet's post-read decode is not deadline-bound. Shrinking it needs a production change, which is out of scope.

### Task 11: m9 make the noncanonical recipient case deterministic
**Finding:** `operator/reporter/sinkid/record_test.go:67` appends "1" to a random key, which multiplies it by 58. About 1.3 % of the time (1M-key measurement) the result is another canonical 32-byte key, so `DecodeRecord` accepts it.
**Files:** Modify: `operator/reporter/sinkid/record_test.go:67` / Test: same
- [ ] Step 1: Write the failing test. No new test is needed: `TestDecodeRecord/noncanonical_recipient` (65-70) is the flaky one.
- [ ] Step 2: Run and confirm failure.
  `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=600 -run '^TestDecodeRecord$' ./operator/reporter/sinkid/ 2>&1 | grep -c -- '--- FAIL: TestDecodeRecord/noncanonical_recipient'`
  Expect about 8 (10 were measured), each with `An error is expected but got nil.` at line 99.
- [ ] Step 3: Implement. At line 67, change `recipient.String()+"1"` to `recipient.String()+"0"`. "0" is outside the base58 alphabet, so the text never decodes.
- [ ] Step 4: Run and confirm pass. The Step 2 command prints `0`. Then: `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/sinkid/`
- [ ] Step 5: Commit. `git add operator/reporter/sinkid/record_test.go && git commit -m "test(reporter): make the noncanonical recipient case deterministic" -m "Appending \"1\" to a random key multiplies it by 58, which about once in 75 runs is another canonical 32-byte key. \"0\" is not base58."`

### Task 12: m12 wait for the in-flight reads before shutdown
**Finding:** `operator/reporter/akash/akash_test.go:562` sleeps 100 ms instead of proving that the two permitted reads reached `hungSink`.
**Files:** Modify: `operator/reporter/akash/akash_test.go:516-524,537-538,551,562` (test only)
- [ ] Step 1: Write the failing check. The flake did not reproduce: 80 runs under `taskpolicy -b … -count=40 -cpu 1,2` all passed. Late reads still win the free permits, because `Acquire` checks before AfterFunc's cancel goroutine runs, so only the 1 s bound breaks under starvation. Use a mutation instead: temporarily make `time.Sleep(700 * time.Millisecond)` the first statement of the read goroutine (line 558).
- [ ] Step 2: Run and confirm failure.
  `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestShutdownCancelsQueuedReads$' ./operator/reporter/akash/`
  It fails at line 583 with `"1.1…s" is not less than "1s"` and the message "shutdown takes about one request timeout", or at 584/585 if a late read lost its permit race.
- [ ] Step 3: Implement. Give `hungSink` an `entered chan struct{}` field. `Transfer` sends `s.entered <- struct{}{}` before `<-ctx.Done()`. Move `const calls = 20` above the server setup, create `sink := &hungSink{entered: make(chan struct{}, calls)}` and register `sink`. Replace line 562 with:
```go
	// Both permits are held once two reads reach the node; the others wait for a permit.
	for range opts.Concurrency {
		select {
		case <-sink.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the reads holding the permits never reached the node")
		}
	}
```
- [ ] Step 4: Run and confirm pass. The Step 2 command passes with the mutation still in place. Remove the mutation, then run:
  `GOWORK=off CGO_ENABLED=1 GO111MODULE=on taskpolicy -b go test -mod=readonly -race -count=40 -cpu 1,2 -run '^TestShutdownCancelsQueuedReads$' ./operator/reporter/akash/`
  `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/akash/`
- [ ] Step 5: Commit, with the mutation removed. `git add operator/reporter/akash/akash_test.go && git commit -m "test(reporter): wait for the in-flight reads before shutting down" -m "The test slept 100 ms and assumed both permitted reads were in flight. hungSink now signals each query it receives, and the test waits for two, bounded, so a slow scheduler cannot break the timeout bound or the counts."`

### Task 13: m19 Check each veto in the no-repeat test
**Finding:** `operator/reporter/tx/norepeat_test.go:412` (`len(ex) >= headRefund`, which is 44) still passes when a veto never executed, because refunds 44/45 and the round-2 veto add entries.
**Files:** Modify/Test: `operator/reporter/tx/norepeat_test.go:404-412`
- [ ] Step 1: Write the new assertion below :412, keeping :412 for now. Add a temporary mutation right after `ex := h.executions()` (:404) that drops veto 40's executions from `ex`, as if it never ran:
```go
delete(ex, executedVeto(h.ids[firstVeto], 1)) // MUTATION (remove in Step 3): veto 40 never executed
delete(ex, executedVeto(h.ids[firstVeto], 2))
// … below :412 (round 2 exists only for the resolved transfer):
for i := firstVeto; i < headRefund; i++ {
	id := h.ids[i]
	require.Positive(t, ex[executedVeto(id, 1)]+ex[executedVeto(id, 2)], "veto %d", i)
}
```
- [ ] Step 2: Run `REPORTER_SEED=1 GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestNoVoteExecutesTwice$/^round_0$' ./operator/reporter/tx/`. It should fail at the new loop with `"0" is not positive` and `Messages: veto 40`. Reaching the loop proves the old :412 passed with veto 40 missing. If :412 fails first, no refund executed in that round; rerun with another `REPORTER_SEED`.
- [ ] Step 3: Delete the old :412 and both MUTATION lines, and keep the loop. `h.executions()` still checks that nothing executes more than once.
- [ ] Step 4: Run `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 -run '^TestNoVoteExecutesTwice$' ./operator/reporter/tx/` (20 rounds); it passes. Then run the whole package, `GOWORK=off CGO_ENABLED=1 GO111MODULE=on go test -mod=readonly -race -count=1 ./operator/reporter/tx/` (about 2.5 min); it passes.
- [ ] Step 5: Commit:
```sh
git add operator/reporter/tx/norepeat_test.go
git commit -m "test(reporter): require every veto to execute in the no-repeat test

The total count let a refund or a round-2 veto stand in for a veto that never
executed. Check each veto transfer in round 1 or 2."
```

### Task 14: m16 Make the call-count subtest self-contained
**Finding:** `operator/reporter/internal/simnode/solana_test.go:428-432` checks counters that only sibling subtests increment, so the subtest fails when run alone.
**Files:** Modify: `operator/reporter/internal/simnode/solana_test.go:428-432` / Test: the same subtest
- [ ] Step 1: Write the failing test: none to add. `TestSolanaMethods/calls_are_counted_per_method`, run alone, is the RED.
- [ ] Step 2: Run it and confirm it fails: `gt -run '^TestSolanaMethods$/^calls_are_counted_per_method$' ./operator/reporter/internal/simnode/`
  Expected: `solana_test.go:429: … Error: "0" is not positive` and `--- FAIL`.
- [ ] Step 3: Implement: rewrite the subtest body to record the counts, make one call of each method, and check +1 deltas. `GenesisHash` and `StreamBlock` each POST exactly once (`solana/rpc.go:267-274`, `solana/block.go:162-169`).
  ```go
  hashes, blocks := n.Calls("getGenesisHash"), n.Calls("getBlock")
  _, err := rpc.GenesisHash(ctx)
  require.NoError(t, err)
  require.NoError(t, rpc.StreamBlock(ctx, legacyOnly, func(aktsolana.Tx) error { return nil }))
  require.Equal(t, hashes+1, n.Calls("getGenesisHash"))
  require.Equal(t, blocks+1, n.Calls("getBlock"))
  require.Zero(t, n.Calls("getSlot"))
  ```
- [ ] Step 4: Run and confirm it passes: the Step 2 command; then `gt -run '^TestSolanaMethods$' ./operator/reporter/internal/simnode/`; then `gt ./operator/reporter/internal/simnode/`
- [ ] Step 5: Commit: `git add operator/reporter/internal/simnode/solana_test.go && git commit -m "test(reporter): make the simnode call-count subtest self-contained" -m "It counted calls made by sibling subtests, so it failed when run alone."`

### Task 15: m14 Detach gRPC replies while the read lock is held
**Finding:** `operator/reporter/internal/simnode/grpc.go:126-141` releases `n.mu` before gRPC encodes the reply (grpc v1.84.0 `server.go:1284-1289` calls `SendMsg` after the handler returns). Replies alias scenario memory: `grpc.go:171` copies `Params` shallowly, and `Params.Reporters` shares its array with `Policies[].Members` (`scenarios.go:231,273`).
**Files:** Modify: `operator/reporter/internal/simnode/grpc.go:3-14` (import `reflect`), `:141` / Test: `operator/reporter/internal/simnode/akash_test.go` (new test after `TestBlockIntervalAndLatency`; add imports `sync` and `github.com/stretchr/testify/assert`)
- [ ] Step 1: Write the failing test `TestGRPCRepliesAreSnapshots`. Inside goroutines use `assert`, as `solana_test.go:651-671` does; use `require` only on the test goroutine.
  ```go
  c := newChain(t)
  sc := c.scenario(t)
  sc.Akash.Params.Reporters = []string{"0", "r1", "r2"} // Enabled is true and 0 is even: consistent
  n, _ := startAkash(t, sc, simnode.Options{})
  q := sinkv1.NewQueryClient(grpcConn(t, n))
  stop := make(chan struct{})
  var wg sync.WaitGroup
  for range 4 {
  	wg.Add(1)
  	go func() {
  		defer wg.Done()
  		for {
  			select {
  			case <-stop:
  				return
  			default:
  			}
  			res, err := q.Params(t.Context(), &sinkv1.QueryParamsRequest{})
  			if !assert.NoError(t, err) {
  				return
  			}
  			i, err := strconv.Atoi(res.Params.Reporters[0])
  			assert.NoError(t, err)
  			assert.Equal(t, i%2 == 0, res.Params.Enabled, "a reply mixes the old and the new state")
  		}
  	}()
  }
  for i := range 2000 { // in place, as any Mutate callback may edit
  	n.Mutate(func(sc *simnode.Scenario) {
  		sc.Akash.Params.Enabled = i%2 == 0
  		sc.Akash.Params.Reporters[0] = strconv.Itoa(i)
  	})
  }
  close(stop)
  wg.Wait()
  ```
- [ ] Step 2: Run it and confirm it fails: `gt -run '^TestGRPCRepliesAreSnapshots$' ./operator/reporter/internal/simnode/`
  Expected: `WARNING: DATA RACE`, with the write in the Mutate callback and the read in `(*Params).MarshalToSizedBuffer` under `grpc.(*serverStream).SendMsg`; then `race detected during execution of test` and `--- FAIL`.
- [ ] Step 3: Implement: in `intercept`, replace `return handler(...)` (`:141`) with `res, err := handler(...)`; return the error as is, otherwise `return detach(res)`. Add `func detach(res any) (any, error)` below `intercept`: `gogoproto.Marshal`, then `gogoproto.Unmarshal` into a fresh value of the same type, while the lock is still held. Why a round trip rather than a typed clone:
  - It is generic over every reply type of the five registered services.
  - It is deep by construction: `math.Int` and `LegacyDec` `*big.Int`s, `Any`, nested slices.
  - Clients cannot see the difference, since they only receive the encoding.
  - abci_query already encodes under the lock (`grpc.go:63-69`, `comet.go:184-189`).
  - `gogoproto.Clone`'s reflective merge is not relied on for customtype fields.
  - The cost is one extra encode and decode per reply.
  ```go
  m, ok := res.(gogoproto.Message)
  if !ok {
  	return nil, status.Errorf(codes.Internal, "simnode: reply %T is not a gogoproto message", res)
  }
  raw, err := gogoproto.Marshal(m)
  if err != nil {
  	return nil, status.Error(codes.Internal, err.Error())
  }
  out, _ := reflect.New(reflect.TypeOf(m).Elem()).Interface().(gogoproto.Message)
  if err := gogoproto.Unmarshal(raw, out); err != nil {
  	return nil, status.Error(codes.Internal, err.Error())
  }
  return out, nil
  ```
- [ ] Step 4: Run and confirm it passes: the Step 2 command; `gt ./operator/reporter/internal/simnode/`; `gt -run '^TestGoldens$' ./operator/reporter/scan/` (wire replies unchanged). Optional: `REPORTER_PERF=1 gt -run '^TestSteady100k$' -timeout 60m ./operator/reporter/scan/`. It rebuilds simnode from source, so it covers the extra encode.
- [ ] Step 5: Commit: `git add operator/reporter/internal/simnode/grpc.go operator/reporter/internal/simnode/akash_test.go && git commit -m "fix(reporter): detach simnode gRPC replies under the read lock" -m "gRPC encodes a reply after the interceptor returns, outside the lock, so a Mutate that edits scenario memory in place raced the encoding and could tear a reply. Copy each reply through its encoding first, as abci_query and the Solana endpoint already encode under the lock."`

### Task 16: m7 Fail the golden generator when simnode exits before it is ready
**Finding:** `contracts/solana/reporter/goldens.ts:60` uses `once(lines, "line")`, which never settles at EOF. The event loop then drains and the script exits 0, so the `finally` check at `:63-66` never runs.
**Files:** Modify: `contracts/solana/reporter/goldens.ts:51-68` (`serve`) / Test: none. The check is a command: `goldens.ts` runs `void main()` on import, so `tsx --test` has no way to call `serve` without a refactor.
- [ ] Step 1: Write the failing check: a fail-closed `go` shim. Only `go run … list` reaches the real go, so `serve` always exits 2 and no golden can be written.
  ```bash
  SHIM=$(mktemp -d); REAL_GO=$(command -v go)
  printf '#!/bin/sh\ncase "$*" in\n  *" list") exec "%s" "$@" ;;\n  *) echo "go-shim: refusing: $*" >&2; exit 2 ;;\nesac\n' "$REAL_GO" > "$SHIM/go"; chmod +x "$SHIM/go"
  ```
- [ ] Step 2: Run it and confirm it fails: `(cd contracts/solana && PATH="$SHIM:$PATH" npx tsx reporter/goldens.ts clean; echo "exit=$?")`
  Expected today: `go-shim: refusing: run ./operator/reporter/internal/simnode/cmd/simnode serve --scenario clean`, then `exit=0` with no error and no `clean: … events` line. Ignore the DEP0040 warning. `git status --short operator/reporter/testdata` shows nothing.
- [ ] Step 3: Implement in `serve`:
  - Add `const failed = (code: number | null) => \`simnode serve --scenario ${scenario} failed with exit code ${code}\`;`.
  - Replace `:60` with `const first = await Promise.race([once(lines, "line").then(([line]) => line as string), exited.then(([code]) => { throw new Error(failed(code)); })]);`.
  - Change the `finally` assert to `assert.equal(code, 0, failed(code));`.

  `Promise.race` subscribes to both inputs, so the `exited` branch that rejects after a normal start is handled. A scratch replica confirmed it: no unhandled rejection on success, exit 1 on early exit. simnode serves until stdin closes, so `exit` cannot beat a ready line it printed.
- [ ] Step 4: Run and confirm it passes: rerun Step 2. Expected `AssertionError [ERR_ASSERTION]: simnode serve --scenario clean failed with exit code 2` and `exit=1`. Then, without the shim, `(cd contracts/solana && npx tsx reporter/goldens.ts clean)` must print `clean: 6 events` and exit 0. `git status --short operator/reporter/testdata` must stay empty: the goldens are deterministic and `pda_vectors.json` keeps its two programs. Do not commit testdata. Finally `(cd contracts/solana && npx tsc --noEmit && npx prettier --check reporter/goldens.ts)`.
- [ ] Step 5: Commit: `git add contracts/solana/reporter/goldens.ts && git commit -m "fix(reporter): fail goldens when simnode exits before it is ready" -m "The generator waited for a ready line that never came; the event loop then drained and the script exited 0 without the remaining goldens."`

### Task 17: m8 Import local-stack reporter keys only when absent
**Finding:** `contracts/solana/local/stack.ts:537-545`: the bare catch turns every `keys show` failure into an import, and `readFile` is awaited after the import child has already been spawned.
**Files:** Modify: `contracts/solana/local/stack.ts:533-545`
- [ ] Step 1: Write the failing test: no automated test. `stack.ts` has no seam: `main` runs only under `require.main === module`, `goReporterHome` is not exported, and `local:up` boots the whole stack. Step 2 pins the CLI behaviour the fix relies on instead.
- [ ] Step 2: Run it and confirm the contract: `make akt-reporter && H=$(mktemp -d) && { .cache/bin/akt-reporter keys show reporter -a --home "$H"; echo "exit=$?"; .cache/bin/akt-reporter keys list --output json --home "$H"; echo " exit=$?"; }`
  Expected: `Error: reporter is not a valid name or address: decoding bech32 failed: invalid separator index -1` and `exit=1`. There is no "key not found" text, and every error exits 1. Then `[] exit=0`.
- [ ] Step 3: Implement: replace the try/catch with a keyring listing, and read the key before spawning:
  ```ts
  const { stdout } = await keys(["list", "--output", "json"]);
  if ((JSON.parse(stdout) as { name: string }[]).some((k) => k.name === "reporter")) return;
  const secret = await readFile(join(ROOT, `reporter${index}.key`), "utf8");
  const imported = keys(["import-hex", "reporter"]);
  imported.child.stdin?.end(secret);
  await imported;
  ```
- [ ] Step 4: Run and confirm it passes: `(cd contracts/solana && npx tsc --noEmit && npx prettier --check local/stack.ts)`; the baseline `tsc` exits 0. Then confirm detection after an import: `printf '%064x\n' 1 | .cache/bin/akt-reporter keys import-hex reporter --home "$H" && .cache/bin/akt-reporter keys list --output json --home "$H" | jq -e 'any(.[]; .name == "reporter")'` prints `true`. Optional end-to-end, which needs the local-stack prerequisites: `REPORTER_GO_BIN="$PWD/.cache/bin/akt-reporter" npm --prefix contracts/solana run local:up`, then `run local:status` and `run local:down`.
- [ ] Step 5: Commit: `git add contracts/solana/local/stack.ts && git commit -m "fix(reporter): import local stack reporter keys only when absent" -m "keys show exits 1 for any error, so the bare catch turned every failure into an import. List the keyring instead, and read the key before spawning the import so its failure is always awaited."`

### Task 18: m-doc State the rounding of the derived max slot lead
**Finding:** `contracts/solana/docs/operator-configuration.md:85` and `operator/reporter/config/resolve.go:51` give `(max-age + 60 s) × 5`, but `resolve.go:222-226` rounds the sum up to whole seconds first. Sub-second `max-age` values such as `500ms` pass validation (`:179-181`): 305 slots, not 302.5.
**Files:** Modify: `contracts/solana/docs/operator-configuration.md:85`, `operator/reporter/config/resolve.go:51`, `operator/reporter/config/config.go:167` (flag help), `docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md:242`. Leave spec :925 (a porting note) and `scan/golden_test.go:47` (it uses an integer max-age) as they are.
- [ ] Step 1: Write the failing test: none (docs and help text only).
- [ ] Step 2: Baseline: `grep -n 'rounded up to whole seconds' contracts/solana/docs/operator-configuration.md operator/reporter/config/resolve.go operator/reporter/config/config.go docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md` prints nothing.
- [ ] Step 3: Implement:
  - Rules cell of row 85: `At most 100,000. Derived as \`(max-age + 60 s) × 5\` slots, with the sum rounded up to whole seconds`. This is 98 characters, inside the 103-wide column; re-pad with `(cd contracts/solana && npx prettier --write docs/operator-configuration.md)`.
  - `resolve.go:51`: `// SlotLead is solana.max-slot-lead or, when that is 0, (max-age + 60 s) rounded up to whole seconds, x 5.`
  - `config.go:167`: `"slots the cursor may lead, at most 100000; 0 derives (max-age + 60s, rounded up to whole seconds) x 5"`.
  - Spec :242: `The lead is \`solana.max-slot-lead\`, or (max-age + 60 s, rounded up to whole seconds) × 5 slots per second when that key is 0.`
- [ ] Step 4: Verify by read-back: rerun the Step 2 grep, which must list all four lines. Then run `(cd contracts/solana && npx prettier --check docs/operator-configuration.md)`. `gofmt -l operator/reporter/config/` must print nothing. Finally run `gt ./operator/reporter/config/ ./cmd/akt-reporter/cmd/ ./operator/reporter/clitest/`, which catches any test that pins the flag help.
- [ ] Step 5: Commit: `git add contracts/solana/docs/operator-configuration.md operator/reporter/config/resolve.go operator/reporter/config/config.go docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md && git commit -m "docs(reporter): state the rounding of the derived max slot lead" -m "The derivation rounds max-age plus 60 s up to whole seconds before multiplying by 5, so a sub-second max-age differs from the exact formula."`

### Task 19: m1 Disable test caching in the performance gates
**Finding:** `.github/workflows/reporter-perf.yaml:33-36` uses only cacheable flags (`-run`, `-timeout`, `-v`; `-p` is a build flag). `setup-ubuntu/action.yaml:33-36` (setup-go@v5, default `cache: true`) restores GOCACHE. The tests read only fixed env vars and write outside the module root, which cmd/go ignores. A rerun of an unchanged commit can therefore replay a cached pass.
**Files:** Modify: `.github/workflows/reporter-perf.yaml:36`
- [ ] Step 1: Write the failing test: none (CI config).
- [ ] Step 2: Run the check and confirm it fails: `yq '.jobs.perf.steps[] | select(.name == "Run performance gates") | .run' .github/workflows/reporter-perf.yaml | grep -F -- '-count=1'` exits 1.
- [ ] Step 3: Implement: change `-timeout 60m -v -p 1` to `-count=1 -timeout 60m -v -p 1`.
- [ ] Step 4: Verify: `if command -v actionlint >/dev/null; then actionlint .github/workflows/reporter-perf.yaml; fi`. actionlint is not installed here, so the YAML parse is the check: the Step 2 pipeline must print the `-count=1 …` line and exit 0.
- [ ] Step 5: Commit: `git add .github/workflows/reporter-perf.yaml && git commit -m "ci(reporter): disable test caching in the performance gates" -m "setup-go restores GOCACHE and the gate uses only cacheable flags, so a rerun of an unchanged commit could replay a cached pass without measuring, writing profiles or drawing a new no-repeat seed."`

### Task 20: M6 Justify the slot index conversions for gosec
**Finding:** `operator/reporter/tx/slots.go:54` and `:57` raise gosec G115 (uint64 to int). They appear only as lint-go annotations; the job still passes because of `--issues-exit-code=0`. The values are always below 128 and 1024.
**Files:** Modify: `operator/reporter/tx/slots.go:54,57`
- [ ] Step 1: Edit both lines, keeping the arithmetic unchanged because `TestSlotVectors` pins the slots. :54 becomes `return int(h % VoteSlots) //nolint:gosec // h % VoteSlots is below 128`, and :57 becomes `return VoteSlots + int(h%SingleSlots) //nolint:gosec // h % SingleSlots is below 1024`.
- [ ] Step 2: Run from the repo root, so that `.golangci.yaml` and golangci-lint v2.3.0 from `.cache/bin` are used: `golangci-lint run ./operator/reporter/tx/... --timeout=10m 2>&1 | grep 'slots.go' || echo "no slots.go findings"`. It should print `no slots.go findings`. The package's other findings (G115 at broadcaster_test.go:1918 and manual_gas_test.go:171, and prealloc) are out of scope.
- [ ] Step 3: Commit:
```sh
git add operator/reporter/tx/slots.go
git commit -m "style(reporter): justify the slot index conversions for gosec

gosec G115 flags int(h % VoteSlots) and int(h % SingleSlots), but both values
are below 1024. Annotate them like the outcome conversion at slots.go:45."
```
