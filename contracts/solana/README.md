# AKT on Solana

The mainnet bridge is deployed. Use these entry points:

| Task                                                            | Reference                                                                                      |
| --------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Contract launch, administration, upgrades and incident response | [Canonical deployment guide](docs/deploy-program.md)                                           |
| Deployed binaries, IDLs, source pins and historical evidence    | [Mainnet artifacts](deployments/mainnet/README.md)                                             |
| Public program, mint and multisig addresses                     | [Mainnet reference](https://akash.network/docs/learn/akash-sink/mainnet/)                      |
| Install or upgrade relayers and reporters                       | [Operator guides](https://akash.network/docs/node-operators/akash-sink/)                       |
| Native Akash upgrade                                            | [Sink deployment](../../_docs/sink-deployment.md)                                              |
| Program design and verification                                 | [Implementation](docs/implementation.md), [verifier](docs/verifier.md), [audit](docs/audit.md) |
| Protocol, proof and service recovery details                    | [Sink reference](docs/sink-migration-v2.md)                                                    |

Run development commands from `contracts/solana`. Local test networks and fresh
program namespaces are separate from the existing mainnet deployment.

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

- AKT: `../../.cache/solana/target/deploy/akt.so`
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
