# akt-reporter (Go) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the TypeScript sink reporter release with a CGO-free Go binary, `akt-reporter`. It must make the same decisions, review 100,000 open transfers per pass in at most 60 s, broadcast through unordered Akash transactions, take akash-style configuration, and ship through GoReleaser as a self-starting multi-arch image.

**Architecture:** Each pass reads Akash state through gRPC at one pinned height, lists open transfers as compact IDs, and reviews them in chunks of 96. A chunk shares one finalized Solana snapshot, and evidence comes from a per-slot block index. A pure policy package plans messages, and a broadcaster sends them in batched, unordered transactions signed by the Cosmos keyring or a Cosmos-KMS `SignerService`. Each transaction's timeout comes from a fixed nonce slot of its messages (spec §6.1), so the chain refuses any second execution of a vote, veto or refund.

**Tech Stack:** Go (module `pkg.akt.dev/node/v2`), cobra and viper, cosmos-sdk fork v0.53.8-akash.1 (client, tx, keyring, keys), CometBFT fork v0.38.26-akash.1 RPC client, chain-sdk `pkg.akt.dev/go/node/sink/v1`, `gagliardetto/solana-go` v1.14.0 (keys and PDAs only), grpc, prometheus, `golang.org/x/sync`, testify, GoReleaser v2.17.1 `dockers_v2`.

**Spec:** `docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md`. Section numbers below (for example §6.4) refer to it, and executors read both documents.

## Global Constraints

- Every `go` command in this plan runs with `GOWORK=off`. Export it before you start; the commands below omit it for brevity. The local `go.work` points `pkg.akt.dev/go` at a sibling checkout, which CI does not have.
- All reporter code lives in module `pkg.akt.dev/node/v2` and builds with `CGO_ENABLED=0 -mod=readonly`. Do not add, remove or upgrade modules in `go.mod`; only the direct/indirect moves from `go mod tidy` are allowed.
- Solana blocks and transactions are decoded from JSON (`encoding: json`, `maxSupportedTransactionVersion: 1`). Never decode binary Solana transactions.
- The event schema is the TS schema (§9.1, Appendix A):
  - 64-bit integers and amounts are decimal strings;
  - bytes are standard base64;
  - enums are numbers;
  - message fields use ts-proto camelCase;
  - transaction hashes are uppercase hex;
  - `messages` and `alerts` are `[]`, never `null`.
- Events go to stdout, one line per event. Logs go to stderr.
- Error and alert texts quoted in this plan or the spec are copied verbatim.
- Configuration:
  - home `$HOME/.akt-reporter` (`--home`, `AKT_REPORTER_HOME`), file `<home>/config/reporter.toml`;
  - environment prefix `AKT_REPORTER_`, mapping `.` and `-` to `_`;
  - precedence: flag, then environment, then file, then default.
- Network guard (§8.4):
  - `mainnet` requires chain ID `akashnet-2` and genesis `5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d`, and refuses program `FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5`;
  - `testnet` refuses both mainnet identities.
- Transactions are unordered only:
  - sequence 0, `SIGN_MODE_DIRECT`, memo `AKT sink reporter`;
  - `timeout_timestamp` is the slot timeout T(w, s) of spec §6.1, computed from the planning snapshot's block time, never from the host clock;
  - no `timeout_height`.
- No repeated execution (spec §6.1): one transaction carries messages of a single window and slot. The nonce-scheme constants (2 min window, 128 vote slots, 1,024 veto and refund slots, 1 ms spacing, the hash input) are protocol values. They are never configuration; only tests in package `tx` may scale them.
- Release artifacts:
  - GoReleaser `dockers_v2` image for `linux/amd64` and `linux/arm64`;
  - distroless static base, UID 10001, `ENTRYPOINT ["/usr/bin/akt-reporter"]`, `CMD ["start"]`.
- Tests use `github.com/stretchr/testify/require`. A test ported from TS keeps the TS title as its subtest name.
- Commits use Conventional Commits, at most 10 lines.

## Review Focus

These are the failure modes the spec implies but no ported TS test exercises. Each has a test in the task that owns the code.

1. **A lower snapshot, a restart, a lost response or a second process signing with the same key.** A load-balanced node can return a lower height than an earlier refresh, and a restarted process forgets what it had in flight.
   - Every decision must still use one height.
   - No vote, veto or refund may execute twice (spec §6.1).
   - Tests: Task 10 `TestTrackerAcceptsLowerHeight`; Task 12 `TestReviewWaitsForHeight`; Task 18 `TestUnorderedReplayKey` (the real application); Task 19 `TestEarlierWindowNeverSent`, `TestFirstSendInCache`, `TestRestartNoDoubleExecution`, `TestTwoProcessesShareSlots`, `TestVetoAcrossGovernance` and `TestNoVoteExecutesTwice`.
2. **Solana chunk snapshots complete out of slot order across workers.** Follow-mode light-client alerts must never flap. Test: Task 11 `TestFollowRuleOutOfOrder`.
3. **255-byte deposit owners**, which give bech32 strings of about 420 characters. Parsing, ordering, key suffixes and PDAs must all work. Test: Task 1 `TestParseLongOwner`.
4. **A host clock up to 10 s ahead of or behind both chains.** Freshness must accept [−10 s, `max-age`], and slot windows and timeouts must derive from block time, not the host clock. Tests: Task 10 `TestFreshnessWindow`, Task 18 `TestSlotsUseBlockTime`.
5. **Operator misconfiguration:** unknown environment variables, both signers set, a read-only home, the `os` keyring without a secret service. Each needs a clear error or warning, never a silent fallback. Tests: Task 7 `TestUnknownEnvWarns` and `TestBroadcastNeedsExactlyOneSigner`, Task 13 `TestKeysUseConfiguredBackend` and `TestOSBackendUnavailable`, Task 21 `TestReadOnlyHome`.

---

## File Structure

| Path | Responsibility |
|---|---|
| `operator/reporter/internal/clock/clock.go` | `Clock` interface, real clock, fake clock for tests |
| `operator/reporter/sinkid/{id,record}.go` | Canonical transfer IDs, ordering, key suffix, record decoding, `uakt` coin checks |
| `operator/reporter/events/{writer,types}.go` | JSON-lines writer and TS-shaped event types |
| `operator/reporter/metrics/metrics.go` | Prometheus collectors (spec §9.3) and `/metrics` server |
| `operator/reporter/policy/{policy,planned}.go` | Port of `policy.ts`; planned-message keys |
| `operator/reporter/solana/{accounts,events,mint,pda,logs}.go` | Anchor/Borsh/SPL decoders, discriminators, PDAs, program-log attribution |
| `operator/reporter/solana/{rpc,block}.go` | JSON-RPC client, streaming block decoder, writability rules |
| `operator/reporter/solana/evidence.go` | `Resolves`, status matching, per-slot evidence index |
| `operator/reporter/config/{config,load,resolve,template}.go` | Config struct, flags, viper loading, validation, TOML template |
| `operator/reporter/internal/simnode/...` | Test simulator: Solana JSON-RPC, CometBFT RPC, gRPC; scenarios; `cmd/simnode` |
| `operator/reporter/akash/{client,tracker,list}.go` | gRPC and CometBFT clients, height tracker, listing, audit cursor, broadcast and tx lookup |
| `operator/reporter/observe/observe.go` | Port of `SolanaObserver` |
| `operator/reporter/scan/scan.go` | Pass orchestration |
| `operator/reporter/signer/{signer,keyring,remote}.go`, `signer/kmspb/` | Signers and the vendored Cosmos-KMS proto with generated code |
| `operator/reporter/tx/{slots,builder,prune,fees,broadcaster,table}.go` | Nonce slots, unordered transaction building, simulation pruning, fees, broadcaster and slot table |
| `x/sink/sink_unordered_test.go` | Replay-key test on the real application (spec §11.3) |
| `cmd/akt-reporter/main.go`, `cmd/akt-reporter/cmd/*.go` | CLI |
| `operator/reporter/clitest/*_test.go` | CLI tests that need `simnode`. They live here because Go's `internal` rule keeps `cmd/` from importing it |
| `contracts/solana/reporter/goldens.ts` | Development-only TS golden exporter |
| `operator/reporter/testdata/goldens/*.jsonl` | Parity goldens |
| `x/sink/sink_reporter_load_test.go` | Node-load benchmark |
| `.goreleaser-reporter.yaml`, `_build/akt-reporter.Dockerfile`, `_build/akt-reporter-smoke.env`, `make/reporter.mk` | Build, image, smoke configuration |
| `.github/workflows/reporter-perf.yaml`, `.github/workflows/release-reporter.yaml` | Performance and release workflows (plus edits to `tests.yaml` and `release-operators.yaml`) |
| `_run/akt-reporter/*`, `contracts/solana/deploy/systemd/akt-reporter.service`, `contracts/solana/docs/*` | Deployment files and documentation |

---

# Milestone 1: preview reporter

At the end of Task 15, `akt-reporter scan` and `start` run in preview against real or simulated nodes, match the TS goldens, and meet the 100k benchmark.

### Task 1: Clock and transfer IDs

**Files:**
- Create: `operator/reporter/internal/clock/clock.go`, `operator/reporter/sinkid/id.go`, `operator/reporter/sinkid/record.go`
- Modify: `go.mod`, `go.sum` (`go mod tidy` direct/indirect moves only)
- Test: `operator/reporter/internal/clock/clock_test.go`, `operator/reporter/sinkid/id_test.go`, `operator/reporter/sinkid/record_test.go`

**Interfaces:**
- Produces:
  - `clock.Clock` (`Now() time.Time`, `After(time.Duration) <-chan time.Time`), `clock.Real() Clock`, `clock.NewFake(time.Time) *Fake`, `(*Fake).Advance(time.Duration)`
  - `sinkid.ID{Owner string; OwnerBytes []byte; Height int64; Sequence uint64}`
  - `sinkid.Parse(string) (ID, error)`, `sinkid.FromProto(sinkv1.ID) (ID, error)`, `(ID).String() string`, `(ID).Proto() sinkv1.ID`, `sinkid.Compare(a, b ID) int`, `(ID).KeySuffix() []byte`
  - `sinkid.Record{ID ID; Recipient solana.PublicKey; Amount uint64}`, `sinkid.DecodeRecord(sinkv1.TransferState) (Record, error)`, `sinkid.CoinAmount(sdk.Coin, positive bool) (sdkmath.Int, error)`

- [ ] **Step 1: Write the failing tests** (ports of `contracts/solana/shared/id.ts` and `shared/proof.ts` behavior)

```go
func TestParse(t *testing.T) {
	// valid: canonical lowercase akash owner of 20 bytes, "/1/1" and "/9223372036854775807/18446744073709551615"
	// invalid, each with the error "ID must be canonical owner/positive-int64-height/positive-uint64-sequence":
	//   "/01/1", "/+1/1", "/0/1", "/1/0", height 9223372036854775808, an uppercase owner, prefix "cosmos", a 0-byte owner
}
func TestParseLongOwner(t *testing.T) { // Review Focus 3: a 255-byte owner round-trips through Parse/String/Proto/KeySuffix
}
func TestCompare(t *testing.T) { // ordering: height, then sequence, then owner length, then owner bytes
}
func TestKeySuffix(t *testing.T) {
	// owner = 20 x 0x01, height 42, sequence 7
	// want = 0x14 || 20 x 0x01 || BE64(42) || BE64(7)
}
func TestDecodeRecord(t *testing.T) {
	// errors: "missing record data", "noncanonical recipient", "transfer amount exceeds uint64",
	//         "invalid uakt Coin", "amount must be positive"
}
func TestFakeClock(t *testing.T) { // After(5s) fires only after Advance(5s)
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/sinkid/... ./operator/reporter/internal/clock/...`. Expected: FAIL (undefined: Parse, ID, NewFake).

- [ ] **Step 3: Implement the produced API.** Use `github.com/cosmos/cosmos-sdk/types/bech32` (`DecodeAndConvert` accepts up to 1023 characters) and require the round trip to re-encode identically. Owners must be 1 to 255 bytes. Decimal components must match `^[1-9][0-9]*$`. `KeySuffix` is `[len(owner)] || owner || BE64(height) || BE64(sequence)`.

- [ ] **Step 4: Run the tests and confirm they pass.** Same command. Expected: PASS. Then run `go mod tidy`; expected: only `// indirect` markers change, for example on `github.com/gagliardetto/solana-go`.

- [ ] **Step 5: Commit** `feat(reporter): add transfer IDs, records and test clock`

### Task 2: Events and metrics

**Files:**
- Create: `operator/reporter/events/writer.go`, `operator/reporter/events/types.go`, `operator/reporter/metrics/metrics.go`
- Test: `operator/reporter/events/events_test.go`, `operator/reporter/metrics/metrics_test.go`

**Interfaces:**
- Consumes: `sinkid.ID` (Task 1).
- Produces:
  - `events.NewWriter(io.Writer) *Writer`; `(*Writer).Emit(v any)` (safe for concurrent use, one line per call); `(*Writer).Close() error`.
  - Types with exact JSON tags:
    - `Review{ID "id"; Mode "mode,omitempty"; Audit "audit"; Observed "observed"(any); Messages "messages"; Alerts "alerts"}`
    - `Absent{Kind "kind"; ContextSlot "contextSlot"; SlotLimit "slotLimit"}`
    - `Resolved{Kind; ContextSlot; Observation "observation"}`
    - `Observation{Outcome int32 "outcome"; Slot string "slot"; TransactionSignature string "transactionSignature"}`
    - `Message{TypeURL "typeUrl"; Value any "value"}`
    - `TxHash{ID "id"; TransactionHash "transactionHash"}`
    - `TransferError{ID "id"; Error "error"}`, `PassError{Error "error"}`
    - `LightClientChanged{Alert "alert"; Previous "previous"; Current "current"; Slot "slot"}`
    - `LightClientMismatch{Alert; Pinned "pinned"; Current "current"}`
    - `AuditUnavailable{Alert; Error "error"}`
    - `Summary{Scanned int "scanned"; Audited int "audited"; Index string "index"; Failures int "failures"; LightClientProgram string "lightClientProgram"}`
  - Helpers: `events.U64(uint64) string`, `events.ObservationOf(sinkv1.Observation) Observation`, and `events.MessageOf(sdk.Msg) (Message, error)` for `MsgReportOutcome`, `MsgVetoVoid` and `MsgFinalizeRefund`, using the field shapes in Appendix A.
  - `metrics.New(prometheus.Registerer) *Metrics`, with the collectors and labels of spec §9.3 as exported fields. Methods are nil-safe, so a nil `*Metrics` records nothing. `metrics.Serve(ctx, addr string, g prometheus.Gatherer) error`.

