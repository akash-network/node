# x/sink integration decisions

Status as of 2026-10-19, for maintainers and agents preparing x/sink for a
mainnet release. This file is its own commit; drop it before opening an
upstream pull request if it is not wanted there. Do not act on an Open item
without its owner; record each decision here with its date.

## What this branch contains

Based on `main` at `5193e546`:

1. `feat(x/sink): add sink module`: keeper, message and query servers,
   genesis, open-transfer index, bank send restriction, ante decorator, pause
   guardian, amount caps, and the install helper `PrepareInstall`.
2. `feat(app): wire x/sink into the app`: store key, keeper, module ordering,
   blocked module address, send restriction.
3. `test(x/sink): cover deposits, settlement and refunds`.
4. `feat(upgrades): add provisional x/sink install upgrade`.
5. `build(deps): prepare go.mod for the chain-sdk sink api`: a placeholder.

The protobuf API, Go types, validation and CLI live in chain-sdk's matching
`feat/sink` branch.

## Compatibility constraints

A Solana program verifies x/sink state with ICS23 membership proofs of this
store. Changing any of the following breaks minting and needs a coordinated
change on the Solana side:

- Store name `sink`. Transfer records at `0x02 0x00` and governance void orders
  at `0x02 0x01`, each followed by the ID as an 8-byte big-endian integer
  (`TransferKey` and `VoidKey` in the SDK).
- The 77-byte transfer record: version `1` (1 byte), ID (u64 LE), sender
  address bytes (20), Solana recipient (32), amount in uakt (u64 LE), creation
  height (u64 LE). Records are written once and never rewritten.
- The 9-byte void order: version `1`, then the ID (u64 LE).
- Transfer IDs are global, nonzero and never reused. Never reset or replace
  this store on a chain the Solana program already trusts.
- Protobuf field numbers in `akash.sink.v1`.
- The sink module address stays a blocked bank address; only `MsgSink`
  deposits into it, and only `FinalizeRefund` moves coins out.

## Decisions needed

| ID  | Decision            | Context                                                                                                                                  | Recommendation                                                                  | Owner                 | Status |
| --- | ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- | --------------------- | ------ |
| A1  | Upgrade name/height | `upgrades/software/v2.2.0-sink-provisional` is a placeholder in upstream's layout                                                       | Rename it, or fold the handler into the release's own upgrade                   | Akash maintainers     | Open   |
| A2  | Launch reserve      | Removed 2026-10-26. Solana launches with no genesis mint, so nothing on Akash backs a supply that does not exist. The upgrade needs no funding account, and plan info is standard cosmovisor info | None | Akash governance      | Closed |
| A3  | chain-sdk release   | The `go.mod` commit is a placeholder until chain-sdk tags `pkg.akt.dev/go` and `pkg.akt.dev/go/cli` with sink                           | Tag chain-sdk, then replace commit 5 with `go get` of the tags and `go mod tidy` | chain-sdk maintainers | Open   |
| A4  | Amino names         | `sink/MsgSink` and `sink/MsgGuardianPause` differ from bme's `akash/bme/v1/...` style; changing them changes wallet sign bytes           | Decide before any wallet integration ships                                      | Akash maintainers     | Open   |
| A5  | Default parameters  | Deposits and refunds off, no reporters, `min_transfer` 0 (allowed only while deposits are off), 72-hour challenge period, no pause guardian, no per-account amount cap. Caps since 2026-10-26: `max_transfer` 1,000,000 AKT, `solana_daily_mint_cap` 10,000,000 AKT, `daily_refund_cap` 1,000,000 AKT | Keep; governance sets values after the upgrade                                  | Akash maintainers     | Open   |
| A6  | Previous handler    | Upstream usually drops the previous upgrade handler (`v2.1.0`) when adding the next                                                     | Follow the usual release practice                                               | Akash maintainers     | Open   |
| A7  | Challenge floor tag | Release builds require a 72-hour challenge period. The `sinkdev` build tag lowers the floor to 1 second for test networks              | Keep; never build a release with `sinkdev`                                      | Akash maintainers     | Open   |

Governance parameters to set after installation (reporters and threshold,
challenge period, caps, minimum transfer, pause guardian) are chosen together
with the Solana-side launch; ask the Solana-side owners for the agreed values.

## Audit fixes, 2026-10-22

The x/sink audit of `5193e546..4d21e5a0` found no critical or high findings.
This branch fixes:

| Finding | Fix |
| ------- | --- |
| M-01 | Superseded 2026-10-26: the handler no longer reads plan info (A2) |
| M-02 | Refunds are paid oldest first from a derived refund queue |
| M-03 | Holds stay for governance; `open-transfers --held-only` lists them for one restart proposal. Runbook: guardian pause, then one proposal that removes the reporter and restarts its holds |
| M-04 | Governance can void a Minted transfer under a conflict hold, after a void order; the amount must fit the refund cap |
| L-01 | `PrepareInstall` deletes withdraw addresses set to the sink address |
| L-02 | Confirming a void requires a void order |
| L-03 | Challenge period at least 72 hours (A7); a minimum transfer is required while deposits are enabled |
| L-04 | A lowered refund cap must cover the largest open transfer |
| I-01, I-09 | Refunds record state before paying and check the payment reached the sender |
| I-02 | Superseded 2026-10-26: the launch reserve is gone (A2) |
| I-03 | Destinations must be canonical, non-small-order points (or listed vaults) |
| I-04 | Tallies count only minted and voided votes |
| I-08 | `sink_deposit` carries recipient and height |
| I-10 | Genesis rejects future refund and usage days |
| I-11 | Superseded 2026-10-26: the launch reserve is gone (A2) |
| G-01 to G-05 | Address computed once, params read once, bucket switch, totals written once, settlement reads skip the void key |

`app/sink_audit_test.go` holds a regression test for each behavior change.

Left for owners, because each changes the design or needs new protobuf fields:

| Finding | Question |
| ------- | -------- |
| I-05 | Add a governance message that sweeps a surplus at the sink address to the community pool |
| I-06 | Add a chain-wide daily deposit cap next to the per-account limits |
| I-07 | Bump the reporter epoch only when a member is removed or the threshold rises |
| I-12 | Measure the challenge window in blocks as well as seconds |
| M-03 | Keep held transfers out of the per-account pending limit with a separate counter |

## How to test

Until chain-sdk is tagged, build against a chain-sdk checkout of its
`feat/sink` branch with a scratch modfile:

```bash
cp go.mod .cache/sink.mod && cp go.sum .cache/sink.sum
go mod edit -modfile=.cache/sink.mod \
  -replace pkg.akt.dev/go=<chain-sdk>/go \
  -replace pkg.akt.dev/go/cli=<chain-sdk>/go/cli
go mod tidy -modfile=.cache/sink.mod
go test -modfile=.cache/sink.mod ./x/sink/... ./app/... ./upgrades/...
```

The full test suite and the app simulations (full, nondeterminism,
import-export, after-import) passed this way on 2026-10-19.
