# grpcsuite — exhaustive post-upgrade gRPC tx/query verification

`grpcsuite` exercises **every in-scope Akash and mounted Cosmos SDK transaction,
plus every in-scope query over the chain's gRPC API** after a network upgrade, so
that a passing run means the upgraded chain's API surface is verified accurate.
CLI is explicitly out of scope.

It runs in two places against the **same** code:

| Driver | Build tag | Target | Speed | Purpose |
| --- | --- | --- | --- | --- |
| `tests/upgrade` universal worker (`grpcsurface_worker_test.go`) | `e2e.upgrade` | testnetify-forked, freshly-upgraded validator | slow (full upgrade) | **acceptance path** — runs after every upgrade |
| `tests/fullsurface` (`fullsurface_test.go`) | `e2e.integration` | in-process single-validator `testutil/network` | minutes | fast local iteration + per-PR CI |

Run the fast path: `make test-grpc-surface`.

For narrower local debugging:
- `make test-grpc-surface-tx` runs the authored tx packs and gates tx coverage.
  Packs still call query RPCs for setup and assertions.
- `make test-grpc-surface-query` runs the dynamic query smoke sweep and gates
  query coverage without mutating chain state.

The acceptance path runs automatically inside `make -C tests/upgrade test` (the
existing `network-upgrade` CI job), because the suite is registered as the
**universal post-upgrade worker** (runs for every upgrade name).

## How it works

- **All checks run over gRPC.** Queries route through a gRPC-backed
  `client.Context` (`WithGRPCClient`); transactions are signed locally and
  broadcast via the cosmos `tx.ServiceClient`, then polled for inclusion — nothing
  touches Comet RPC. See `grpcconn.go`, `txbroadcast.go`.
- **Dynamic discovery + coverage gate (`coverage.go`).** The binary drives *what*
  is tested: gRPC server reflection lists the live Query services; the
  InterfaceRegistry lists the registered `Msg` implementations, restricted to the
  active served version. The gate fails if any in-scope Msg/query was never
  exercised — so a new RPC added by a future upgrade turns CI red until a case
  exists. This is what keeps "test every single one" self-maintaining.
- **Dynamic query smoke sweep (`smoke.go`).** Fires an empty request at every
  discovered query method, auto-covering the entire query surface and failing on
  any advertised-but-`Unimplemented` method. Authored query cases add correctness
  with real inputs.
- **Authored, dependency-ordered packs (`pack_*.go`).** A valid tx needs real
  prior state (lease ⇐ bid ⇐ order ⇐ deployment+provider), so transactions are
  authored scenarios, not fuzzed. Packs run in order and thread created handles
  through the shared `World`.
- **Governance fast-path (`gov.go`).** Gov-gated messages (every `MsgUpdateParams`,
  `MsgFundVault`, etc.) are batched into one proposal, voted through by the
  funder (who holds ~all voting power on a testnetify fork / single-validator
  net), and recorded once passed. Requires a short voting period (set in
  `tests/upgrade/testnet.json` and the in-process driver).

## Adding a module pack

1. Create `pack_<module>.go` implementing `Pack` (`Name`, `Available`, `Run`).
2. Gate `Available` on `d.HasModule("akash.<module>.<version>")` (Query service).
3. In `Run`, fund accounts via `s.FundAccountDefault`, build msgs with the SDK
   types, broadcast with `s.BroadcastOK` (happy path) or `s.BroadcastExpectErr`
   (negative / disabled), query with the generated `NewQueryClient(s.Conn)`.
4. For gov-gated msgs, build them with `Authority: s.GovAuthority()` and pass via
   `s.PassGovProposal(...)`.
5. Publish handles other packs need via `s.World.Set`; read with `s.World.Get`.
6. Register the pack in `pack.go` in dependency order.

The deployment, provider and gov-params packs are worked examples.

## Status

**Full in-scope coverage** — run `make test-grpc-surface` and read the
`coverage:` line:
- **Queries: 130/130 in-scope methods** (smoke sweep + authored typed cases).
- **Transactions: 76/76 in-scope messages.** Akash coverage includes deployment,
  provider, market, audit, escrow, cert, oracle, bme and module params. Cosmos SDK
  coverage includes auth, authz, bank, consensus, distribution, evidence,
  feegrant, gov v1, legacy gov v1beta1, mint, slashing, staking, upgrade and
  vesting.

`RequireFullCoverage` is `true` in both drivers, so the gate **fails** if any
in-scope tx or query stops being exercised — e.g. when a future upgrade adds a new
Akash or mounted Cosmos SDK RPC, until a case is authored for it.

The suite targets exactly the surface the running `main` binary serves: discovery
is driven by gRPC reflection + the interface registry, and every pack is
reflection-gated (`Available`/`HasModule`). A pack whose module is not served by
the binary under test is skipped automatically, so the suite always matches the
active surface with no manual bookkeeping.
