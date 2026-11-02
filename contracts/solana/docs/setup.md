# Setup and operations

This workspace is `contracts/solana` in the [Akash node repository](https://github.com/akash-network/node). Check out [chain-sdk](https://github.com/akash-network/chain-sdk) next to the node checkout, on the branch or tag that matches node's sink API.

## Toolchain

The stack builds on macOS Apple Silicon and Ubuntu 26.04 x86_64. Windows needs WSL2. The Linux host uses the official Anchor release described below. Allow several GB for caches and ledgers.

| Tool                     | Version                                                 |
| ------------------------ | ------------------------------------------------------- |
| Node.js                  | 24.x; `.nvmrc` pins 24.14.1                             |
| Go                       | 1.26.2                                                  |
| Rust through rustup      | 1.89.0, pinned in `rust-toolchain.toml`                 |
| Solana / Agave CLI       | 2.3.13, including `solana-test-validator`               |
| Anchor CLI               | 0.32.1                                                  |
| Native tools             | Git, Python 3, C compiler, make, curl, tar, patch, lsof |
| Optional generation tool | protoc, only when regenerating bindings                 |

On macOS, install Xcode tools with `xcode-select --install`. Homebrew can provide `python`, `cmake`, `pkg-config`, and `protobuf`. On Ubuntu, install `build-essential pkg-config libssl-dev libudev-dev libclang-dev clang cmake python3 curl git patch lsof protobuf-compiler`.

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

From `contracts/solana/`:

```bash
npm run doctor
npm run setup
export AKT_LOCAL_RUN=dev
npm run testnet:up
npm run testnet:demo
npm run testnet:verify
```

`doctor` checks versions and the node and chain-sdk checkouts. `setup` builds `chain-sdk/ts`, packs it with `npm pack` into `.cache/chain-sdk.tgz`, runs `npm ci`, and builds `.cache/bin/akash` in the node repository against `chain-sdk/go` and `chain-sdk/go/cli`. It then downloads and patches the pinned verifier, builds the programs and encoder, and checks TypeScript. The local npm archive is ignored and verified against `package-lock.json`; no registry publication is needed. Go SDK substitution lives in the ignored `node/.cache/solana.mod`; committed dependency files stay unchanged. `node/.cache/solana-sdk.json` records the SDK path used.

No direnv setup or separate wasm library download is needed for this local build. The Go wasmvm dependency includes native dynamic libraries. Advanced node build commands can still use direnv.

Custom paths are supported:

```bash
export NODE_SINK_DIR=/absolute/path/to/node
export CHAIN_SDK_DIR=/absolute/path/to/chain-sdk
npm run setup
```

Keep `NODE_SINK_DIR` set for runtime commands with a custom path. By default node is this repository and chain-sdk is `../chain-sdk` next to it. Do not use an SDK checkout containing unrelated changes.

The demo deposits 10 AKT and confirms its mint, then deposits 3 AKT to an invalid recipient and waits for a void/refund. A fresh successful run leaves 1,000,010 AKT on Solana and the same amount escrowed on Akash. Later runs add transfers. Health checks read finalized state and reconcile supply, escrow and independently indexed burns.

```bash
tail -f .cache/testnetify-stack-dev/relayer.log
cat .cache/testnetify-stack-dev/demo-result.json
cat .cache/testnetify-stack-dev/reconciliation.json
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

Rebuild native changes with `bash scripts/build-node.sh`, Solana changes with `npm run build`. A retained ledger contains its previously loaded programs. Stop it and choose a new paired run name to test rebuilt code, or explicitly upgrade the local programs. Never reset a checkpoint to hide a failure. The client expires after 24 hours without sufficient updates; preserve the old evidence and choose a new run name for a new experiment.

### Optional old-node upgrade rehearsal

The ordinary demo does not need the old binary. For upgrade tests:

```bash
git -C ../.. worktree add --detach .cache/upgrade-base 5193e54672e49ef328f67758ef5c91e24990c09a
bash scripts/build-node.sh ../../.cache/upgrade-base
npm run test:upgrade
```

Create the worktree only once. `AKASH_UPGRADE_BASE=/absolute/path/to/akash` selects an existing old binary instead. The rehearsal starts an old chain, passes a real upgrade proposal, swaps binaries, and checks funding and failure cases. It uses ports 30657/30656/9390 and writes `.cache/upgrades/`.

## Troubleshooting

For occupied ports, inspect `lsof -nP -iTCP:18999 -sTCP:LISTEN` and `npm run testnet:status`. Stop the owning stack, not unrelated listeners. For startup failures, inspect the named service's log under the selected run directory. A rebuild cannot repair an expired client. Interrupted testnetify conversion fails deliberately so the preserved seed can be inspected.

After restarting a retained Solana ledger, allow new blocks to finalize before
running the health check. A long-running validator can also prune early
transactions. If a reporter lacks its own stored vote and the original transaction
is unavailable, the full check fails even when supply reconciliation is healthy.
Increasing retention cannot recover deleted history. Preserve the old run for
investigation and use a new run name for a fresh experiment. Production requires
retained transaction history and indexed evidence.
