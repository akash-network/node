# x/sink integration decisions

Status as of 2026-10-19, for maintainers and agents preparing x/sink for a
mainnet release. This file is its own commit; drop it before opening an
upstream pull request if it is not wanted there. Do not act on an Open item
without its owner; record each decision here with its date.

## What this branch contains

Based on `main` at `5193e546`:

1. `feat(x/sink): add sink module`: keeper, message and query servers,
   genesis, open-transfer index, bank send restriction, ante decorator, pause
   guardian, amount caps, and the install helpers `PrepareInstall` and
   `FundLaunchReserve`.
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

- Store name `sink`. Transfer records at `transfer/{id}` and governance void
  orders at `void/{id}`, IDs in decimal without leading zeros.
- The 77-byte transfer record: version `1` (1 byte), ID (u64 LE), sender
  address bytes (20), Solana recipient (32), amount in uakt (u64 LE), creation
  height (u64 LE). Records are written once and never rewritten.
- The 9-byte void order: version `1`, then the ID (u64 LE).
- Transfer IDs are global, nonzero and never reused. Never reset or replace
  this store on a chain the Solana program already trusts.
- Protobuf field numbers in `akash.sink.v1`.
- The sink module address stays a blocked bank address; only `MsgSink`
  deposits into it, and only `FinalizeRefund` and the reserve funding move
  coins in or out.

## Decisions needed

| ID  | Decision            | Context                                                                                                                                  | Recommendation                                                                  | Owner                 | Status |
| --- | ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- | --------------------- | ------ |
| A1  | Upgrade name/height | `upgrades/software/v2.2.0-sink-provisional` is a placeholder in upstream's layout                                                       | Rename it, or fold the handler into the release's own upgrade                   | Akash maintainers     | Open   |
| A2  | Reserve source      | Must hold 1,000,000 AKT, liquid, at the upgrade height, or the upgrade fails and the chain halts. Balances already at the sink address go to the community pool | Name the account in the proposal and state the community-pool disposition; add `reserve_source` to the CI upgrade test plan info | Akash governance      | Open   |
| A3  | chain-sdk release   | The `go.mod` commit is a placeholder until chain-sdk tags `pkg.akt.dev/go` and `pkg.akt.dev/go/cli` with sink                           | Tag chain-sdk, then replace commit 5 with `go get` of the tags and `go mod tidy` | chain-sdk maintainers | Open   |
| A4  | Amino names         | `sink/MsgSink` and `sink/MsgGuardianPause` differ from bme's `akash/bme/v1/...` style; changing them changes wallet sign bytes           | Decide before any wallet integration ships                                      | Akash maintainers     | Open   |
| A5  | Default parameters  | Deposits and refunds off, no reporters, `min_transfer` 0, no pause guardian, no per-account amount cap                                  | Keep; governance sets values after the upgrade                                  | Akash maintainers     | Open   |
| A6  | Previous handler    | Upstream usually drops the previous upgrade handler (`v2.1.0`) when adding the next                                                     | Follow the usual release practice                                               | Akash maintainers     | Open   |

Governance parameters to set after installation (reporters and threshold,
challenge period, caps, minimum transfer, pause guardian) are chosen together
with the Solana-side launch; ask the Solana-side owners for the agreed values.

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