- [ ] **Step 1: Write the failing tests**

```go
func TestEventsMatchAppendixA(t *testing.T) {
	// Build each Appendix A event with concrete values (contextSlot 350000000, slotLimit 350000600, outcome 1,
	// slot 349999000, policyVersion 2, round 1, height 1200, sequence 4) and require.JSONEq against the
	// Appendix A line with the same values. Covers: the absent review, the resolved broadcast review with
	// MsgReportOutcome, the audit early exit (no "mode" key), transactionHash, the per-transfer error, the
	// three alerts, the pass error and the summary.
}
func TestEmptySlicesRenderAsArrays(t *testing.T) { // Review{} marshals "messages":[] and "alerts":[]
}
func TestWriterConcurrent(t *testing.T) { // 100 goroutines x 100 Emits: 10,000 lines, each valid JSON
}
func TestMetricsNilSafe(t *testing.T) { // calling every method on (*Metrics)(nil) does not panic
}
func TestMetricsRegistered(t *testing.T) { // gathering the registry yields every §9.3 name with exactly its listed labels
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/events/... ./operator/reporter/metrics/...`. Expected: FAIL.

- [ ] **Step 3: Implement.** The writer owns one goroutine fed by a buffered channel and writes through a `bufio.Writer`, flushing whenever the channel drains. `Close` drains and flushes.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `feat(reporter): add TS-compatible events and metrics`

### Task 3: Decision policy

**Files:**
- Create: `operator/reporter/policy/policy.go`, `operator/reporter/policy/planned.go`
- Test: `operator/reporter/policy/policy_test.go`

**Interfaces:**
- Consumes: `sinkid.ID`, `sinkid.DecodeRecord`, `sinkid.CoinAmount` (Task 1).
- Produces:
  - `policy.Snapshot{Params sinkv1.Params; Policy sinkv1.ReporterPolicy; RefundUsage sinkv1.EpochUsage; Epoch uint64; Time int64; RefundHead *sinkid.ID; Transfer sinkv1.TransferState}`
  - `policy.Kind` (`KindAbsent`, `KindResolved`); `policy.Observed{Kind Kind; ContextSlot uint64; SlotLimit *uint64; Observation sinkv1.Observation}`; `policy.Plan{Messages []sdk.Msg; Alerts []string}`
  - Alert constants `AlertFabricatedSlot`, `AlertNoStatus`, `AlertContradiction`, `AlertGovernanceReview`, holding the four Appendix A strings
  - Functions:
    - `ChallengeSeconds(sinkv1.SettlementData) (int64, error)`
    - `ReporterThreshold(sdkmath.LegacyDec, members int) (int, error)`
    - `ReportedObservations(sinkv1.TransferState) []sinkv1.Observation`
    - `MinimumSlot(sinkv1.TransferState, slotLimit *uint64) uint64`
    - `CompareOutcome(sinkv1.TransferState, sinkv1.Outcome) (contradicts, terminalMatches bool)`
    - `SettledConsistently(sinkv1.TransferState, sinkv1.Outcome) bool`
    - `PlanOutcome(reporter string, s Snapshot, o Observed) (Plan, error)`
  - `policy.MsgKind` with `MsgVote = 1`, `MsgVeto = 2`, `MsgRefund = 3`; these values are the kind bytes of the slot hash (spec §6.1)
  - `policy.Key{ID string; Kind MsgKind; Outcome sinkv1.Outcome; Round, PolicyVersion uint64}`; `policy.KeyOf(sinkid.ID, sdk.Msg) Key`
  - `policy.Planned{ID sinkid.ID; Msg sdk.Msg; Key Key; PlanHeight int64; PlanTime time.Time}`, where `PlanTime` is the block time of the snapshot the message was planned from

- [ ] **Step 1: Write the failing tests.** Port each of the 9 `test(...)` cases in `contracts/solana/reporter/policy.test.ts` as subtests of `TestPolicy`, using the TS titles, fixtures and assertions. Add:

```go
func TestPlanErrors(t *testing.T) {
	// verbatim errors: "invalid sink settlement", "account is not a current sink reporter",
	// "reporter quorum must exceed two thirds of the members", "current policy threshold mismatch",
	// "invalid plausible slot limit", "observation is behind reported evidence"
}
func TestVetoReason(t *testing.T) { // MsgVetoVoid.Reason == "No Solana status at finalized slot 123" for contextSlot 123
}
func TestKeyOf(t *testing.T) { // votes key on outcome, round and policy version; veto and refund keys ignore them
}
func TestNoReplanAfterExecution(t *testing.T) {
	// spec §6.1 policy lemma. Subtests: a recorded vote (same outcome, round and policy version); a vote that ran without
	// effect under HOLD_CONFLICT, and on a minted, a refund-pending and a held transfer; a refresh vote after which the
	// settlement's policy version is current; an executed veto (HOLD_VETO); an executed refund (STATUS_REFUNDED)
	// => PlanOutcome plans no message with that key in any later state of the same round
}
func TestVetoAfterGovernanceResolution(t *testing.T) {
	// a veto executed in round 1, then a resolution (round 2, HOLD_NONE, STATUS_REFUND_PENDING, new challenge end) and
	// still no Solana status => a veto is planned again; its KeyOf equals the round-1 veto's key (veto keys have no round)
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/policy/...`. Expected: FAIL.

- [ ] **Step 3: Implement** the functions as a line-by-line port of `policy.ts`. The threshold is `ceil(ratio × members)`, and `3 × threshold > 2 × members` is required unless `members == 0`. The ratio must lie in [0, 1]. Amounts use `sdkmath.Int`.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `feat(reporter): port the sink decision policy`

### Task 4: Solana decoders, PDAs and program logs

**Files:**
- Create: `operator/reporter/solana/accounts.go`, `events.go`, `mint.go`, `pda.go`, `logs.go`
- Test: `operator/reporter/solana/decode_test.go`, `operator/reporter/solana/logs_test.go`

**Interfaces:**
- Consumes: `sinkid.ID` (Task 1).
- Produces:
  - `DiscLaunchConfig`, `DiscBridgeConfig`, `DiscTransferStatus`, `DiscTransferResolved` (`[8]byte`)
  - `LaunchConfig{Version uint8; Initializer, Mint solana.PublicKey; BridgeEnabled bool; InitializedAtSlot uint64; Bump, MintAuthorityBump uint8}`
  - `BridgeConfig{LightClientProgram solana.PublicKey; ChainID string}`; the remaining fields are decoded but unused
  - `TransferID{Owner []byte; Height, Sequence uint64}`, `Minted{Amount uint64; Recipient solana.PublicKey}`, `VoidReason uint8` (`VoidInvalidRecipient`, `VoidGovernance`), `Outcome{Minted *Minted; Voided *VoidReason}`
  - `TransferStatus{ID TransferID; ProofHeight, ResolvedAtSlot uint64; Outcome Outcome}`; `TransferResolved` has the same fields
  - `Mint{MintAuthority *solana.PublicKey; Supply uint64; Decimals uint8; Initialized bool; FreezeAuthority *solana.PublicKey}`
  - `DecodeLaunchConfig`, `DecodeBridgeConfig`, `DecodeTransferStatus`, `DecodeMint` (each `func([]byte) (T, error)`; trailing bytes allowed except for the mint, which must be exactly 82 bytes); `DecodeTransferResolved([]byte) (TransferResolved, error)` (exact length)
  - `PDA(program solana.PublicKey, seeds ...[]byte) solana.PublicKey`, `StatusAddress(program solana.PublicKey, id sinkid.ID) solana.PublicKey`
  - `ProgramEvents(logs []string, program solana.PublicKey) ([]TransferResolved, error)`

- [ ] **Step 1: Write the failing tests**

```go
func TestDiscriminatorsMatchDeployedInterface(t *testing.T) {
	// read ../../../contracts/solana/tests/fixtures/deployed-interface.json; require the account
	// discriminators of LaunchConfig, BridgeConfig, TransferStatus and the event TransferResolved to equal the constants
}
func TestTransferStatusRoundTrip(t *testing.T) {
	// the vectors of state.rs status_and_event_retain_identity_and_resolution_evidence: owner 20 x 0x02,
	// height 9223372036854775807, sequence 18446744073709551615, outcomes Minted / Voided{InvalidRecipient} /
	// Voided{Governance}; the status is zero-padded to 340 bytes and still decodes
}
func TestTransferResolvedStrict(t *testing.T) { // one trailing byte, or outcome tag 2, or void tag 2: error
}
func TestDecodeConfigAndMint(t *testing.T) {
	// LaunchConfig at 84 bytes and BridgeConfig at 656 bytes, both padded, decode;
	// a mint COption tag of 2 is an error; mint data of 81 or 83 bytes is an error
}
func TestStatusAddressSeeds(t *testing.T) {
	// StatusAddress == FindProgramAddress(["transfer", sha256(owner), LE64(height), LE64(sequence)])
}
func TestProgramEvents(t *testing.T) {
	// subtests port the log arrays and outcomes of solana.test.ts:
	// "events emitted inside a CPI count only from an AKT program frame",
	// "truncated or unbalanced logs never supply evidence",
	// "log truncation keeps only top-level AKT frames that already succeeded";
	// verbatim errors "inconsistent truncated logs" and "inconsistent program log stack"
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/solana/... -run 'Disc|Round|Strict|Decode|Seeds|ProgramEvents'`. Expected: FAIL.

- [ ] **Step 3: Implement.** Borsh is little-endian, and `Vec<u8>` is a u32 length followed by bytes, with the owner limited to 1 to 255. `COption` uses a u32 tag where 0 is none, 1 is some, and anything else is an error. Port `programEvents` from `solana.ts:99-141` using the same two regexes. Decode only `Program data:` lines whose 8-byte discriminator is `DiscTransferResolved`; a decode failure is an error (§5.4). Ignore other discriminators (§5.6 intentional difference 5).

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `feat(reporter): add AKT account, event and log decoders`

### Task 5: Solana JSON-RPC client and streaming block decoder

**Files:**
- Create: `operator/reporter/solana/rpc.go`, `operator/reporter/solana/block.go`
- Create: `operator/reporter/solana/testdata/getblock_v1.json`, `operator/reporter/solana/testdata/getblock_v1.provenance.txt`
- Test: `operator/reporter/solana/rpc_test.go`

**Interfaces:**
- Consumes: `metrics.Metrics` (Task 2).
- Produces:
  - `RPCOptions{URL string; Timeout time.Duration; Concurrency, BlockConcurrency int; Metrics *metrics.Metrics}`, `NewRPC(RPCOptions) *RPC`
  - `ErrUnavailable` (text `Solana node is temporarily unavailable`), `ErrBlockUnavailable`
  - `Account{Owner solana.PublicKey; Executable bool; Data []byte}`
  - `Tx{Slot uint64; Version string; Signatures []string; Failed bool; Logs []string; HasLogs bool; Keys []solana.PublicKey; Writable []bool}`
  - Methods on `*RPC`:
    - `GenesisHash(ctx) (string, error)`
    - `MultipleAccounts(ctx, keys []solana.PublicKey) (contextSlot uint64, accts []*Account, err error)` (at most 100 keys)
    - `BlockTime(ctx, slot uint64) (*int64, error)` (cached per slot in an LRU of 4,096)
    - `Transaction(ctx, sig string) (*Tx, error)` (`nil, nil` when not found)
    - `StreamBlock(ctx, slot uint64, each func(Tx) error) error`

- [ ] **Step 1: Capture the v1 fixture.** Keep it isolated from the project's Solana toolchain; nothing here touches the repository except the two `testdata` files.
  1. Download `https://github.com/anza-xyz/agave/releases/download/v4.2.2/solana-release-<target>.tar.bz2` (target `x86_64-unknown-linux-gnu` or `aarch64-apple-darwin`) and extract it into `.cache/agave-4.2.2/`.
  2. Start the validator: `.cache/agave-4.2.2/bin/solana-test-validator --reset --ledger .cache/agave-4.2.2/ledger --rpc-port 18899 --quiet`.
  3. In a throwaway module outside the repository (`mktemp -d`, then `go mod init capture`, then `go get github.com/solana-foundation/solana-go/v2@v2.1.0`), write a program that airdrops to a fresh key, sends one System Program transfer built with the `solana.TransactionV1Config` option, waits for finalization, and prints the `getBlock` result for that slot (`encoding: json`, `maxSupportedTransactionVersion: 1`, `transactionDetails: full`, `rewards: false`).
  4. Save the result as `testdata/getblock_v1.json`. Record the Agave version, the solana-go version and the capture date in `getblock_v1.provenance.txt`.

  Expected: the block contains a transaction with `"version": 1`. If the v4.2.2 download is unavailable, stop and report: this fixture is the spec's mitigation for the v1 JSON shape (§14).

- [ ] **Step 2: Write the failing tests**, against an `httptest.Server` that returns canned JSON-RPC.

