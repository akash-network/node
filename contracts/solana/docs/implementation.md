# Code walkthrough

Paths are relative to `contracts/solana`. This workspace contains the AKT
Solana program, its relayer and reporters, and the build/deployment tools.
Akash's native `x/sink` module and the public `chain-sdk` types live outside it.

## Contents

- [Follow a transfer](#follow-a-transfer)
- [Contract files](#contract-files)
- [Light client](#light-client)
- [Services](#services)
- [Build, deploy, and test](#build-deploy-and-test)

## Follow a transfer

1. A wallet submits `MsgSink` to Akash. Native `x/sink` locks the AKT and stores
   a transfer identified by `owner/height/sequence`.
2. [`local/relay.ts`](../local/relay.ts) discovers the transfer through native
   queries. It obtains a storage proof and updates the Solana light client
   when a newer authenticated Akash root is needed.
3. The relayer uploads the proof through
   [`instructions/proof_buffer.rs`](../programs/akt/src/instructions/proof_buffer.rs).
   Uploads are sequential, bounded, and owned by their submitter.
4. [`mint_transfer.rs`](../programs/akt/src/instructions/mint_transfer.rs)
   checks the proof, recipient, and mint limits. It mints through the legacy
   SPL Token program, or records a permanent void for an invalid recipient.
5. [`reporter/solana.ts`](../reporter/solana.ts) reads the finalized outcome and
   transaction evidence. [`reporter/policy.ts`](../reporter/policy.ts) decides
   which native report or refund action is eligible.
6. Reporters submit signed transactions to Akash. Native `x/sink` handles
   quorum, challenge timing, settlement, and refunds.

A governance void follows the same proof-upload path.
[`void_transfer.rs`](../programs/akt/src/instructions/void_transfer.rs)
proves the native settlement and its permanent void authorization. Mint and void
create the same status account. Once either succeeds, the other cannot execute.

## Contract files

The Rust program is in [`programs/akt/src`](../programs/akt/src).

| File                                                                               | What it does                                                                                |
| ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| [`lib.rs`](../programs/akt/src/lib.rs)                                             | Public Anchor instructions.                                                                 |
| [`state.rs`](../programs/akt/src/state.rs)                                         | Bridge configuration, mint policy, proof buffers, and permanent outcomes.                   |
| [`wire.rs`](../programs/akt/src/wire.rs)                                           | Native IDs, storage keys, record encoding, and hashed owner PDA seeds.                      |
| [`light_client.rs`](../programs/akt/src/light_client.rs)                           | Authenticate the pinned client and consensus accounts, check time, and verify ICS23 proofs. |
| [`authorization.rs`](../programs/akt/src/authorization.rs)                         | Read the permanent governance authorization from a proven settlement.                       |
| [`instructions/resolution.rs`](../programs/akt/src/instructions/resolution.rs)     | Store the terminal outcome, emit its event, and enforce direct invocation.                  |
| [`instructions/genesis.rs`](../programs/akt/src/instructions/genesis.rs)           | Create the mint once, with zero supply and bridge issuance disabled.                        |
| [`instructions/bridge_admin.rs`](../programs/akt/src/instructions/bridge_admin.rs) | Configure the client, pause issuance, set limits, and re-pin a client.                      |

Other files in `instructions/` manage the guardian, metadata, and upgrade-authority
checks. [`constants.rs`](../programs/akt/src/constants.rs),
[`errors.rs`](../programs/akt/src/errors.rs), and
[`events.rs`](../programs/akt/src/events.rs) define their shared values.

## Light client

The ICS07 Tendermint verifier and access manager come from the full
`akash-network/ibc-contracts` commit pinned in
[`programs/akt/Cargo.toml`](../programs/akt/Cargo.toml).
[`scripts/setup-light-client.sh`](../scripts/setup-light-client.sh) builds them
and checks their generated IDLs against [`idls/`](../idls/README.md).

ICS07 verifies signed Akash headers and stores consensus roots. The access
manager controls who can submit header updates. AKT reads these authenticated
accounts and verifies membership locally through the pinned membership library.
It does not call ICS07's membership instruction by CPI.

[`tools/header-encoder`](../tools/header-encoder) encodes headers using upstream
Borsh types. [`local/header-upload.ts`](../local/header-upload.ts) uploads them
and submits verification transactions. [Verifier details](verifier.md) cover
header updates, freezing, pruning, and relayer permissions.

## Services

| File                                                                                        | What it does                                                          |
| ------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| [`reporter/akash.ts`](../reporter/akash.ts)                                                 | Read native queries at a fixed committed height.                      |
| [`local/open-transfers.ts`](../local/open-transfers.ts)                                     | Discover transfers with the server's pagination tokens.               |
| [`local/proof-job.ts`](../local/proof-job.ts) and [`local/durable.ts`](../local/durable.ts) | Persist proof jobs and signed transaction recovery state.             |
| [`shared/id.ts`](../shared/id.ts) and [`shared/proof.ts`](../shared/proof.ts)               | Share identity, proof, and Solana account derivation between clients. |
| [`reporter/run.ts`](../reporter/run.ts)                                                     | Start the reporter in preview or signing mode.                        |
| [`local/bridge-admin.ts`](../local/bridge-admin.ts)                                         | Inspect state and prepare or execute authorized administration.       |
| [`local/stack.ts`](../local/stack.ts)                                                       | Start development chains and services.                                |

The production relayer and admin CLI currently live under `local/`.
[`deploy/service.sh`](../deploy/service.sh) runs production services with explicit
configuration. The testnet launcher is a separate development tool.

## Build, deploy, and test

- [`scripts/build-programs.sh`](../scripts/build-programs.sh) builds AKT and its
  IDL. [`tools/verifiable-build`](../tools/verifiable-build) builds committed
  sources in the pinned Linux image.
- [`deploy/programs.ts`](../deploy/programs.ts) checks build artifacts, cluster
  identity, deployed bytes, and authorities. The Solana CLI performs deployment.
- [`tests/bridge.ts`](../tests/bridge.ts) exercises the program on a validator.
  [`tests/sink-proof.ts`](../tests/sink-proof.ts) uses a real native deposit proof.
- [`programs/akt/tests/sbf`](../programs/akt/tests/sbf) runs the ELF
  in an SBF VM. [`tools/proof-fixtures`](../tools/proof-fixtures) generates proof
  fixtures through native libraries.

Follow the [README](../README.md) to build or start Solana,
[program deployment](deploy-program.md) for the deployment sequence, and
[tests](reproduce.md) to run checks. [Audit reference](audit.md) lists the trust
assumptions, invariants, and remaining limitations.
