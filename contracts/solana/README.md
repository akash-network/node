# AKT on Solana

This workspace contains the Solana programs and operator services for Akash
Sink, a one-way transfer of AKT from Akash to Solana. The AKT mint starts with
zero supply. Each mint requires an authenticated proof of an Akash deposit;
reporters observe finalized Solana outcomes and settle them on Akash.

The Akash implementation is [`x/sink`](../../x/sink/README.md). Schema, generated
clients and CLI ownership are described in [SDK integration](docs/sdk-integration.md).
The transfer portal is maintained in its own repository.

## Production deployment

Start with the [production runbook](docs/production-deployment.md). It links the
program ceremony, service installation, activation checks and incident response.
Production uses `akashnet-2` and Solana mainnet-beta, fresh program IDs, a reviewed
release manifest, and separate ceremony, multisig, relayer and reporter keys.

| Task                                        | Entry point                                                      |
| ------------------------------------------- | ---------------------------------------------------------------- |
| Build committed program artifacts           | `npm run build:verifiable -- --rev COMMIT OUTPUT`                |
| Check a reviewed release offline            | `npm run deploy:programs -- check --manifest FILE`               |
| Check the cluster and existing programs     | `npm run deploy:programs -- preflight --manifest FILE --url RPC` |
| Deploy and verify program bytes             | [Program deployment](docs/deploy-program.md)                     |
| Build and install relayers/reporters        | [Service deployment](docs/deploy-services.md)                    |
| Emit reviewed multisig instructions         | `npm run admin -- COMMAND ...`                                   |
| Test deployment guards without transactions | `npm run test:deployment`                                        |

The manifest template intentionally contains invalid placeholders. Fill it from
independently reviewed build output and ceremony public keys. Neither a passing
preflight nor a successful upload establishes launch readiness: initialize the
programs, transfer authorities, configure the checkpoint and policy, validate the
services, and complete the activation gates. No mainnet deployment is claimed.

## Development and testnets

Follow [development setup](docs/setup.md) to build with the existing Akash
Makefile, install the locked SDK package, and build the pinned IBC programs.
Accelerated refund tests require an explicitly selected `sinkdev` binary.
The ordinary build retains the production challenge-period floor.

`testnet:up` creates development keys and starts both test chains and services.
`testnet:down` preserves keys and ledgers for restart. See
[local stack](docs/local-stack.md), [four-validator testnet](docs/testnetify.md),
and [test suites](docs/reproduce.md).

## Reference

- [Protocol and proofs](docs/bridge-protocol.md), [mint implementation](docs/implementation.md),
  [settlement rules](docs/sink-reporting.md), [canonical launch/recovery](docs/sink-migration-v2.md).
- [Relayer operations](docs/operators.md), [reporters and key custody](docs/reporter.md),
  [infrastructure](docs/hosting.md), [token metadata](docs/token-metadata.md).
- [Audit scope](docs/audit.md), [IBC verifier](docs/verifier.md),
  [header scaling](docs/header-scaling.md), [signature batches](docs/signature-batches.md).
