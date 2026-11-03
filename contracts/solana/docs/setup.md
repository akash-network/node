# Setup and operations

This page prepares a development workspace and test networks. Production
operators should start with [production deployment](production-deployment.md)
and [service installation](deploy-services.md).

This workspace is `contracts/solana` in the [Akash node repository](https://github.com/akash-network/node).
Akash uses its existing root Makefile, direnv environment and `go.mod` pins.
Solana programs use Anchor and Cargo. The verifier is a pinned commit of
[akash-network/ibc-contracts](https://github.com/akash-network/ibc-contracts/tree/0da3165d8428a1b7792cc17551ecd1d3985761e6).

## Toolchain

The stack builds on macOS Apple Silicon and Ubuntu 26.04 x86_64. Windows needs WSL2. The Linux host uses the official Anchor release described below. Allow several GB for caches and ledgers.

| Tool                     | Version                                                      |
| ------------------------ | ------------------------------------------------------------ |
| Node.js                  | 24.x; `.nvmrc` pins 24.14.1                                  |
| Go                       | 1.26.2                                                       |
| Rust through rustup      | 1.89.0, pinned in `rust-toolchain.toml`                      |
| Solana / Agave CLI       | 2.3.13, including `solana-test-validator`                    |
| Anchor CLI               | 0.32.1                                                       |
| Native tools             | Git, Python 3.11+, C compiler, direnv, make, curl, tar, lsof |
| Optional generation tool | protoc, only when regenerating bindings                      |

On macOS, install Xcode tools with `xcode-select --install`. Homebrew can provide `python`, `cmake`, `pkg-config`, and `protobuf`. On Ubuntu, install `build-essential pkg-config libssl-dev libudev-dev libclang-dev clang cmake python3 curl git direnv lsof protobuf-compiler`.

Install [Node 24](https://nodejs.org/en/download) and [Go 1.26.2](https://go.dev/dl/). With nvm installed, run `nvm install && nvm use` inside `contracts/solana/`. With another recent Go version installed, `export GOTOOLCHAIN=go1.26.2` selects and downloads the required Go toolchain.

Follow the official [Anchor installation guide](https://www.anchor-lang.com/docs/installation) and [Agave installation guide](https://docs.anza.xyz/cli/install), choosing the pinned versions. After installing rustup:

```bash
. "$HOME/.cargo/env"
rustup toolchain install 1.89.0 --component rustfmt --component clippy
sh -c "$(curl -sSfL https://release.anza.xyz/v2.3.13/install)"
export PATH="$HOME/.local/share/solana/install/active_release/bin:$HOME/.cargo/bin:$PATH"
bash scripts/install-anchor.sh
```

The local network preloads programs at fixed development IDs. It does not need the original program keypair, an external funded wallet, or a change to your global Solana RPC.

Use the official Anchor 0.32.1 release installed by the script or AVM.
The script checks the release SHA-256. A `cargo install --locked` build of the
same version uses an older IDL dependency that failed on the clean Linux host
with `solana-packet` serde errors. The release binary matches the tested
macOS setup and avoids that compiler flag mismatch.

## Build and run

Initialize node's normal [direnv environment](../../../.envrc) first:

```bash
# From the node repository root.
direnv allow
make bins
```

Supply the reviewed TypeScript SDK archive described in
[SDK integration](sdk-integration.md), then from `contracts/solana/`:

```bash
npm run doctor
npm run setup
```

`setup` runs `npm ci`, `make bins`, the pinned IBC builds, the AKT Anchor build,
the header encoder build and TypeScript checking. It does not rewrite SDK
packages, Go dependency files or build tags. The normal binary is `$AKASH`
(default `node/.cache/bin/akash`). `test:sink` honors the same Go workspace
selection as ordinary Go commands and never discovers a cached modfile.

### Local SDK development

Only if you are changing node and the Go SDK together, use Go's existing
workspace support. From the node root, create or update the ignored `go.work`:

```bash
# Use `go work use` alone if go.work already exists.
go work init .
go work use ../chain-sdk/go ../chain-sdk/go/cli
export GOWORK="$PWD/go.work"
make bins
```

Use the actual SDK checkout paths. Keep that workspace local; production builds
and release qualification use `GOWORK=off make bins` and the committed SDK pins.
A missing or incompatible published pin is a release blocker, not a reason to
substitute another module tree during release builds.

### Accelerated testnet

The local genesis asks for a 20-second challenge. Select `sinkdev` explicitly
using the existing Makefile's build-tag argument and give the output a distinct
path. From the node root, after `npm run setup`:

```bash
mkdir -p .cache/solana/bin
make bins AKASH="$PWD/.cache/solana/bin/akash-sinkdev" \
  BUILD_TAGS='osusergo netgo ledger muslc gcc sinkdev'
export AKASH="$PWD/.cache/solana/bin/akash-sinkdev"
cd contracts/solana
export AKT_LOCAL_RUN=dev
npm run testnet:up
npm run testnet:demo
npm run testnet:verify
```

Restore `AKASH` and `AKT_LOCAL_RUN` in a new shell before resuming this testnet.
Production and mainnet-fork qualification use the ordinary build without
`sinkdev`. An accelerated test run does not qualify that build.

### Generated files

| Path                              | Contents                                                         |
| --------------------------------- | ---------------------------------------------------------------- |
| `.cache/bin/akash`                | Normal Akash Makefile output                                     |
| `.cache/solana/bin/akash-sinkdev` | Explicit accelerated-testnet build                               |
| `.cache/solana/chain-sdk.tgz`     | Reviewed TypeScript package, locked by npm integrity             |
| `.cache/solana/deps/`             | Pinned IBC checkout, its Anchor output and Metaplex test program |
| `.cache/solana/target/`           | AKT Cargo/Anchor output and header encoder                       |
| `.cache/solana/run/`              | Local networks and tests, including private development keys     |

Cargo also uses its normal user-level registry/git/toolchain caches; npm installs
`node_modules` beside `package.json`. The fixed release container uses its own
build and toolchain directories. These are separate from project scratch.

If setup finds the old patched IBC archive, move that exact directory aside under
`.cache/solana` before retrying. The fetcher preserves existing sources and rejects
a different revision or dirty checkout. Do not remove ledgers or keys as a build
repair.

`NODE_SINK_DIR=/absolute/path/to/node` selects another node source for fixture
tests. Load that checkout's normal build environment and set `AKASH` to its
Makefile output for runtime commands.

The demo deposits 10 AKT and confirms its mint, then deposits 3 AKT to an invalid recipient and waits for a void/refund. A fresh successful run leaves 1,000,010 AKT on Solana and the same amount escrowed on Akash. Later runs add transfers. Health checks read finalized state and reconcile supply, escrow and independently indexed burns.

```bash
tail -f ../../.cache/solana/run/testnetify-stack-v2-dev/relayer.log
cat ../../.cache/solana/run/testnetify-stack-v2-dev/demo-result.json
cat ../../.cache/solana/run/testnetify-stack-v2-dev/reconciliation.json
npm run testnet:status
npm run testnet:down
```

Stopping preserves ledgers. A new shell must restore `AKT_LOCAL_RUN=dev`. Run lifecycle commands and scenarios sequentially. Run names share ports; stop the current profile before using another name. The single-validator alternative is `npm run local:up` on ports 26657 and 8899.

## Checks and iteration

```bash
npm run typecheck
npm run test:unit
npm test
npm run test:reporter
npm run test:durable
npm run testnet:scenarios
npm run testnet:verify
```

`testnet:scenarios` changes validator availability, pauses minting and exercises governance. Run it after the demo settles. `bench:headers`, `test:receipts`, `test:recovery`, `test:misbehaviour` and `test:expiry` each use an isolated validator on port 19999; run them separately. See [the test guide](reproduce.md) for all suites, their expected results and evidence paths.

Rebuild native changes with `make bins` from the node root, Solana changes with `npm run build`. A retained ledger contains its previously loaded programs. Stop it and choose a new paired run name to test rebuilt code, or explicitly upgrade the local programs. Never reset a checkpoint to hide a failure. The client expires after 24 hours without sufficient updates; preserve the old evidence and choose a new run name for a new experiment.

### Native upgrade tests

Use node's existing upgrade harness and the registered release handler; see
[existing-chain upgrade checks](reproduce.md#5-existing-chain-upgrade).

## Troubleshooting

For occupied ports, inspect `lsof -nP -iTCP:18999 -sTCP:LISTEN` and `npm run testnet:status`. Stop the owning stack, not unrelated listeners. For startup failures, inspect the named service's log under the selected run directory. A rebuild cannot repair an expired client. Interrupted testnetify conversion fails deliberately so the preserved seed can be inspected.

After restarting a retained Solana ledger, allow new blocks to finalize before
running the health check. A long-running validator can also prune early
transactions. If a reporter lacks its own stored vote and the original transaction
is unavailable, the full check fails even when supply reconciliation is healthy.
Increasing retention cannot recover deleted history. Preserve the old run for
investigation and use a new run name for a fresh experiment. Production requires
retained transaction history and indexed evidence.

The sink service and wire contract is now version 2. Follow [launch and recovery](sink-migration-v2.md) before running these commands against a new namespace. Existing numeric-ID deployments remain separate.
