# Sink SDK integration

The sink API follows the existing Akash module layout. `chain-sdk` owns the schema,
generated clients, shared CLI, codec registration, validation and proof encoding.
`node` owns the keeper, bank restriction, transaction handlers and app wiring.
`node/contracts/solana` owns the Solana contracts, relaying and reporters. The
portal lives in its own repository.

| Work                       | Location                                         |
| -------------------------- | ------------------------------------------------ |
| Protocol schema            | `chain-sdk/proto/node/akash/sink/v1/`            |
| Go API and validation      | `chain-sdk/go/node/sink/v1/`                     |
| Go query facade            | `client.Query().Sink()`                          |
| CLI                        | `chain-sdk/go/cli/sink_{tx,query}.go`            |
| TypeScript SDK             | `sdk.akash.sink.v1.sink()` and `getSink()`       |
| TypeScript message exports | `@akashnetwork/chain-sdk/private-types/akash.v1` |
| Native execution           | `node/x/sink/keeper/`                            |
| REST registration          | `node/x/sink/module.go`                          |

Record queries are available at `/akash/sink/v1/records/{id}`. State, immutable
transfer details and account usage use `/akash/sink/v1/state`,
`/akash/sink/v1/transfers/{id}` and `/akash/sink/v1/accounts/{sender}`.
Malformed IDs return HTTP 400; missing records return HTTP 404.
`/akash/sink/v1/open_transfers?start_after=&limit=` pages every transfer that is
neither minted nor refunded, in ID order (`akash query sink open-transfers`).
Reporters and relayers use it instead of reading every record.

The governance-set `pause_guardian` signs `MsgGuardianPause`
(`akash tx sink guardian-pause --deposits --refunds`). It can only turn deposits
and/or refunds off; `shared/sink.ts` encodes it for wallets, including Amino.

`MsgSinkResponse.id` is an included-transaction result. Synchronous CLI broadcast
still returns a transaction hash, following the ordinary SDK flow. The SDK's
TypeScript transaction method decodes the typed response. The portal retains its
hash before broadcasting and resolves the ID after inclusion, including recovery
from an interrupted response or page reload.

## Development cycle

Change the SDK schema, regenerate with its existing local Go/TypeScript plugins,
then update node handlers and SDK consumers together. Do not copy generated
files into node.

`npm run setup` installs and builds the chain-sdk checkout, creates a local
package with `npm pack`, then installs this workspace with `npm ci`. The archive lives under `.cache`
and is ignored. Its integrity is pinned in `package-lock.json`. Nothing is
published to npm. Both Go SDK modules use the configured local checkout through
the node helper's ignored modfile.

After changing SDK TypeScript output, refresh its local package and lockfile from
`contracts/solana`:

```bash
source scripts/workspace.sh
npm ci --prefix "$CHAIN_SDK_DIR/ts"
npm run build --prefix "$CHAIN_SDK_DIR/ts"
mkdir -p .cache
sdk_package=$(cd "$CHAIN_SDK_DIR/ts" && npm pack --silent --pack-destination "$solana_repo/.cache")
mv ".cache/$sdk_package" .cache/chain-sdk.tgz
npm install --save @akashnetwork/chain-sdk@file:.cache/chain-sdk.tgz
bash scripts/build-node.sh
npm run typecheck
npm run test:reporter
```

Commit the SDK change in chain-sdk and the consumers and package lock in node
together. The archive and runtime evidence stay out of Git.
SDK 64-bit values use `Long`; SDK `fromPartial` accepts bigint inputs. Use explicit
numeric conversions at app boundaries so IDs, amounts, epochs and slots retain
all bits.

Run the shared SDK checks from `contracts/solana` after setup:

```bash
source scripts/workspace.sh
(cd "$CHAIN_SDK_DIR/go" && GOWORK=off go test -mod=readonly ./node/sink/v1 ./node/client/... -count=1)
(cd "$CHAIN_SDK_DIR/go/cli" && GOTOOLCHAIN=go1.25.1 GOWORK=off go test -mod=readonly . -run TestSinkQuerySDKCommand -count=1)
npm run test:unit --prefix "$CHAIN_SDK_DIR/ts" -- src/sdk/chain
```

The standalone CLI check uses its pinned Go 1.25.1 toolchain. Its existing Sonic
dependency does not compile under Go 1.26. The node build uses the node's updated
dependency graph and passed with Go 1.26.

## Audit corrections

This integration removes the node-owned schema, app-owned generated bindings,
separate protoc scripts, node-only CLI registration, and the empty REST gateway.
It adds sink to the normal Go query facade and discovery metadata. Existing
message names, field numbers, Amino names and immutable proof bytes are preserved.
The Amino adapter in `shared/sink.ts` remains because wallet signing needs that
conversion; it uses the SDK message types.

## Verification, 2026-10-14

The SDK Go API/client tests, shared CLI query test, native sink/app/CLI suites,
14 SDK TypeScript tests, 31 portal/backend tests and 22 reporter tests passed.
The reporter tests cover 64-bit values above JavaScript's safe integer range.
Typechecking, formatting, SDK export validation and the portal build passed.
Regenerating the SDK left all 664 generated Go/TypeScript files unchanged.
Two independent SDK builds produced identical npm archives matching the
consumer lockfile, and a fresh `npm ci` accepted that archive.

The proof suite passed all 18 Solana checks and the native signed-report round
trip. It used an isolated validator on port 19997 while the live stack remained
on 8899. A real 452-byte Akash proof minted using 133,486 compute units; altered
amounts and duplicate mint attempts rejected. These fixture proofs use an
explicitly trusted test root. The live demo separately authenticates signed
Akash headers.

The rebuilt native REST gateway and shared SDK CLI returned existing records
and rejected malformed or missing IDs. The browser checks passed at desktop
and mobile sizes. The live browser check submitted a real signed transaction,
recovered after a dropped broadcast reply and reload, and queried record 7 by
ID alone. It uses mock wallet adapters; real extension popups and hardware
wallets still need manual acceptance testing.

The sequential live demo minted 10 AKT for record 8 and voided/refunded 3 AKT for
record 9. Final supply and escrow both equal 1,000,042 AKT, including the intact
1,000,000 AKT launch reserve. No transfers or refunds remain pending, and the
reconciler reports no alerts. See `.cache/live-stack-msgsink/demo-result.json`
and `reconciliation.json`. Run the demo and live browser check sequentially;
both spend from the local faucet's sender, so concurrent runs invalidate the
demo's balance assertions. That earlier overlapping run and an initial
stack-not-ready attempt are retained in the evidence directory. The isolated
proof suite's first attempt also retained its faucet-port conflict log before
rerunning successfully on separate ports.

Local evidence is ignored under `.cache/sdk-integration/` and
`.cache/bridge-ui/verification/2026-10-14T20-26-28-112Z-live/`.
