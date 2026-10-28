# AKT on Solana

The Solana side of Akash Sink, a one-way transfer of AKT from Akash to Solana.
It contains the AKT mint program, patches to the upstream Tendermint light
client, the relayer, the reporters and scripts to run both chains locally. The
Akash side is the [`x/sink` module](../../x/sink/README.md) in this repository.

Genesis creates the AKT mint with zero supply. Every mint requires an
authenticated Akash state proof of a deposit. The demo exercises a deposit,
mint, confirmation, invalid-recipient void and refund.

The transfer portal lives in its own repository.

## Build and run

Install the [pinned toolchain](docs/setup.md), then check out
[chain-sdk](https://github.com/akash-network/chain-sdk) next to this node
checkout, on the branch or tag that matches node's sink API:

```bash
git clone https://github.com/akash-network/node.git
git clone https://github.com/akash-network/chain-sdk.git
cd node/contracts/solana
npm run setup
export AKT_LOCAL_RUN=dev
npm run testnet:up
npm run testnet:demo
npm run testnet:verify
```

`NODE_SINK_DIR` and `CHAIN_SDK_DIR` override the node and chain-sdk paths.
`setup` installs JS dependencies and builds the Akash node with
`scripts/build-node.sh`, the upstream verifier, the AKT program and the header
encoder. First builds take several minutes. It does not start a chain or send a
public-chain transaction.

`testnet:up` starts four Akash validators, one Solana test validator, a relayer,
three reporters and a reconciliation monitor. It creates development keys
automatically. Initial startup and the demo each take a few minutes while signed
headers and transfer outcomes reach finalized Solana state.

```bash
npm run testnet:status   # process status, chain heights and balances
npm run testnet:down     # stop; preserve keys and ledgers
npm run testnet:up       # resume the same network
```

Keep `AKT_LOCAL_RUN=dev` set in each terminal. Akash RPC is
`http://127.0.0.1:27657`; Solana RPC is `http://127.0.0.1:18999`. Logs and
results are under `.cache/testnetify-stack-dev/`. See
[setup and troubleshooting](docs/setup.md) and
[testnet scenarios](docs/testnetify.md).

The sink schema, generated clients and CLI live in chain-sdk. The node executes
the handlers and exposes the generated REST routes; the reporters consume the
SDK package. [SDK integration notes](docs/sdk-integration.md) cover ownership,
generation and local dependency updates.

## Guides

| Guide                                                                                          | Brief synopsis                                                                        |
| ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| [Setup](docs/setup.md)                                                                         | Pinned tools, clean builds, paths, ports and troubleshooting.                         |
| [Testing](docs/reproduce.md)                                                                   | Ordered commands and expected outcomes for every suite.                               |
| [Mint implementation](docs/implementation.md)                                                  | Canonical mint identity, zero-supply genesis and authority rules.                     |
| [Bridge protocol](docs/bridge-protocol.md)                                                     | How signed Akash headers and ICS23 proofs authorize mint or void.                     |
| [Settlement rules](docs/sink-reporting.md)                                                     | Record-ID tracking, escrow and refunds on Akash.                                      |
| [Reporters](docs/reporter.md)                                                                  | Finalized Solana evidence, signed Akash reports and failure handling.                 |
| [Operators](docs/operators.md)                                                                 | Running an independent relayer against any deployment.                                |
| [Local stack](docs/local-stack.md) / [four-validator rehearsal](docs/testnetify.md)            | Both chains, relayer, reporters, reconciliation and live fault scenarios.             |
| [100-validator results](docs/header-scaling.md) / [verifier patches](docs/verifier-patches.md) | Signature batches, consensus-state pruning, runtime costs and recovery checks.        |
| [Audit package](docs/audit.md)                                                                 | Scope, trust model, instructions, accounts, invariants and accepted risks.            |
| [Deploying the programs](docs/deploy-program.md)                                               | Launch commands: program IDs, verifiable build, deploy, multisig handover and checks. |
| [Production deployment](docs/production-deployment.md)                                         | Testnet shortcuts, launch blockers, authorities, parameters and launch order.         |
| [Hosting](docs/hosting.md)                                                                     | Hosts, versions, ports, monitoring and the launch order for DevOps.                   |
| [Token metadata](docs/token-metadata.md)                                                       | Metaplex metadata, hosting and the mainnet launch gate.                               |
