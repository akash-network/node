# AKT on Solana

Build and deploy the AKT program, the IBC light client, and its access manager.
Run commands from `contracts/solana` unless stated otherwise.

## Contents

- [Build](#build)
- [Deploy](#deploy)
- [Start a Solana testnet](#start-a-solana-testnet)
- [Code walkthrough](docs/implementation.md)
- [Program deployment guide](docs/deploy-program.md)

## Build

Install Node 24, Go 1.26.2, Rust 1.89.0, Solana CLI 2.3.13, and Anchor 0.32.1.
[Tool installation](docs/setup.md#toolchain) lists the required native packages.

Place the reviewed chain-sdk package at `../../.cache/solana/chain-sdk.tgz`.
Use the version and archive described in [SDK integration](docs/sdk-integration.md).

```bash
npm ci
npm run setup:light-client
npm run build
```

The build writes:

- AKT: `../../.cache/solana/target/deploy/akt_solana.so`
- IBC verifier: `../../.cache/solana/deps/ibc-contracts/ibc-solana/target/deploy/ics07_tendermint.so`
- Access manager: `../../.cache/solana/deps/ibc-contracts/ibc-solana/target/deploy/access_manager.so`

IDLs and TypeScript types are generated beside each program's `deploy` directory.
The IBC build uses the merged repository at the committed Cargo pin.

Check the build:

```bash
npm run test:unit
npm run typecheck
npm run test:services
```

## Deploy

For a new production deployment, set the production program IDs and commit the
changes using [the launch instructions](docs/deploy-program.md#2-launch-commit).
The current account layout requires fresh Solana state and an unused native
Sink module.

Build the committed release with Docker:

```bash
source scripts/workspace.sh
export RELEASE_DIR="$SOLANA_CACHE/releases/$(git rev-parse HEAD)"
npm run build:verifiable -- --rev "$(git rev-parse HEAD)" "$RELEASE_DIR/build"
cp deploy/manifest.example.json "$RELEASE_DIR/manifest.json"
```

Fill in the manifest with the reviewed program IDs, authorities, commit, and
build hashes. Set `buildDirectory` to `build`. Set `RPC` to the target cluster.

```bash
export RPC=https://your-solana-rpc.example
npm run deploy:programs -- check --manifest "$RELEASE_DIR/manifest.json"
npm run deploy:programs -- preflight --manifest "$RELEASE_DIR/manifest.json" --url "$RPC"
```

Deploy the three programs with the Solana CLI, then initialize them, transfer
the upgrade authorities, and configure the bridge. Follow
[the deployment guide](docs/deploy-program.md) in order, keeping the same
`RELEASE_DIR`. It includes the commands and checks for each transaction.

After the ceremony, verify the deployed bytes and authorities:

```bash
npm run deploy:programs -- verify --manifest "$RELEASE_DIR/manifest.json" --url "$RPC" --authority multisig
```

[Service installation](docs/deploy-services.md) covers starting the relayer and
reporters against the deployed programs.

## Start a Solana testnet

Start a local Solana validator:

```bash
npm run setup:metadata
npm run localnet
```

The RPC is `http://127.0.0.1:17899`. The validator stays in the foreground;
Ctrl-C stops it and preserves its ledger. Restart with the same command.

This starts Solana with Metaplex loaded. Deploy AKT, the IBC verifier, and the
access manager before using the bridge. To exercise the contracts with all
programs loaded automatically, run this in another terminal:

```bash
npm test
```

The tests use a separate validator on port 19899 and stop it when they finish.
To run transfers with an Akash chain, relayer, and reporters, follow
[the local bridge instructions](docs/local-stack.md#start-and-exercise-it).