```go
func TestWritability(t *testing.T) {
	// legacy and v0 (with meta.loadedAddresses) and the v1 fixture: Writable follows the header counts over
	// static keys, then loadedAddresses.writable, then loadedAddresses.readonly
}
func TestStreamBlockOrderAndFields(t *testing.T) {
	// a block of 3 transactions (failed; no logs; success): `each` sees 3 Tx in order with Failed / HasLogs set
}
func TestUnknownVersionSuppliesNoEvidence(t *testing.T) { // version 2 => Tx.Writable is all false
}
func TestErrors(t *testing.T) {
	// an RPC error from getMultipleAccounts => ErrUnavailable;
	// a getBlock error or a null result => ErrBlockUnavailable;
	// 101 keys => error before any request
}
func TestBlockTimeCached(t *testing.T) { // two BlockTime(7) calls => one request; a null result is returned as nil
}
```

- [ ] **Step 3: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/solana/... -run 'Writability|Stream|Unknown|Errors|BlockTime'`. Expected: FAIL.

- [ ] **Step 4: Implement.** Use `net/http` with a keep-alive `Transport` sized to the concurrency, and `golang.org/x/sync/semaphore` for the two limits. `StreamBlock` walks `result.transactions` with `json.Decoder.Token()` and decodes one element at a time.

- [ ] **Step 5: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 6: Commit** `feat(reporter): add Solana JSON-RPC client and streaming block decoder`

### Task 6: Evidence index

**Files:**
- Create: `operator/reporter/solana/evidence.go`
- Test: `operator/reporter/solana/evidence_test.go`

**Interfaces:**
- Consumes: `RPC`, `Tx`, `ProgramEvents`, `StatusAddress`, the decoders (Tasks 4 and 5).
- Produces:
  - `Resolves(tx Tx, program, address solana.PublicKey, match func(TransferResolved) bool) bool`
  - `MatchesStatus(ev TransferResolved, st TransferStatus) bool`
  - `NewEvidenceIndex(rpc *RPC, program solana.PublicKey, maxSlots int, maxBytes int64) *EvidenceIndex`
  - `(*EvidenceIndex).Lookup(ctx, slot uint64, address solana.PublicKey, st TransferStatus) (solana.Signature, error)`
  - `ErrBlockMissing` (`resolution evidence unavailable: block missing from owned node history`), `ErrNotInBlock` (`resolution evidence unavailable in its block`), `ErrMultipleInBlock` (`multiple resolution transactions in block`)
  - The CLI constructs it with `maxSlots` 256 and `maxBytes` 64 MiB.

- [ ] **Step 1: Write the failing tests**

```go
func TestEvidenceFromSolanaTests(t *testing.T) {
	// subtests port solana.test.ts: "observer chooses the successful resolution over a failed replay",
	// "a version 1 resolution transaction is evidence", "spam referencing the status account cannot hide the resolution",
	// "two resolutions for one status fail closed",
	// "resolution events must match identity, proof height, transaction slot and void reason"
}
func TestIndexEvictsOldestSlot(t *testing.T) {
	// maxSlots 2: look up slot 1, slot 2, then slot 1 again (no fetch), then slot 3 => slot 1 is evicted even though
	// it was used last; a later lookup of slot 1 fetches it again (4 getBlock calls in total)
}
func TestIndexByteBudget(t *testing.T) { // maxBytes holds one slot's entries: adding a higher slot evicts the lower one
}
func TestConcurrentLookupsShareFetch(t *testing.T) { // 10 goroutines on one slot => one getBlock call
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/solana/... -run 'Evidence|Index|Concurrent'`. Expected: FAIL.

- [ ] **Step 3: Implement.**
  - **Index entries** (§5.4): for each candidate event, keep the event, the transaction's first signature, and whether the transaction writes `StatusAddress(program, event.ID)`.
  - **Lookup:** collect the distinct matching transactions.
  - **Fetches:** concurrent fetches of one slot share a single request (`singleflight`).
  - **Eviction:** evict the numerically lowest slot first, whenever the slot count or the approximate byte total (the sum of entry sizes) is over budget. This is not LRU.
  - **Signature:** the returned signature must decode to 64 bytes, not all zero.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `feat(reporter): add per-slot resolution evidence index`

### Task 7: Configuration

**Files:**
- Create: `operator/reporter/config/config.go`, `load.go`, `resolve.go`, `template.go`
- Test: `operator/reporter/config/config_test.go`

**Interfaces:**
- Consumes: the `solana` PDA helper (Task 4).
- Produces:
  - `config.Config`, whose fields mirror spec §8.3, with sub-structs `Solana`, `Scan`, `Tx`, `Signer.Remote` and `Metrics`. Durations are `time.Duration`; `Gas`, `GasPrices`, `Fees` and `Tx.MaxFee` are strings.
  - `config.Default() Config`, `config.AddFlags(*pflag.FlagSet)` (one flag per key, with dotted names such as `--solana.rpc`), `config.Known() []string`
  - `config.Load(fs *pflag.FlagSet, environ []string) (Config, []string /*warnings*/, error)`
  - `config.Resolved{Config; ProgramID, Mint solana.PublicKey; LightClientPin *solana.PublicKey; Reporter sdk.AccAddress; SlotLead uint64; GasPerMessage uint64 /*0 = auto*/; GasPrices sdk.DecCoins; Fees sdk.Coins; MaxFee sdk.Coin}`
  - `config.Resolve(Config) (Resolved, error)`, which returns every broken rule joined with `errors.Join`
  - `config.Render(Config) ([]byte, error)`
  - Constants `MainnetChainID`, `MainnetGenesis`, `DevProgramID`

- [ ] **Step 1: Write the failing tests**

```go
func TestDefaults(t *testing.T) { // every default in spec §8.3, for example Scan.Workers == 16, Tx.MaxMsgs == 25; no tx.timeout or tx.batch-window key
}
func TestPrecedence(t *testing.T) { // for scan.workers: flag 9 > env 8 > file 7 > default 16
}
func TestUnknownFileKeyFails(t *testing.T) { // reporter.toml with "scan.wrokers = 3" => error naming the key
}
func TestUnknownEnvWarns(t *testing.T) { // AKT_REPORTER_SERVICE_HOST=1 => one warning, no error (Review Focus 5)
}
func TestResolveRules(t *testing.T) {
	// table: each §8.3 bound and format rule, the mint PDA rule, the network guard for mainnet and testnet,
	// and credentials in node or solana.rpc URLs
}
func TestBroadcastNeedsExactlyOneSigner(t *testing.T) { // neither or both of from / signer.remote.address => error (Review Focus 5)
}
func TestBroadcastAgeBudget(t *testing.T) {
	// spec §6.3: broadcast with signer.remote.timeout 10s => max-age 70s is valid, 71s is an error naming both keys;
	// with the keyring signer, max-age 80s is valid and 81s is not; preview ignores the rule
}
func TestSlotLeadDerived(t *testing.T) { // max-slot-lead 0 and max-age 60s => SlotLead 600
}
func TestRenderRoundTrip(t *testing.T) { // Load(Render(c)) == c for Default() with required keys filled in
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/config/...`. Expected: FAIL.

- [ ] **Step 3: Implement.**
  - `Load` uses a private `viper.Viper`: `SetEnvPrefix("AKT_REPORTER")`, `SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))`, `AutomaticEnv`.
  - It reads `<home>/config/reporter.toml` when present and applies the precedence with a copy of chain-sdk's `bindFlags` (`chain-sdk/go/cli/configs.go:269`).
  - Unknown file keys are found by reading the file into a separate viper and comparing `AllKeys()` with `Known()`.
  - `Render` uses `text/template` with one comment per key.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `feat(reporter): add akash-style configuration`

### Task 8: Test simulator, Solana side

**Files:**
- Create: `operator/reporter/internal/simnode/scenario.go`, `scenarios.go`, `solana.go`, `encode.go`, `node.go`
- Test: `operator/reporter/internal/simnode/solana_test.go`

**Interfaces:**
- Consumes: the `solana` discriminators and `StatusAddress` (Task 4).
- Produces:
  - `simnode.Scenario`: a JSON-serializable chain state.
    - **Solana part:** genesis hash; slots and block times; program, config, bridge, mint and status accounts; blocks and transactions with logs; failures to inject.
    - **Akash part:** added in Task 9.
    - `Passes int`: how many passes the goldens run; default 1.
    - All times are offsets from start. `Start` rebases them to `Options.Clock.Now()`, so freshness checks pass in both the Go tests and the TS golden export.
  - `simnode.Options{Clock clock.Clock; AkashLatency, SolanaLatency, BlockFetchLatency, BlockInterval time.Duration}`
  - `simnode.Start(ctx, Scenario, Options) (*Node, error)`; `Node{CometURL, GRPCAddr, SolanaURL string}`; `(*Node).Close()`; `(*Node).Calls(method string) int`; `(*Node).Mutate(func(*Scenario))`
  - `simnode.Scenarios() map[string]func() Scenario`, defined in `scenarios.go`. It starts empty here; later tasks add named entries to the same file.
  - Encoders in `encode.go` for the AKT accounts and the `TransferResolved` event. They are an independent test oracle and must not call the production decoders.

- [ ] **Step 1: Write the failing tests**

```go
func TestSolanaMethods(t *testing.T) {
	// getGenesisHash, getMultipleAccounts (base64; context.slot), getBlockTime (null for unknown slots),
	// getBlock and getTransaction (json, version honored) answer from the scenario; decoding the encoded
	// accounts with the production decoders round-trips
}
func TestGetMultipleAccountsLimit(t *testing.T) { // 101 keys => JSON-RPC error, as Agave does
}
func TestLatencyInjection(t *testing.T) { // SolanaLatency 5ms => each request takes at least 5ms
}
func TestTimesRebased(t *testing.T) { // a block time offset of -5s reads as Clock.Now()-5s after Start
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/internal/simnode/...`. Expected: FAIL.

- [ ] **Step 3: Implement** the HTTP JSON-RPC handler over the scenario with per-method call counters. Each server listens on `127.0.0.1:0`.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `test(reporter): add Solana side of the chain simulator`

### Task 9: Test simulator, Akash side

**Files:**
- Create: `operator/reporter/internal/simnode/akash.go`, `comet.go`, `grpc.go`
- Modify: `operator/reporter/internal/simnode/scenarios.go`
- Test: `operator/reporter/internal/simnode/akash_test.go`

**Interfaces:**
- Consumes: chain-sdk `sinkv1` types and QueryServer; cosmos-sdk `authtypes`, `txtypes` and `cmtservice` server interfaces.
- Produces:
  - The Akash part of `Scenario`:
    - chain ID, node ID, tx index on or off;
    - heights with block times, the application last height;
    - params, policies, sink state, transfers per height, accounts;
    - per-message simulation failures, and an option to echo a different height;
    - broadcast injection (§6.5): per broadcast attempt, a transport error before admission, admission followed by a lost response, a `tx already exists in cache` JSON-RPC error, a `failed to add unordered nonce` rejection, or a CheckTx rejection with a given code, codespace and log;
    - `UnorderedDisabled bool`, which rejects every transaction with `unordered transactions are not enabled`;
    - per-transaction delivery codes.
  - CometBFT JSON-RPC endpoints: `status`, `abci_info`, `block`, `abci_query`, `broadcast_tx_sync`, `tx`. `abci_query` routes `/akash.sink.v1.Query/*` and `/cosmos.auth.v1beta1.Query/Account`, and answers a missing transfer with code 22, codespace `sdk`.
  - A gRPC server with the gogoproto codec: sink `Query`, auth `Account`, tx `Simulate`, tendermint `GetNodeInfo`. It applies the `x-cosmos-block-height` semantics of `baseapp/grpcserver.go:50-77`.
  - `(*Node).CommitBlock()` (includes pending transactions and advances the height and time), `(*Node).Broadcasts() [][]byte`, `(*Node).Halt()`
  - The chain's execution model (spec §2.2), which the no-repeat tests rely on:
    - **Admission.** `broadcast_tx_sync` answers bytes identical to a pending or included transaction with the JSON-RPC error `tx already exists in cache`, as CometBFT does before CheckTx. It refuses a transaction whose (signer, `timeout_timestamp`) is held by a pending or committed transaction, with code 18, codespace `sdk` and log `failed to add unordered nonce: …`. It also refuses:
      - a timeout before the latest block time, with `unordered transaction has a timeout_timestamp that has already passed`;
      - a timeout more than 10 min after the latest block time, with `unordered tx ttl exceeds 10m0s`.
    - **`(*Node).Gossip(txBytes []byte)`** adds a transaction to the pending pool without admission, as if another node had accepted it.
    - **`CommitBlock` order:**
      1. Drop replay keys whose timeout is at or before the new block's time. A block's time never equals the timeout of a held or pending replay key: the simulator moves it 1 ns later, so its tests stay clear of the SDK boundary that `TestUnorderedReplayKeyBoundary` covers.
      2. Drop expired pending transactions.
      3. Execute the rest in arrival order:
         - a transaction whose replay key is held is included with code 18 and that log, with no effect and no fee;
         - otherwise its key is stored and its fee charged, and its messages are applied unless an injected delivery code fails it.
      4. Drop pending transactions whose replay key is now held, as mempool recheck does.
    - **Applying messages:**
      - a vote is appended to `settlement.votes`, or is a code-0 no-op when this reporter already voted (`x/sink/keeper/settlement.go:135-138`);
      - a veto sets `HOLD_VETO`;
      - a refund sets `STATUS_REFUNDED` and advances the refund head.

      Sink state is versioned per height, so a height-pinned read sees the state after that height's block.
    - **`(*Node).Executions() map[string]int`:** code-0 executions, duplicate no-ops included, keyed the way spec §6.1 counts them:
      - a vote: `<type URL>|<id>|<outcome>|<round>|<policy version>`;
      - a veto: `<type URL>|<id>|<settlement round at execution>`;
      - a refund: `<type URL>|<id>`.
    - **`(*Node).Resolve(id sinkid.ID, status sinkv1.Status)`:** a governance resolution, applied as `x/sink/keeper/settlement.go:296-362` does. It increments the round; clears the votes, the hold and the veto fields; sets the status; and sets a new challenge end for `STATUS_REFUND_PENDING`.
    - **`(*Node).SetLag(blocks int64)`:** `status`, `abci_info`, `block` and unpinned gRPC reads report the height `blocks` below the latest, like a lagging node behind a load balancer.
    - **`Options.AlignWindow bool`:**
      - `Start` rebases chain time so the latest block time lies in the first 60 s of a Unix-aligned 2-minute window and 0 to 70 s behind `Options.Clock.Now()`. Block times then advance with the clock, keeping that offset.
      - CLI tests set it together with `max-age = "80s"` and a `BlockInterval` of at most 5 s. Block age then stays under 80 s, and a run never starts inside a send cutoff.
  - Named scenarios in `scenarios.go`:
    - `clean`: open transfers that plan nothing;
    - `stale-akash`: the latest block is older than `max-age`;
    - `votes`: 30 resolved, void-requested transfers that each plan one `MsgReportOutcome`.

- [ ] **Step 1: Write the failing tests**

```go
func TestHeightHeaderEcho(t *testing.T) { // header H => answer at H with echo H; no header => latest height echoed
}
func TestABCIQueryNotFound(t *testing.T) { // Transfer of an unknown ID via abci_query => code 22, codespace "sdk"
}
func TestGRPCNotFound(t *testing.T) { // the same via gRPC => codes.NotFound
}
func TestSimulateMessageIndex(t *testing.T) { // a failure injected on message 2 => error containing "message index: 2"
}
func TestCommitBlockIncludesBroadcasts(t *testing.T) { // broadcast_tx_sync then CommitBlock => tx(hash) returns the result
}
func TestBroadcastInjection(t *testing.T) { // each injected outcome is returned for its attempt; UnorderedDisabled rejects every tx
}
func TestNamedScenariosStart(t *testing.T) { // clean, stale-akash and votes all Start without error
}
func TestReplayKeyRule(t *testing.T) {
	// two txs from one signer with one timeout: broadcast refuses the second with code 18 "failed to add unordered nonce";
	// the second Gossiped into the same block is included with code 18 and no effect; after a block whose time passed the
	// timeout, a third tx with it is refused as expired; Executions counts one execution; re-broadcasting identical
	// bytes => the JSON-RPC error "tx already exists in cache"
}
func TestMessageEffectsVersioned(t *testing.T) {
	// an executed vote is visible at height h+1 and not at h; a duplicate vote executes as a code-0 no-op and Executions
	// counts 2; a veto sets HOLD_VETO; a refund sets STATUS_REFUNDED and moves the refund head; Resolve starts round 2
	// with HOLD_NONE
}
func TestLagAndAlign(t *testing.T) {
	// SetLag(2) => status reports latest-2; AlignWindow => latest block time mod 2m < 60s and 0-70s behind the clock
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/internal/simnode/...`. Expected: FAIL.

- [ ] **Step 3: Implement.** Use `github.com/cometbft/cometbft/rpc/jsonrpc/server` for the JSON-RPC surface and `grpc.NewServer(grpc.ForceServerCodec(codec.NewProtoCodec(reg).GRPCCodec()))` for gRPC. Decode broadcast transactions with the SDK `TxConfig` so that tests can assert on them. Copy the sink state per height (copy-on-write per transfer is enough). Scenarios stay small, and `steady-100k` executes nothing.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `test(reporter): add Akash side of the chain simulator`

### Task 10: Akash client

**Files:**
- Create: `operator/reporter/akash/client.go`, `tracker.go`, `list.go`
- Test: `operator/reporter/akash/akash_test.go`

**Interfaces:**
- Consumes: `sinkid`, `clock`, `metrics`, `simnode` (tests).
- Produces:
  - `akash.Options{Node, GRPCAddr string; GRPCInsecure bool; ChainID string; MaxAge, RequestTimeout time.Duration; Concurrency int; MinRefresh time.Duration /*default 1s*/; Clock clock.Clock; Metrics *metrics.Metrics}`
  - `akash.Dial(ctx, Options) (*Client, error)`
  - `akash.State{Height int64; Time time.Time; Params sinkv1.Params; Policy sinkv1.ReporterPolicy; RefundUsage, SendUsage sinkv1.EpochUsage; Totals sinkv1.Totals; Epoch uint64; RefundHead *sinkid.ID}`
  - Methods on `*Client`:
    - `Latest(ctx) (*State, error)`
    - `Transfer(ctx, height int64, id sinkid.ID) (sinkv1.TransferState, error)` (`ErrRecordNotFound` = `Sink record not found`)
    - `ListOpen(ctx, height int64, pageSize int) ([]sinkid.ID, error)`
    - `Audit(ctx, height int64, after *AuditCursor, limit int) (AuditPage, error)`
    - `AccountNumber(ctx, sdk.AccAddress) (uint64, error)` (`reporter account not found`)
    - `Simulate(ctx, txBytes []byte) (gasUsed uint64, err error)`
    - `BroadcastSync(ctx, txBytes []byte) (BroadcastResult, error)`
    - `Tx(ctx, hash []byte) (*TxResult, error)` (`nil, nil` when not found)
    - `TxIndexEnabled() bool`
  - `AuditCursor{Kind CursorKind /*CursorNext, CursorTail*/; Key []byte; LastID sinkid.ID}`, `AuditPage{IDs []sinkid.ID; Cursor *AuditCursor; HasMore bool}`
  - `BroadcastResult{Code uint32; Codespace, Log string; Hash []byte; InCache bool}`, `TxResult{Height int64; Code uint32; Codespace, Log string}`. `BroadcastSync` turns the JSON-RPC error `tx already exists in cache` into `InCache: true` with no error (spec §6.5).

- [ ] **Step 1: Write the failing tests** against `simnode`.

```go
func TestAkashFromTS(t *testing.T) {
	// subtests port akash.test.ts and the paging and audit tests of scan.test.ts:
	// "chronological discovery reuses the server's last-page cursor and finds later earlier-sorting owners",
	// "native pages preserve fixed height, reject duplicates, stalled cursors and pruned state",
	// "fixed-height listings return every record past the old 100,000 bound";
	// every TS error text listed for the tracker and listing in spec §5.3 appears verbatim
}
func TestDialRejectsDifferentNodes(t *testing.T) { // node IDs differ => "Akash RPC and gRPC endpoints are different nodes"
}
func TestTrackerRefreshAtMostOncePerInterval(t *testing.T) { // 100 concurrent Latest => 1 status call
}
func TestTrackerCachesFailure(t *testing.T) { // a stale node: 2 calls within MinRefresh => 1 status call, both fail
}
func TestTrackerAcceptsLowerHeight(t *testing.T) { // Review Focus 1: H=10 then H=9 => the second State has height 9 and passes all checks
}
func TestFreshnessWindow(t *testing.T) { // Review Focus 4: block time now+10s accepted, now+11s and now-61s rejected
}
func TestHeightEchoMismatch(t *testing.T) { // simnode echoes H+1 => "Akash query height mismatch"
}
func TestMergeOrder(t *testing.T) {
	// one ID both disputed and refund-pending keeps its dispute position; order is disputes,
	// refund-pending, void-requested, transfer-pending
}
func TestPlaintextForLoopback(t *testing.T) { // grpc-insecure false with 127.0.0.1 connects in plaintext
}
func TestBroadcastInCache(t *testing.T) { // identical bytes broadcast twice => the second BroadcastResult has InCache true and no error
}
func TestTxResultCodespace(t *testing.T) { // a delivered tx with codespace "sdk" and code 18 => TxResult carries both; code 18 under another codespace stays distinct
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/akash/...`. Expected: FAIL.

- [ ] **Step 3: Implement.**
  - **gRPC:** `grpc.NewClient` with the SDK gogoproto codec and a 64 MiB receive limit. Credentials are plaintext for loopback hosts and when `GRPCInsecure` is set, and TLS 1.2 or later otherwise.
  - **Height pinning:** each sink read sets `x-cosmos-block-height` and checks the echo through `grpc.Header`.
  - **CometBFT:** `rpc/client/http` with a tuned `http.Transport`.
  - **Tracker:** the refresh sequence of spec §5.3 behind `singleflight`, caching the snapshot or the error for `MinRefresh`.
  - **Listing:** the four sources concurrently, each paging sequentially, merged in priority order.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `feat(reporter): add height-pinned Akash client`

### Task 11: Solana observer

**Files:**
- Create: `operator/reporter/observe/observe.go`
- Test: `operator/reporter/observe/observe_test.go`

**Interfaces:**
- Consumes: `solana.RPC`, `EvidenceIndex`, the decoders, `Resolves` and `MatchesStatus`; `policy.Observed`; `sinkid.Record`; `clock`.
- Produces:
  - `observe.Options{GenesisHash string; ProgramID, Mint solana.PublicKey; LightClientPin *solana.PublicKey; ChainID string; MaxAge time.Duration; SlotLead uint64; Clock clock.Clock}`
  - `observe.Change{Previous, Current solana.PublicKey; ContextSlot uint64}`
  - `observe.MismatchError{Pinned, Current solana.PublicKey}`, whose `Error()` is `bridge light client <current> differs from pinned <pinned>`
  - `observe.New(rpc *solana.RPC, idx *solana.EvidenceIndex, o Options, onChange func(Change)) *Observer`. `onChange` is called under the follow lock.
  - `observe.Deployment{ContextSlot uint64; BridgeEnabled bool; LightClient solana.PublicKey}`; `(*Observer).Deployment(ctx) (Deployment, error)`
  - `(*Observer).Chunk(ctx, ids []sinkid.ID) (*Chunk, error)`
  - `observe.Finalized{Kind policy.Kind; ContextSlot uint64; Address solana.PublicKey; Status *solana.TransferStatus; Outcome sinkv1.Outcome; Slot uint64}`; `(*Observer).Finalized(*Chunk, sinkid.Record) (Finalized, error)`
  - `(*Observer).Observe(ctx, *Chunk, sinkid.Record, reported []sinkv1.Observation, own *sinkv1.Observation) (policy.Observed, error)`

- [ ] **Step 1: Write the failing tests** against `simnode`.

```go
func TestObserverFromTS(t *testing.T) {
	// subtests port the remaining solana.test.ts cases: deployment and ownership checks, the absent path,
	// fabricated, lagging and stale slots, reported-signature checks, own-evidence reuse,
	// uint64 precision, the pinned and following light client
}
func TestChunkUsesOneAccountsCall(t *testing.T) { // 96 IDs => 1 getMultipleAccounts call of 100 keys
}
func TestNullBlockTimeIsStale(t *testing.T) { // => "stale Solana finalized state"
}
func TestFollowRuleOutOfOrder(t *testing.T) {
	// Review Focus 2: observations (slot, program) applied as
	// (110,A) (105,B) (108,A) (120,B) (130,B) (125,A) (140,A):
	// onChange fires twice, A->B at 120 and B->A at 140; the older (105,B) (108,A) (125,A) are ignored
}
func TestOwnSignatureWithZeroBytes(t *testing.T) { // a 64-byte own signature containing zero bytes, not all zero, is reused
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/observe/...`. Expected: FAIL.

- [ ] **Step 3: Implement** a port of `solana.ts:153-538` following spec §5.4. The age check runs when the chunk snapshot is taken. The follow state (program and highest slot) is updated under one mutex, which is also held while `onChange` runs.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `feat(reporter): port the Solana observer`

### Task 12: Pass orchestration

**Files:**
- Create: `operator/reporter/scan/scan.go`
- Test: `operator/reporter/scan/scan_test.go`

**Interfaces:**
- Consumes: `akash.Client`, `observe.Observer`, `events.Writer`, `policy.PlanOutcome`, `policy.Planned` and `KeyOf`, `sinkid.ID`, `metrics`.
- Produces:
  - `scan.Queuer` interface: `Queue(policy.Planned) bool`. Every `Planned` carries the chunk snapshot's height and block time.
  - `scan.Options{Reporter string; Broadcast bool; Workers, PageSize, AuditPerScan int}`
  - `scan.New(a *akash.Client, o *observe.Observer, ev *events.Writer, opts Options, q Queuer) *Scanner`, where `q` is nil in preview
  - `(*Scanner).Pass(ctx context.Context, settle func() int) (failures int)`. `ctx` bounds the reviews. Once they end (finished, cancelled or aborted), `Pass` calls `settle` exactly once, if it is non-nil, and adds its result to the failures. Only then does it write the summary, and only when the pass reached the review phase. The caller decides what `settle` waits for (Task 21).
  - `(*Scanner).ReviewRefundHead(ctx, minHeight int64) error` and `(*Scanner).Review(ctx, ids []sinkid.ID, minHeight int64) error`, the follow-up reviews of spec §6.6, used by Tasks 19 and 20.
    - Both take `Latest` until its height is at least `minHeight`, retrying at the tracker's refresh rate until `ctx` ends.
    - `Review` then re-reads the IDs at that height in chunks of up to 96, writes the normal review events, and queues what it plans. IDs that are no longer open are skipped.
  - `(*Scanner).OnChange(observe.Change)`, which writes `light-client-changed` and is passed to `observe.New`

- [ ] **Step 1: Write the failing tests** against `simnode`.

```go
func TestScanFromTS(t *testing.T) {
	// subtests port scan.test.ts: "all held terminal disputes are reviewed first and reread before decisions",
	// "pruned queries and light-client pin mismatches stop the scan without fallback",
	// "every open transfer past the old bounds is reviewed, disputes and refunds first",
	// "an audit query failure still reviews open transfers and retries its window",
	// "broadcast scans do not pay for bare void orders, frozen same-outcome votes or existing conflicts"
	// (asserting planned messages through a fake Queuer)
}
func TestOrderingGuarantees(t *testing.T) {
	// the summary is the last line; a light-client-changed alert precedes every review decided under the new client
}
func TestSettleBeforeSummary(t *testing.T) {
	// settle returning 3 => the summary's failures include the 3; events written by settle precede the summary
}
func TestPinMismatchStopsAfterRunningChunks(t *testing.T) {
	// the alert and error are the final lines; no review follows them; no summary; settle still runs once
}
func TestChunkErrorsArePerTransfer(t *testing.T) {
	// Solana down for one chunk => 96 {id,error} lines, failures == 96, and the next chunk is reviewed
}
func TestAuditEarlyExitEvent(t *testing.T) { // {"id":...,"audit":true,"observed":"resolved","messages":[],"alerts":[]}
}
func TestCancelledPassReportsListedCount(t *testing.T) { // cancelled mid-pass => the summary's scanned == number listed
}
func TestPlannedCarriesSnapshot(t *testing.T) { // every queued Planned has PlanHeight == H′ and PlanTime == the block time of its chunk's snapshot
}
func TestReviewIDs(t *testing.T) {
	// Review(ctx, 3 IDs, 0) => one fresh Latest, 3 review events, their planned messages queued with that snapshot;
	// an ID that is no longer open writes no event
}
func TestReviewWaitsForHeight(t *testing.T) {
	// Review Focus 1: simnode SetLag(1) at height 20, Review(ctx, ids, 20) => no review until the lag clears, then one at
	// height 20; a ctx that ends first => ctx's error and no event
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/scan/...`. Expected: FAIL.

- [ ] **Step 3: Implement** a port of `scan.ts` following spec §5.2 and §5.6. Chunks of 96 go through a bounded channel to `Workers` goroutines. Each chunk takes `a.Latest`, re-reads its transfers at H′, takes `o.Chunk`, then reviews each transfer. Write the review event before queuing messages. The audit cursor lives on the `Scanner`.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `feat(reporter): add concurrent pass orchestration`

### Task 13: Preview CLI

**Files:**
- Create: `cmd/akt-reporter/main.go`, `cmd/akt-reporter/cmd/root.go`, `init.go`, `check.go`, `run.go`, `keys.go`, `version.go`
- Test: `cmd/akt-reporter/cmd/cmd_test.go` (no simulator), `operator/reporter/clitest/preview_test.go` (with `simnode`)

**Interfaces:**
- Consumes: everything above.
- Produces:
  - `cmd.NewRootCmd() *cobra.Command`
  - `cmd.Execute(*cobra.Command) int`, returning the exit code. `main` calls `os.Exit(cmd.Execute(cmd.NewRootCmd()))`.
  - In preview, `run.go` builds the clients, observer, scanner and the metrics listener (when `metrics.listen` is set). `start` loops `Pass(ctx, nil)` then `scan.interval`; `scan` runs one pass. Task 21 adds broadcast.

- [ ] **Step 1: Write the failing tests**

```go
// cmd/akt-reporter/cmd/cmd_test.go
func TestCheckPrintsValid(t *testing.T) { // stdout == "Configuration valid. No RPC request or transaction sent.\n"; no network access
}
func TestInitWritesAndRefusesOverwrite(t *testing.T) {
	// the second init fails; init --overwrite succeeds; a fresh init writes the file and prints the rules
	// it does not yet satisfy (for example the required reporter)
}
func TestKeysUseConfiguredBackend(t *testing.T) {
	// Review Focus 5: `keys add r` with no backend flag creates <home>/keyring-test; an explicit
	// --keyring-backend file wins
}
func TestOSBackendUnavailable(t *testing.T) {
	// Review Focus 5: keyring-backend os with DBUS_SESSION_BUS_ADDRESS unset (or a darwin build without CGO)
	// => `keys list` fails with an error that names the "os" backend, without a panic
}
func TestKeyMaterialNeverPrinted(t *testing.T) {
	// `keys import-hex r` fed a malformed key on stdin fails, and neither stdout nor stderr contains the input
	// (spec §13; TS run.test.ts "never echoes malformed private key contents")
}
func TestVersionOutput(t *testing.T) { // lines "service akt-reporter", "version ", "commit ", "platform <goos>_<goarch>", "go "
}

// operator/reporter/clitest/preview_test.go
func TestScanExitCodes(t *testing.T) { // scenario "clean" => 0; scenario "stale-akash" => 1 plus {"error":...}
}
func TestStartStopsOnCancel(t *testing.T) { // cancelling the context ends start with code 0 after writing one summary
}
func TestLogsOnStderrEventsOnStdout(t *testing.T) { // every stdout line is JSON; startup logs appear on stderr
}
func TestMetricsInPreview(t *testing.T) { // metrics.listen set in preview => GET /metrics returns akt_reporter_pass_duration_seconds
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./cmd/akt-reporter/... ./operator/reporter/clitest/...`. Expected: FAIL.

- [ ] **Step 3: Implement.**
  - **Root:** `--home` (`AKT_REPORTER_HOME`), the log flags `--log_level`, `--log_format`, `--log_color`, `--log_timestamp` (zerolog to stderr), and `config.AddFlags` as persistent flags.
  - **Bech32 prefixes:** import `pkg.akt.dev/go/sdkutil` for its prefix `init`.
  - **`keys`:** `keys.Commands()` from cosmos-sdk `client/keys`. It declares its own `--keyring-backend` (default `os`) and `--keyring-dir`, which shadow the root flags for `keys` subcommands. Its `PersistentPreRunE` loads the config and builds the `client.Context` with `Codec`, `InterfaceRegistry`, `Input` (stdin, which `import-hex` reads), `Output`, `HomeDir` and a keyring from the configured backend and directory, unless `--keyring-backend` or `--keyring-dir` is explicitly changed on the command line.
  - **Signals:** SIGINT and SIGTERM cancel the review context; a second signal exits immediately.
  - **Construction:** the observer's `onChange` callback must reach the scanner, which is built after it. Declare `var sc *scan.Scanner`, pass `func(c observe.Change) { sc.OnChange(c) }` to `observe.New`, then assign `sc = scan.New(...)`.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS. Also run `CGO_ENABLED=0 go build ./cmd/akt-reporter`; expected exit 0.

- [ ] **Step 5: Commit** `feat(reporter): add preview CLI`

### Task 14: Parity goldens

**Files:**
- Create: `operator/reporter/internal/simnode/cmd/simnode/main.go`
- Modify: `operator/reporter/internal/simnode/scenarios.go`
- Create: `contracts/solana/reporter/goldens.ts`, `operator/reporter/testdata/goldens/*.jsonl`, `operator/reporter/testdata/pda_vectors.json`
- Test: `operator/reporter/scan/golden_test.go`, `operator/reporter/solana/pda_vectors_test.go`

**Interfaces:**
- Consumes: `simnode.Scenarios()`, the `Scanner`, the TS `Scanner`, `AkashClient` and `SolanaObserver`.
- Produces: `simnode serve --scenario NAME` (prints `{"comet":...,"grpc":...,"solana":...}` and serves until stdin closes) and `simnode list`; the golden files; `make reporter-goldens` (added to `make/reporter.mk` in Task 23).

- [ ] **Step 1: Add scenarios** covering the bullet list in spec §11.2, one `Scenarios()` entry each, named in kebab case. The round-robin audit scenario sets `Passes: 3`.

- [ ] **Step 2: Write the failing Go tests**

```go
func TestGoldens(t *testing.T) {
	// for each name in simnode.Scenarios(): run Scenario.Passes preview passes on one Scanner; compare against
	// testdata/goldens/<name>.jsonl as parsed JSON. Lines are grouped per transfer ID; each group's order and the
	// summary-last rule are enforced per pass; error texts are compared when the golden holds a TS-authored message;
	// the §5.6 intentional differences are allowed explicitly per scenario
}
func TestPDAVectors(t *testing.T) { // testdata/pda_vectors.json from TS: StatusAddress and the config, bridge, mint and mint_authority PDAs
}
```

- [ ] **Step 3: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/... -run 'Goldens|PDAVectors'`. Expected: FAIL (the golden files do not exist).

- [ ] **Step 4: Write `goldens.ts`.** For each scenario it spawns `go run ./operator/reporter/internal/simnode/cmd/simnode serve --scenario NAME` with `GOWORK=off` and its working directory at the repository root. It builds the TS `AkashClient`, `SolanaObserver` and `Scanner` (preview, `auditPerScan` 100), runs `scan()` `Passes` times, and writes every event to `operator/reporter/testdata/goldens/NAME.jsonl`. It also writes `pda_vectors.json`.

- [ ] **Step 5: Generate the goldens.**
  - Prerequisite: the `contracts/solana` development setup (`contracts/solana/docs/setup.md`: `npm run setup`, `npm run build`).
  - Run: `cd contracts/solana && npx tsx reporter/goldens.ts`. Expected: one `.jsonl` per scenario.
  - Then run `npm run typecheck` and `npm run format:check` in `contracts/solana`. Expected: both pass.

- [ ] **Step 6: Run the tests and confirm they pass.** Same command as Step 3. Expected: PASS. A mismatch is a port bug: fix the Go side, not the golden.

- [ ] **Step 7: Commit** `test(reporter): add TS parity goldens`

### Task 15: Benchmark `steady-100k`

**Files:**
- Create: `operator/reporter/internal/simnode/steady.go`
- Modify: `operator/reporter/internal/simnode/scenarios.go` (register `steady-100k`)
- Test: `operator/reporter/scan/steady_test.go`

**Interfaces:**
- Consumes: `simnode` and its `cmd/simnode` (Task 14), `Scanner`, `events`.
- Produces: `simnode.Steady100k() Scenario`, `BenchmarkPass`, and `TestSteady100k`, gated by `REPORTER_PERF=1`.

- [ ] **Step 1: Write the failing test**

```go
func TestSteady100k(t *testing.T) {
	// skip unless REPORTER_PERF=1; workload and latency exactly as spec §11.4
	// simnode runs as a subprocess (`simnode serve --scenario steady-100k`) so the heap measures the reporter only
	// require: wall time <= 60s; peak /memory/classes/heap/objects:bytes sampled every 100ms <= 512 MiB;
	// summary failures == 0, scanned == 100000, audited == 100;
	// exactly 5,000 MsgReportOutcome planned, 1,000 alerts "Transfer requires governance review",
	// 100 audit early exits, no other messages or alerts; write cpu.pprof and heap.pprof to $REPORTER_PERF_OUT
}
```

- [ ] **Step 2: Run the test and confirm it fails.** Run: `REPORTER_PERF=1 go test ./operator/reporter/scan/ -run TestSteady100k -timeout 10m`. Expected: FAIL (undefined `Steady100k`).

- [ ] **Step 3: Implement `Steady100k`** with the categories of spec §11.4. Preserve the simnode latencies (`AkashLatency` 2 ms, `SolanaLatency` 5 ms, `BlockFetchLatency` 200 ms). Add `BenchmarkPass` in `steady_test.go`: it runs one pass per iteration against the same subprocess.

- [ ] **Step 4: Run the test and confirm it passes.**
  - Expected: PASS. If it misses the time or heap bound, profile and fix before continuing; do not relax the bounds.
  - Then run `make lint-go`. Expected: no findings.

- [ ] **Step 5: Commit** `test(reporter): add steady-100k benchmark`

---

# Milestone 2: broadcasting

At the end of Task 21, `broadcast = true` signs through the keyring or Cosmos-KMS and sends batched, unordered transactions under the nonce-slot rules of spec §6, so no vote, veto or refund executes twice.

### Task 16: Keyring signer

**Files:**
- Create: `operator/reporter/signer/signer.go`, `operator/reporter/signer/keyring.go`
- Test: `operator/reporter/signer/keyring_test.go`

**Interfaces:**
- Produces:
  - `signer.Signer` interface: `Address() sdk.AccAddress`, `PubKey() cryptotypes.PubKey`, `Sign(ctx, signBytes []byte) ([]byte, error)`
  - `signer.NewKeyring(kr keyring.Keyring, from string, reporter sdk.AccAddress) (Signer, error)`
  - `signer.NormalizeAndVerify(pub cryptotypes.PubKey, signBytes, sig []byte) ([]byte, error)`. It accepts 64 or 65 bytes (dropping v), converts high S to low S, and verifies over `sha256(signBytes)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestKeyringSigner(t *testing.T) {
	// a test-backend keyring with key "r": Sign over bytes verifies with PubKey;
	// a key that does not derive reporter => "signer does not match reporter";
	// a non-secp256k1 key => error
}
func TestNormalizeAndVerify(t *testing.T) {
	// a high-S signature becomes low-S and verifies; a 65-byte r||s||v is accepted;
	// a wrong key or a bit flip => error; a 63-byte signature => error
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/signer/... -run 'Keyring|Normalize'`. Expected: FAIL.

- [ ] **Step 3: Implement.** Use `kr.Sign(uid, bytes, signing.SignMode_SIGN_MODE_DIRECT)` and decred `secp256k1/v4` for normalization.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 5: Commit** `feat(reporter): add keyring signer`

### Task 17: Cosmos-KMS remote signer

**Files:**
- Create: `operator/reporter/signer/kmspb/signerservice.proto`, copied from `github.com/cosmos/kms` commit `e3dbb2736910f0f07de71d618238928bf706df88` (`proto/signerservice/signerservice.proto`), with that commit in a header comment
- Create: `operator/reporter/signer/kmspb/signerservice.pb.go`, `signerservice_grpc.pb.go` (generated and committed), `operator/reporter/signer/kmspb/generate.go`
- Create: `operator/reporter/signer/remote.go`
- Test: `operator/reporter/signer/remote_test.go`

**Interfaces:**
- Consumes: `signer.Signer`, `NormalizeAndVerify` (Task 16).
- Produces:
  - `signer.RemoteOptions{Address, KeyID, CAFile, ServerName, CertFile, KeyFile string; Timeout time.Duration}`
  - `signer.NewRemote(ctx, RemoteOptions, reporter sdk.AccAddress) (Signer, error)`
  - `TestRemoteAgainstKMS`, driven by environment variables:
    - `REPORTER_KMS_ADDR`, the server address;
    - `REPORTER_KMS_CA`, the CA file;
    - `REPORTER_KMS_KEYS`, a comma-separated list of key IDs.

    It is skipped when `REPORTER_KMS_ADDR` is unset.

- [ ] **Step 1: Generate the code.**
  - Install the generators: `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12` and `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1`.
  - `generate.go` holds `//go:generate protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative signerservice.proto`. Run `go generate ./operator/reporter/signer/kmspb/`.
  - Expected: `go build ./operator/reporter/signer/...` exits 0.

- [ ] **Step 2: Write the failing tests** against an in-process fake `SignerService` with TLS from a test CA.

```go
func TestRemoteSchemes(t *testing.T) {
	// ECDSA_SECP256K1: payload == signBytes, response r||s; ECDSA_SECP256K1ETH: payload == sha256(signBytes),
	// response r||s||v; both verify
}
func TestRemoteRejects(t *testing.T) {
	// a key that does not derive reporter; scheme ED25519; a bad signature; a response over 65 bytes;
	// a server with an untrusted certificate => errors, and Sign returns no signature
}
func TestRemoteAgainstKMS(t *testing.T) {
	// skip unless REPORTER_KMS_ADDR is set (release gate, Task 27): for each ID in REPORTER_KMS_KEYS, GetKey derives
	// the reporter address, then NewRemote signs a sign doc and NormalizeAndVerify accepts it
}
```

- [ ] **Step 3: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/signer/... -run Remote`. Expected: FAIL.

- [ ] **Step 4: Implement** following spec §7.2. TLS is always on, with no plaintext path. `GetKey` runs once in `NewRemote`; each `Sign` has a `Timeout` deadline.

- [ ] **Step 5: Run the tests and confirm they pass.** Expected: PASS.

- [ ] **Step 6: Commit** `feat(reporter): add Cosmos-KMS remote signer`

### Task 18: Nonce slots, the replay key and the unordered transaction builder

**Files:**
- Create: `operator/reporter/tx/slots.go`, `builder.go`, `prune.go`, `fees.go`
- Create: `x/sink/sink_unordered_test.go`
- Test: `operator/reporter/tx/slots_test.go`, `operator/reporter/tx/builder_test.go`

**Interfaces:**
- Consumes: `signer.Signer` (Task 16); `policy.Planned`, `policy.Key` (Task 3); `(sinkid.ID).KeySuffix` (Task 1).
- Produces:
  - Slots (spec §6.1), in `slots.go`:
    - constants `VoteSlots = 128` and `SingleSlots = 1024`;
    - `SlotOf(id sinkid.ID, k policy.Key) int`: 0 to 127 for votes, 128 to 1151 for vetoes and refunds;
    - `WindowOf(blockTime time.Time) int64`, `WindowStart(w int64) time.Time`, `SlotTimeout(w int64, slot int) time.Time` (UTC);
    - `SendCutoff(signerTimeout time.Duration) time.Duration`, which is 20 s + `signerTimeout`;
    - the window (2 min), spacing (1 ms), cutoff base (20 s) and flush lead (10 s) live in one unexported package variable, `timing`. Tests in package `tx` scale it with `scaleTiming(t, factor)`, defined in `slots_test.go` and restored by `t.Cleanup`. Nothing else changes it.
  - `tx.NewBuilder(cfg client.TxConfig, chainID string, accountNumber uint64, s signer.Signer) *Builder`
  - `(*Builder).SimBytes(msgs []sdk.Msg, timeout time.Time) ([]byte, error)`
  - `(*Builder).Build(ctx, msgs []sdk.Msg, gasLimit uint64, fee sdk.Coins, timeout time.Time) (txBytes, hash []byte, err error)`
  - `tx.Prune(sim func([]sdk.Msg) (uint64, error), batch []policy.Planned) PruneResult`, where `PruneResult{Kept []policy.Planned; GasUsed uint64; Dropped []Dropped; Deferred []policy.Planned}` and `Dropped{Planned policy.Planned; Err error}`:
    - `Kept` simulated successfully, using `GasUsed`;
    - `Deferred` holds the messages left after three failing rounds (spec §6.4);
    - an error without an index drops the whole batch.
  - `tx.FeeFor(gasLimit uint64, prices sdk.DecCoins, fees sdk.Coins) sdk.Coins`
  - `TestUnorderedReplayKey` and `TestUnorderedReplayKeyBoundary` in package `sink_test`, like the harness. The PR `tests` job's `make test-full` runs them.

- [ ] **Step 1: Write the replay-key test on the real application.** It pins chain behavior that already exists, so it passes at once; it guards the assumption that spec §6.1 rests on.

```go
func TestUnorderedReplayKey(t *testing.T) {
	// Each subtest builds a fresh suite (setupReportingTestSuite) and first deposits transfer 1 (h.deliver(t, "", h.deposit(10))).
	// Reporter 0 signs two unordered txs (sequence 0, timeout = block time + 1m) with the same timeout, each carrying the
	// same MsgReportOutcome for transfer 1 but a different memo, so the bytes differ.
	// t.Run("same block"): commitBlock with both => first code 0, second code 18 with "failed to add unordered nonce";
	// t.Run("consecutive blocks"): first in block h, second in block h+1 => the same codes;
	// in both: the reporter's uakt balance falls by exactly one fee, and the transfer records the reporter's vote once
}
func TestUnorderedReplayKeyBoundary(t *testing.T) {
	t.Skip("the Akash cosmos-sdk fork accepts a timeout equal to the block time; un-skip once it adopts cosmos/cosmos-sdk#26672 (spec §14)")
	// as above, but the second tx goes in a block whose time equals the timeout exactly => refused, never code 0
}
```

Add an unordered variant of `txFromMode` (`x/sink/sink_test.go:185`) in this file, in package `sink_test`: `SetUnordered(true)`, `SetTimeoutTimestamp`, sequence 0.

- [ ] **Step 2: Run it.** Run: `go test ./x/sink/ -run TestUnorderedReplayKey -v`. Expected: `TestUnorderedReplayKey` PASS and `TestUnorderedReplayKeyBoundary` SKIP. If `TestUnorderedReplayKey` fails, stop and report: the slot design depends on this rule.

- [ ] **Step 3: Write the failing reporter tests**

```go
func TestSlotVectors(t *testing.T) {
	// owner 20 x 0x01, height 42, sequence 7, hashed as in spec §6.1:
	// vote with outcome 1, round 1, policy 2 => slot 30; the same vote with outcome 2 => slot 73;
	// veto => slot 217; refund => slot 613
	// block time 1800000005s => window 15000000; SlotTimeout(15000000, 0) == 2027-01-15T08:01:59.999Z,
	// slot 127 => 08:01:59.872Z, slot 128 => 08:01:59.871Z, slot 1151 => 08:01:58.848Z
}
func TestSlotTimeoutsDistinct(t *testing.T) { // the 1,152 timeouts of a window are distinct, at or after WindowStart(w+1)-1.152s, and before WindowStart(w+1)
}
func TestSlotsUseBlockTime(t *testing.T) { // Review Focus 4: WindowOf depends only on its argument; SlotTimeout(WindowOf(bt), s) - bt < 2m for any bt and s
}
func TestSendCutoff(t *testing.T) { // SendCutoff(0) == 20s; SendCutoff(10s) == 30s
}
func TestBuildUnordered(t *testing.T) {
	// decoded tx: unordered == true, TimeoutTimestamp == given, TimeoutHeight == 0, memo "AKT sink reporter",
	// one SignerInfo with sequence 0 and SIGN_MODE_DIRECT, and the signature verifies over the direct sign bytes
	// for the account number
}
func TestPrune(t *testing.T) {
	// a failure at "message index: 1" drops message 1 and keeps the rest with the second simulation's gas;
	// three failing rounds => Kept empty and Deferred holding the remaining messages;
	// an error without an index => every message in Dropped
}
func TestFeeFor(t *testing.T) { // gas 100000 at 0.025uakt => 2500uakt; explicit fees win
}
```

- [ ] **Step 4: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/tx/... -run 'Slot|Cutoff|Build|Prune|Fee'`. Expected: FAIL.

- [ ] **Step 5: Implement.**
  - Build with `TxConfig.NewTxBuilder()` (`SetUnordered`, `SetTimeoutTimestamp`), `authsigning.GetSignBytesAdapter` and `TxEncoder`.
  - The prune index pattern is `message index: (\d+)`.
  - `SlotOf` hashes exactly the bytes of spec §6.1 with `crypto/sha256`.
  - `WindowOf` uses floor division on Unix nanoseconds.

- [ ] **Step 6: Run the tests and confirm they pass.** Same commands as Steps 2 and 4. Expected: PASS.

- [ ] **Step 7: Commit** `feat(reporter): add nonce slots and the unordered transaction builder`

### Task 19: Broadcaster and slot table

**Files:**
- Create: `operator/reporter/tx/broadcaster.go`, `operator/reporter/tx/table.go`
- Modify: `operator/reporter/internal/simnode/scenarios.go` (add `broadcast-mix`)
- Test: `operator/reporter/tx/broadcaster_test.go`, `operator/reporter/tx/norepeat_test.go`

**Interfaces:**
- Consumes: the slot functions, `Builder`, `Prune`, `FeeFor` (Task 18); `akash.Client` through `tx.Chain`; `events`; `clock`; `metrics`.
- Produces:
  - `tx.Chain` interface: the `akash.Client` methods `Latest`, `Simulate`, `BroadcastSync`, `Tx`, `TxIndexEnabled`
  - `tx.Options{Reporter string; MaxMsgs, MaxInFlight int; GasPerMessage uint64; GasAdjustment float64; GasPrices sdk.DecCoins; Fees sdk.Coins; MaxGas uint64; MaxFee sdk.Coin; SignerTimeout time.Duration /*0 for the keyring*/; RefreshInterval time.Duration /*1s*/; WaitLimit time.Duration /*3m without progress*/; ShutdownGrace time.Duration /*30s*/; Clock clock.Clock; Metrics *metrics.Metrics}`
  - `tx.Reviewer` interface: `ReviewRefundHead(ctx, minHeight int64) error` and `Review(ctx, ids []sinkid.ID, minHeight int64) error`, satisfied by `*scan.Scanner` (Task 12)
  - `tx.New(o Options, chain Chain, b *Builder, ev *events.Writer) *Broadcaster`
  - Methods on `*Broadcaster`:
    - `Queue(policy.Planned) bool`, which satisfies `scan.Queuer`. It returns false when the message is skipped or deferred.
    - `Flush()`: the reviews that fed the current batches have ended, so partial vote batches go out now.
    - `Run(ctx) error`
    - `Wait(ctx) (failures int)`: returns once nothing is queued, unresolved or deferred and follow-up reviews are idle, or once `WaitLimit` passes without progress (spec §6.5).
    - `TakeFailures() int`: failures resolved since the last call
    - `SetReviewer(Reviewer)`. This task builds the follow-up goroutine of spec §6.6; Task 20 adds refund-head progression to it.

- [ ] **Step 1: Write the failing tests** against `simnode` (scenario `broadcast-mix`, broadcast injection, `Gossip`, `SetLag`, `Resolve`).
  - Most tests share one fake clock between the broadcaster and `simnode` and advance it explicitly.
  - `TestNoVoteExecutesTwice` runs on the real clock with `scaleTiming(t, 1.0/240)`: a 0.5 s window, an 83 ms cutoff base and a 42 ms flush lead. Task 20's drain uses its own factor.
  - Tests in `norepeat_test.go` plan only through a real `scan.Scanner` (Task 12) over the same `simnode`, wired both ways: the scanner is the broadcaster's `Reviewer`, and the broadcaster is its `Queuer`. They never queue synthetic `Planned` values; the slot-table tests in `broadcaster_test.go` may.
  - No test in package `tx` calls `t.Parallel()`, because `scaleTiming` changes a package variable.

```go
// broadcaster_test.go
func TestQueueNotBlockedBySigner(t *testing.T) { // signer blocked for 5s => 100 Queue calls each return within 100ms
}
func TestPriority(t *testing.T) { // with one free in-flight place: veto, then refund, then vote batch
}
func TestSlotBatching(t *testing.T) {
	// votes of one slot share a tx whose timeout is that slot's SlotTimeout; votes of different slots never share one;
	// a 26th vote for a full slot is deferred (reason slot_full); a veto and a refund each go alone, in slots 128-1151;
	// queuing a key already queued or sent in its slot returns false and changes nothing
}
func TestSendTriggers(t *testing.T) { // a partial batch goes out on Flush, or at its flush lead (10s before the cutoff) without Flush
}
func TestCutoff(t *testing.T) { // keyring: 19s left before a slot's timeout => deferred (reason cutoff), 21s => sent; SignerTimeout 10s: 29s => deferred
}
func TestEarlierWindowNeverSent(t *testing.T) { // Review Focus 1: a plan whose PlanTime is in an earlier window than the latest block seen is deferred (reason cutoff), never signed
}
func TestSlotTaken(t *testing.T) {
	// first send refused with "failed to add unordered nonce" => no event, messages deferred (reason slot_busy), slot
	// closed for the window; a Gossiped rival executes first => our tx is included with code 18 => deferred, not "failed"
}
func TestDeferredReviewedNextWindow(t *testing.T) {
	// deferred IDs go to Reviewer.Review once a block of the next window is seen; if the vote executed meanwhile through
	// a rival tx, that review plans nothing and no event follows
}
func TestInFlightStates(t *testing.T) {
	// queued entries are released with {id,error} on simulation, signing, membership ("reporter was removed") and cap
	// rejection; a released message can be queued again; a sent entry is kept until inclusion or expiry
}
func TestResendsAfterLostResponse(t *testing.T) {
	// first broadcast: transport error; re-send: "tx already exists in cache" or "failed to add unordered nonce"
	// => stays sent, later confirmed by tx(hash); no event other than transactionHash
}
func TestFirstSendInCache(t *testing.T) {
	// two broadcasters build byte-identical single-message txs (a refund) in one window; the second's first send gets the
	// JSON-RPC error "tx already exists in cache" => marked sent, no re-send loop, no error event; both confirm by hash
	// and write transactionHash once; Executions == 1
}
func TestFirstSendRejectionIsFinal(t *testing.T) {
	// CheckTx code 5 on a first send => "sink transaction rejected: <HASH> code 5: <log>" per message; a new plan for
	// the same slot in the same window is then sent
}
func TestResendRejectionNotFinal(t *testing.T) { // after a transport error, a CheckTx rejection of the re-send keeps the tx sent until it resolves
}
func TestConfirmationEvents(t *testing.T) {
	// code 0 => {id, transactionHash} per message with uppercase hex; code 5 => "sink transaction failed: <HASH> code 5"
}
func TestExpiryByChainTime(t *testing.T) {
	// block time passes the timeout and the final lookup misses => "sink transaction expired: <HASH>";
	// afterwards the slot table holds no entry of that window
}
func TestHaltedChain(t *testing.T) {
	// simnode.Halt with one tx sent and one message deferred (slot busy): after WaitLimit of fake time without progress,
	// Wait reports "sink transaction unconfirmed: <HASH>" (tx result unconfirmed) and
	// "sink message not sent: deferred (slot_busy) until <RFC 3339 time>", and counts 2 failures; in start mode both remain
}
func TestNoTxIndex(t *testing.T) {
	// tx_index off: one warning; accepted txs produce no outcome event and no failure; Wait does not block on them;
	// Executions shows each vote once after the next window's reviews
}
func TestCapsSplitBatches(t *testing.T) { // simulated gas above MaxGas => the batch is halved and the second half deferred (reason cap); a single message over the cap => error
}
func TestGasLimit(t *testing.T) { // auto: ceil(simulated x 1.3); GasPerMessage 50000 with 3 messages => 150000
}
func TestUnorderedDisabled(t *testing.T) { // CheckTx "unordered transactions are not enabled" => a rejection per message; no ordered retry
}
func TestShutdownReleasesQueuedSilently(t *testing.T) { // cancel: queued and deferred messages released without events; sent ones wait up to ShutdownGrace
}

// norepeat_test.go (spec §11.3)
func TestRestartNoDoubleExecution(t *testing.T) {
	// broadcaster A sends a two-vote batch that is admitted with its response lost, and A is discarded; broadcaster B,
	// with its own scanner, plans one of those votes in the same window => B's tx is refused (slot taken) or loses in the
	// block; in the next window B's review sees the vote; Executions == 1 for both votes
}
func TestTwoProcessesShareSlots(t *testing.T) { // two broadcasters plan 40 overlapping votes in one window => every vote executes exactly once
}
func TestVetoAcrossGovernance(t *testing.T) {
	// a veto planned in round 1 is admitted but not yet included; simnode.Resolve starts round 2 (refund pending, new
	// challenge end); the next review, in the same window, plans a veto in round 2 => it shares the round-1 veto's slot
	// and timeout, so at most one veto executes in round 2 (Executions keyed by round)
}
func TestNoVoteExecutesTwice(t *testing.T) {
	// scaled timing; 20 seeded rounds over scenario broadcast-mix (about 30 s), 200 with REPORTER_PERF=1. Each round randomly
	// lags the node 0-3 blocks, loses broadcast responses after admission, restarts a broadcaster, runs a second one,
	// Gossips rival txs, resolves a transfer by governance and toggles the tx index.
	// Afterwards every Executions value is <= 1. The seed is logged so a failure can be replayed.
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/tx/... -run 'QueueNot|Priority|SlotBatching|SendTriggers|Cutoff|EarlierWindow|SlotTaken|Deferred|InFlight|Resend|Rejection|Confirmation|Expiry|Halted|NoTxIndex|Caps|GasLimit|Unordered|Shutdown|FirstSendInCache|Restart|TwoProcesses|VetoAcrossGovernance|NoVoteExecutesTwice'`. Expected: FAIL.

- [ ] **Step 3: Implement** spec §6.1, §6.2, §6.4 and §6.5.
  - **Owner goroutine:** the only one that reads or writes the slot table (`table.go`), keyed by (window, slot). It admits, batches, orders by priority, records state and writes outcome events, and never does I/O. `Queue` asks it over a channel and gets the answer back on a reply channel.
  - **Workers:** up to `MaxInFlight` of them simulate, sign, broadcast and look up transactions, then send the results back to the owner.
  - **Latest block time seen:** the maximum over every snapshot the broadcaster has read. The cutoff and the earlier-window rule use it.
  - **Broadcast results:** `InCache` on any send marks the transaction sent and never enters the re-send loop (spec §6.5).
  - **Follow-up goroutine** (spec §6.6): runs reviews one at a time, never on the owner goroutine. Once a block of a later window is seen, it calls `Review` with the earlier windows' deferred IDs, using that block's height as `minHeight`. It then flushes the batches the review filled. Task 20 adds refund-head progression to it.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS, also with `-race`. Then run `REPORTER_PERF=1 go test ./operator/reporter/tx/ -run TestNoVoteExecutesTwice`; expected: PASS after about 5 min.

- [ ] **Step 5: Commit** `feat(reporter): add the slot-table broadcaster`

### Task 20: Refund-head progression and drain

**Files:**
- Modify: `operator/reporter/tx/broadcaster.go` (the refund trigger), `operator/reporter/internal/simnode/scenarios.go` (add `refund-queue`, `drain-5000`)
- Test: `operator/reporter/tx/progression_test.go`, `operator/reporter/tx/drain_test.go`

**Interfaces:**
- Consumes: `(*Scanner).ReviewRefundHead` (Task 12), the follow-up goroutine (Task 19).
- Produces: refund-head progression (spec §6.6). `Wait` blocks until follow-up reviews are idle.

- [ ] **Step 1: Write the failing tests.** The drain runs in plain `go test` under `scaleTiming(t, 1.0/20)`: a 6 s window and 300 ms blocks, with no injected latency, so reviews stay short against the window. The tracker's `MinRefresh` and the broadcaster's `RefreshInterval` are 50 ms. Expect about 60 s in total.

```go
func TestRefundHeadProgression(t *testing.T) {
	// scenario refund-queue: R1, R2, R3 all eligible, in distinct slots; one pass then Wait => three MsgFinalizeRefund
	// included in that order, one per block; Executions shows each refund once
}
func TestProgressionRetriesLowerSnapshot(t *testing.T) {
	// R1 included at height H; simnode SetLag(1) until H+2 => ReviewRefundHead(ctx, H) waits instead of reviewing R1 at
	// H-1, then finalizes R2; Wait does not return early
}
func TestRefundSlotCollisionWaitsForNextWindow(t *testing.T) { // R2's slot already used in this window => R2 is finalized in the next window
}
func TestProgressionStops(t *testing.T) { // a head that is not eligible (challenge not elapsed) stops progression without messages
}
func TestDrain(t *testing.T) {
	t.Run("fast signer", func(t *testing.T) {
		// scenario drain-5000, instant signer, starting 90% into a window: unconfirmed txs never exceed MaxInFlight;
		// every vote is confirmed, failing beyond eight windows; Executions shows each vote once; Pass returns before
		// the drain ends; the number of windows used is logged
	})
	t.Run("signer recovery", func(t *testing.T) {
		// 5ms per signature, then the signer fails for 500ms (the spec's 100ms and 10s, scaled): no restart; the failed
		// messages are released, planned again, and all confirmed once
	})
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/tx/... -run 'Progression|RefundSlot|Drain'`. Expected: FAIL.

- [ ] **Step 3: Implement.** On a code-0 inclusion of a `MsgFinalizeRefund`, the follow-up goroutine calls `ReviewRefundHead` with the inclusion height as `minHeight`. `ReviewRefundHead` reads `RefundHead` from that snapshot, reviews that one transfer as a one-ID chunk, and queues through the normal `Queue`.

- [ ] **Step 4: Run the tests and confirm they pass.** Expected: PASS, also with `-race`.

- [ ] **Step 5: Commit** `feat(reporter): progress through the refund queue after each payment`

### Task 21: Broadcast CLI wiring

**Files:**
- Modify: `cmd/akt-reporter/cmd/run.go`
- Test: `operator/reporter/clitest/broadcast_test.go`

**Interfaces:**
- Consumes: `signer.NewKeyring`, `signer.NewRemote`, `tx.NewBuilder`, `tx.New`, `(*Broadcaster).SetReviewer`, `(*akash.Client).AccountNumber`, `(*Scanner).Pass`.
- Produces: in broadcast mode, `start` and `scan` sign and send.
  - **Startup:** the signer must derive `reporter`; the account number is read (`reporter account not found`); the tx index is checked. `Options.SignerTimeout` is `signer.remote.timeout` for the remote signer and 0 for the keyring.
  - **`scan`:** `settle` is `func() int { b.Flush(); return b.Wait(settleCtx) }`. `settleCtx` is cancelled only by a signal's 30 s grace (spec §5.7), and `Wait` applies the 3-minute waiting limit itself.
  - **`start`:** `settle` is `func() int { b.Flush(); return b.TakeFailures() }`.
  - **On a signal:** the review context is cancelled, and `settle` gets a 30 s grace context (spec §5.7).
  - **Summary:** written only by `Pass`, never by `run.go`.

- [ ] **Step 1: Write the failing tests.** Every test starts `simnode` with `AlignWindow` and sets `max-age = "80s"`.

```go
func TestStartupRefusals(t *testing.T) {
	// node-ID mismatch; a height-echo mismatch on the first snapshot; a KMS key that does not derive reporter;
	// a missing account => exit 1 with the error; preview with both signers configured => starts without opening either
}
func TestScanBroadcastEndToEnd(t *testing.T) {
	// scenario votes: scan --broadcast exits 0; 30 transactionHash events appear before the summary; the summary's
	// failures == 0; every tx is unordered with a slot timeout; Executions shows each vote once
}
func TestScanTwiceNoRepeat(t *testing.T) { // scan --broadcast twice in one window over scenario votes => the second run plans nothing; Executions shows each vote once
}
func TestScanCountsTransactionFailures(t *testing.T) { // a delivery failure injected => the summary's failures == 1 and the exit code is 1
}
func TestStartAttributesLateOutcomes(t *testing.T) { // an outcome resolving after a summary is counted in the next pass's failures
}
func TestSigtermOrdering(t *testing.T) { // SIGTERM mid-pass: no new signing; sent txs confirm within the grace; the summary is the last line
}
func TestReadOnlyHome(t *testing.T) {
	// Review Focus 5 (skipped when os.Geteuid() == 0): a home whose directories are mode 0500, with a test-backend key
	// => broadcast scan loads the key and signs without writing; `keys add` fails with a permission error
}
```

- [ ] **Step 2: Run the tests and confirm they fail.** Run: `go test ./operator/reporter/clitest/... -run 'Startup|Broadcast|ScanTwice|Counts|Late|Sigterm|ReadOnly'`. Expected: FAIL.

- [ ] **Step 3: Implement** the wiring and shutdown order of spec §5.7.

- [ ] **Step 4: Run the tests and confirm they pass.**
  - Expected: PASS.
  - Run the full suite: `go test -race ./operator/... ./cmd/akt-reporter/...`. Expected: PASS.
  - Then run `make lint-go`. Expected: no findings.

- [ ] **Step 5: Commit** `feat(reporter): wire broadcast mode into the CLI`

---

# Milestone 3: release and operations

At the end of Task 27, a `reporter/v*` tag publishes binaries and a multi-arch image behind the KMS and performance gates, and operators have compose, systemd and migration documentation.

### Task 22: Node-load benchmark

**Files:**
- Create: `x/sink/sink_reporter_load_test.go`

**Interfaces:**
- Consumes: the x/sink test harness (`setupReportingTestSuite`, `h.update`, `commitBlock` in `x/sink/sink_reporting_test.go` and `sink_test.go`); `servergrpc.NewGRPCServer` from cosmos-sdk `server/grpc`.
- Produces: `TestReporterNodeLoad`, gated by `REPORTER_PERF=1`, writing its results to `$REPORTER_PERF_OUT/node-load.txt`.

- [ ] **Step 1: Write the test.**
  - **Quotas:** `setupReportingTestSuite` fixes 100-uakt caps. Lift `MaxAmountPerTransfer`, `MaxEpochAmountPerAccount` and `MaxTransferAmountPerEpoch` with `h.update(p)` before depositing.
  - **Helper:** add `depositTx(t, h, seq uint64) []byte`, a copy of `txFromMode` (`x/sink/sink_test.go:185`) that takes the sequence instead of reading it from state. `txFromMode` reads the committed sequence, so every transaction in a batch would otherwise reuse one sequence.
  - **Deposits:** commit 100,000 `MsgSink` deposits with consecutive sequences, in blocks of 1,000. Require every `ExecTxResult.Code == 0`, `len(h.ids) == 100_000` and `h.totals().PendingCount == 100_000` before measuring.
  - **Server:** serve `h.a` with `servergrpc.NewGRPCServer` on `127.0.0.1:0`.
  - **Load:** 32 goroutines query `Transfer` for random IDs at `h.height`, with `x-cosmos-block-height`, for 30 s.
  - **Assertions:** at least 2,000 queries per second and zero errors. Log the p50 and p99 latency.

- [ ] **Step 2: Run the test.** Run: `REPORTER_PERF=1 go test ./x/sink/ -run TestReporterNodeLoad -timeout 30m -v`. Expected: PASS. If it misses 2,000 per second, stop and report the measured rate: spec §14 requires a decision (reuse listed states while the height is unchanged) before release.

- [ ] **Step 3: Commit** `test(sink): add reporter node-load benchmark`

### Task 23: GoReleaser, Dockerfile, make targets and PR CI

**Files:**
- Create: `.goreleaser-reporter.yaml`, `_build/akt-reporter.Dockerfile`, `_build/akt-reporter-smoke.env`, `make/reporter.mk`, `_run/akt-reporter/reporter.example.toml`
- Modify: `Makefile` (add `include make/reporter.mk`), `.github/workflows/tests.yaml` (add the `reporter` job)
- Test: `cmd/akt-reporter/cmd/smoke_test.go`

**Interfaces:**
- Produces:
  - the make targets `akt-reporter`, `test-reporter`, `reporter-snapshot` and `reporter-goldens`;
  - the GoReleaser config of spec §10.1;
  - the Dockerfile of spec §10.2, verbatim;
  - a valid testnet smoke environment, used by Task 27.

- [ ] **Step 1: Write the failing test**

```go
func TestSmokeEnvPassesCheck(t *testing.T) { // the variables in ../../../_build/akt-reporter-smoke.env => `check --network testnet` exits 0
}
```

- [ ] **Step 2: Run the test and confirm it fails.** Run: `go test ./cmd/akt-reporter/cmd/ -run TestSmokeEnvPassesCheck`. Expected: FAIL (file missing).

- [ ] **Step 3: Write the files.**
  - **`_build/akt-reporter-smoke.env`:** `AKT_REPORTER_*` values for a valid testnet configuration.
    - Chain ID `smoke-1`.
    - A canonical 20-byte `akash1` reporter.
    - A 32-byte genesis hash other than mainnet's.
    - A non-development program ID, with `solana.mint` computed once with `solana.PDA(program, []byte("mint"))`.
  - **`_run/akt-reporter/reporter.example.toml`:** the output of `akt-reporter init` on an empty home, with mainnet defaults and the required keys left empty.
  - **`.goreleaser-reporter.yaml`** follows spec §10.1. ldflags: `-s -w -X github.com/cosmos/cosmos-sdk/version.Name=akt-reporter -X github.com/cosmos/cosmos-sdk/version.AppName=akt-reporter -X github.com/cosmos/cosmos-sdk/version.Version={{ .Version }} -X github.com/cosmos/cosmos-sdk/version.Commit={{ .FullCommit }}`.
  - **`make/reporter.mk`:**
    - `akt-reporter`: `CGO_ENABLED=0 GOWORK=off $(GO_BUILD) -trimpath -o $(AKASH_DEVCACHE_BIN)/akt-reporter ./cmd/akt-reporter`
    - `test-reporter`: `GOWORK=off $(GO_VET) ./operator/... ./cmd/akt-reporter/...` then `GOWORK=off CGO_ENABLED=1 $(GO_TEST) -race ./operator/... ./cmd/akt-reporter/...`
    - `reporter-snapshot`: `goreleaser release --snapshot --clean -f .goreleaser-reporter.yaml`
    - `reporter-goldens`: `cd contracts/solana && npx tsx reporter/goldens.ts`
  - **`tests.yaml` `reporter` job:** `actions/checkout@v4`, `./.github/actions/setup-ubuntu` (direnv, Go, `AKASH_ROOT`, as every other job), `make test-reporter`, `make akt-reporter`, then `goreleaser/goreleaser-action@v6` with `version: v2.17.1` and `args: check -f .goreleaser-reporter.yaml`.

- [ ] **Step 4: Verify.**
  - Run: `go test ./cmd/akt-reporter/cmd/ -run TestSmokeEnvPassesCheck`. Expected: PASS.
  - Run: `goreleaser check -f .goreleaser-reporter.yaml`. Expected: `config is valid`.
  - Run: `docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:latest -color .github/workflows/tests.yaml`. Expected: no findings.
  - Run: `make reporter-snapshot`. Expected: archives for 4 platforms in `.cache/goreleaser/reporter`, and local images `…:<version>-amd64` and `…-arm64`.
  - Run: `docker run --rm --network none --read-only <image>-amd64 version`. Expected: exit 0.
  - Run: `docker run --rm --network none --read-only --env-file _build/akt-reporter-smoke.env <image>-amd64 check --network testnet`. Expected: `Configuration valid.`
  - Run: `docker run --rm --network none <image>-amd64` with no config. Expected: it runs `start` and exits 1 with config errors, which proves the default command.

- [ ] **Step 5: Commit** `build(reporter): add GoReleaser, image and CI job`

### Task 24: Performance workflow

**Files:**
- Create: `.github/workflows/reporter-perf.yaml`

**Interfaces:**
- Consumes: `TestSteady100k` (Task 15), `TestNoVoteExecutesTwice` (Task 19), `TestReporterNodeLoad` (Task 22).
- Produces: a workflow with `schedule` (nightly), `workflow_dispatch` and `workflow_call` triggers.
  - Job `perf` runs on `ubuntu-24.04` (4 vCPU), uses `./.github/actions/setup-ubuntu`, and runs `REPORTER_PERF=1 REPORTER_PERF_OUT=$RUNNER_TEMP/perf GOWORK=off go test ./operator/reporter/scan/ ./operator/reporter/tx/ ./x/sink/ -run 'TestSteady100k|TestReporterNodeLoad|TestNoVoteExecutesTwice' -timeout 60m -v`. With `REPORTER_PERF=1`, `TestNoVoteExecutesTwice` (Task 19) runs all 200 rounds.
  - It uploads `$RUNNER_TEMP/perf` with `actions/upload-artifact@v4`.
  - `workflow_call` lets the release workflow use it as a gate.

- [ ] **Step 1: Write the workflow.**

- [ ] **Step 2: Verify.**
  - Run: `docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:latest -color .github/workflows/reporter-perf.yaml`. Expected: no findings.
  - Trigger it once with `gh workflow run reporter-perf.yaml --ref feat/akt-reporter-go` (or on a fork). Expected: green, with profiles and `node-load.txt` attached.

- [ ] **Step 3: Commit** `ci(reporter): run the performance benchmarks nightly and on demand`

### Task 25: Deployment files and documentation

**Files:**
- Modify (replace): `_run/akt-reporter/docker-compose.yaml`, `_run/akt-reporter/.env.example`
- Create: `_run/akt-reporter/reporter.env.example`
- Delete: `_run/akt-reporter/broadcast.yaml`, `_run/akt-reporter/config.example.json`
- Modify: `contracts/solana/deploy/systemd/akt-reporter.service`
- Modify: `contracts/solana/docs/reporter.md`, `operator-releases.md`, `operator-configuration.md`, `deploy-services.md`
- Create: `contracts/solana/docs/reporter-migration.md`

**Interfaces:**
- Produces: the files and documentation of spec §12.1 and §12.2.

- [ ] **Step 1: Rewrite the deployment files.**
  - **Compose:**
    - no `command` and no `platform`;
    - mount `${OPERATOR_HOME}:/home/akt-reporter:ro`;
    - `env_file: [{path: reporter.env, required: false}]`;
    - `stop_grace_period: 60s`;
    - keep the existing hardening keys.
  - **`.env.example`:** `OPERATOR_IMAGE`, `OPERATOR_UID`, `OPERATOR_GID`, `OPERATOR_HOME`.
  - **`reporter.env.example`:** commented `AKT_REPORTER_*` overrides, including `AKT_REPORTER_BROADCAST=true`.
  - **systemd:** `ExecStart=/usr/local/bin/akt-reporter start --home /var/lib/akt-reporter`, keeping the sandboxing lines.

- [ ] **Step 2: Rewrite the documentation.**
  - `reporter-migration.md` contains the spec §12.1 steps and the TS-flag table.
  - `operator-configuration.md` replaces the JSON tables with spec §8.3.
  - `reporter.md` updates the commands, key custody (keyring or Cosmos-KMS) and broadcasting, and keeps the alerts table unchanged. Broadcasting covers:
    - unordered transactions and nonce slots, with what "executes at most once" means (spec §6.1) and its one exception, the SDK boundary;
    - batching, priority and deferral to the next window, including deferred vetoes;
    - follow-up reviews;
    - the two-process rule of spec §12.2, and what a lagging node does (spec §6.5).
  - `reporter-migration.md` states the spec §12.1 step 5 rule in bold: stop the TS reporter, wait 21 blocks on the reporter's own node and then two more minutes before enabling broadcast, and never run both on one key.

- [ ] **Step 3: Verify.**
  - Run: `docker compose -f _run/akt-reporter/docker-compose.yaml --env-file _run/akt-reporter/.env.example config`. Expected: valid, with no `command`.
  - Run: `cd contracts/solana && npx prettier --check docs`. Expected: no differences.

- [ ] **Step 4: Commit** `docs(reporter): document the Go reporter and migration`

### Task 26: Local-stack end-to-end opt-in

**Files:**
- Modify: `contracts/solana/local/stack.ts:436-470`

**Interfaces:**
- Consumes: the `akt-reporter` binary (`make akt-reporter`, which writes `.cache/bin/akt-reporter`).
- Produces: when `REPORTER_GO_BIN` is set, the three reporters run the Go binary as described in spec §11.5. Otherwise behavior is unchanged.

- [ ] **Step 1: Implement.** For each reporter, the stack:
  1. writes `<ROOT>/reporter{i}/config/reporter.toml` with:
     - `network = "testnet"` (the local chain ID is not `akashnet-2`);
     - `node`, `grpc-addr = "127.0.0.1:<AKASH_GRPC_PORT>"` and `chain-id`;
     - the Solana keys and `reporter`;
     - `broadcast = true`, `from = "reporter"`, `scan.interval = "3s"`;
  2. pipes `reporter{i}.key` into `akt-reporter keys import-hex reporter --home <ROOT>/reporter{i}`;
  3. starts `akt-reporter start --home <ROOT>/reporter{i}` through the existing `start(...)` helper. It passes `<ROOT>/reporter{i}` as the marker, because `running()` (`stack.ts:58-63`) matches the marker against the process arguments.

- [ ] **Step 2: Verify.**
  - In `contracts/solana`, run `npm run typecheck` and `npm run format:check`. Expected: both pass.
  - Run `REPORTER_GO_BIN=$PWD/../../.cache/bin/akt-reporter npm run local:up`.
  - Check that `npm run local:status` lists three reporter processes whose `ps` arguments start with the Go binary.
  - Run `npm run local:demo`, then `npm run local:down`.
  - Expected: the demo's transfer reaches `STATUS_MINTED` through the Go reporters' votes, and the demo exits 0.

- [ ] **Step 3: Commit** `test(solana): run the Go reporter in the local stack on request`

### Task 27: Release workflow

**Files:**
- Create: `.github/workflows/release-reporter.yaml`
- Modify: `.github/workflows/release-operators.yaml`: the trigger becomes `relayer/v*` only, and the reporter-only branches (`broadcast.yaml` copy) are removed

**Interfaces:**
- Consumes: Tasks 17, 23, 24 and 25.
- Produces: the release workflow of spec §10.4, with two gate jobs that the publishing job `needs`:
  - **`kms-gate`:**
    - builds `github.com/cosmos/kms` at commit `e3dbb2736910f0f07de71d618238928bf706df88` with `go build -o kms ./cmd/kms`;
    - starts LocalStack as a service container, with the image pinned by digest (record the digest of the image you test with in the workflow);
    - creates an `ECC_SECG_P256K1`/`SIGN_VERIFY` key there;
    - writes a `kms.yaml` whose `grpc` block serves a test-CA TLS certificate and two keys: `secp-eth` (`backend: file`, `algorithm: secp256k1eth`, a generated hex key) and `secp` (`backend: awskms`, `algorithm: secp256k1`, the LocalStack key ID and endpoint);
    - runs `kms start`, then `REPORTER_KMS_ADDR=… REPORTER_KMS_CA=… REPORTER_KMS_KEYS=secp-eth,secp go test ./operator/reporter/signer/ -run TestRemoteAgainstKMS -v`.

    The kms build at this commit supports `secp256k1` over gRPC only through `awskms` (`config/config.go`, `GRPCKey`). If LocalStack cannot create the key, the job fails. Do not drop the `secp` key: spec §7.2 enables a scheme only after it is gated.
  - **`perf-gate`:** `uses: ./.github/workflows/reporter-perf.yaml`.

- [ ] **Step 1: Write the workflow.** The publishing job runs these steps in order:
  1. Run the tag-identity checks copied from `release-operators.yaml:30-48`, with the pattern `^reporter/v…`.
  2. Run `GOWORK=off go test ./operator/... ./cmd/akt-reporter/...`.
  3. Run `docker/setup-buildx-action@v3` (QEMU is not needed), then `goreleaser/goreleaser-action@v6` (`version: v2.17.1`, `release --snapshot --clean -f .goreleaser-reporter.yaml`). Smoke-test the amd64 image with `--network none --read-only`: `version`, then `check --network testnet` with `--env-file _build/akt-reporter-smoke.env`.
  4. Log in to GHCR. Run `release --clean --skip=validate -f .goreleaser-reporter.yaml` with `GORELEASER_CURRENT_TAG=v${VERSION}` and `REPORTER_IMAGE=ghcr.io/${owner,,}/akt-reporter`.
  5. Copy the `gh` draft, upload, anonymous-pull and publish steps from `release-operators.yaml:148-175`. Upload:
     - `*.tar.gz` and the checksums;
     - `_run/akt-reporter/docker-compose.yaml` as `akt-reporter-compose.yaml`;
     - `.env.example`, `reporter.env.example` and `reporter.example.toml`.

- [ ] **Step 2: Lint the workflows.** Run: `docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:latest -color .github/workflows/release-reporter.yaml .github/workflows/release-operators.yaml`. Expected: no findings.

- [ ] **Step 3: Dry-run on a fork, after Task 25 is merged into the branch.** Push tag `reporter/v0.2.0-rc.0`.
  - Expected: both gates are green, the release stays a prerelease with the Task 25 compose files attached, and the image manifest lists `linux/amd64` and `linux/arm64`.
  - If `GORELEASER_CURRENT_TAG` is refused, switch to the spec §14 fallback (snapshot build, `docker buildx imagetools create`, `gh` upload) and record that in the workflow comments.

- [ ] **Step 4: Commit** `ci(reporter): release the Go reporter from reporter/v* tags`

---

## After this plan

After `reporter/v0.2.0` is published, a separate change removes the TS reporter packaging listed in spec §12.3 step 2. The follow-up on the `solana.max-slot-lead` default (§14) remains a separate decision.
