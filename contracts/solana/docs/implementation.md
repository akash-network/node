# AKT on Solana

Local Anchor workspace for AKT's issuance program. Genesis creates a legacy SPL mint with zero supply; every AKT on Solana is minted for a proven Akash deposit. The bridge prototype verifies transfer and governance-void proofs through the real upstream Tendermint client. Local integration tests exercise the complete proof-to-mint path with a generated IAVL checkpoint.

The node's `x/sink` module implements native Akash deposits, proof records, reporter confirmations and guarded refunds. The [complete local stack](local-stack.md) runs both chains, authenticates signed Akash headers on Solana, relays proofs and submits reporter transactions. Its live demo exercises a mint and an invalid-recipient void/refund. The shared relayer now verifies synthetic equal-power 100-validator commits within the runtime limits, resumes interrupted uploads and reclaims temporary rent. A continuous monitor reconciles escrow, observed mints, report lag and SPL burns. See [test commands](reproduce.md). Production operation remains. See [the bridge protocol](bridge-protocol.md), [the x/sink README](../../../x/sink/README.md), [reporter](reporter.md) and [settlement design](sink-reporting.md).

## Implemented launch contract

`initialize` takes no instruction arguments. The current program upgrade authority must sign, and its account pays for creation of the launch configuration and the mint. The instruction validates the deployed program's identity and its corresponding loader-owned ProgramData account before accepting that authority.

| Property                | Enforced behavior                                                                                              |
| ----------------------- | -------------------------------------------------------------------------------------------------------------- |
| Initial issuance        | None. Supply starts at zero; AKT uses six decimals.                                                            |
| Token program           | Legacy SPL Token only.                                                                                         |
| Mint authority          | PDA derived from `["mint_authority"]` under this program.                                                      |
| Freeze authority        | None at creation.                                                                                              |
| One-time initialization | Fixed configuration and mint PDAs; no close or reset instruction.                                              |
| Bridge                  | Starts disabled. The upgrade authority must pin a client and activate it. All issuance requires a valid proof. |

The configuration uses `["config"]` and the mint uses `["mint"]` as PDA seeds. The mint address is the token's identity; it differs from the program ID. Its address is deterministic for this deployment, but calling it canonical AKT requires Akash to designate and publish that mint. The generated local deployment address is only for development.

There is no separate administrator mint instruction. The **program upgrade authority can replace the code**, so production deployment still needs the intended multisig and time lock. Local tests use the development wallet as upgrade authority. This contract does not lock funds on Akash; each mint is backed by the deposit it proves.

The [audit package](audit.md) lists every instruction, account and invariant, the trust model and the accepted risks.

## Implemented bridge

- One-time client configuration binds the deployment and chain ID. Each proof checks account ownership, addresses, frozen state and latest-client expiry. The upgrade authority can re-pin another checked deployment at any time, for recovery or to move verification to other operators; see [re-pinning](bridge-protocol.md#re-pinning-the-light-client).
- Submitters upload proofs in chunks to owned buffers. Completed proofs can be submitted by anyone; only the buffer owner can reclaim its rent.
- Proven transfers mint to a derived ATA or permanently void an invalid recipient, including one whose existing ATA was reassigned to another owner. A proven Akash governance void uses the same terminal status account, preventing a later mint.
- Mint and void reject CPI and any earlier instruction other than compute budget, so their `TransferResolved` events stay visible to reporters.
- The upgrade authority controls pause, per-transfer limits, a fixed UTC daily mint cap and a vault allowlist. Cap failures leave transfers retryable.
- An optional pause guardian, named by the upgrade authority, can pause minting but never enable it.
- Every state change an operator monitors emits an event: `LaunchInitialized`, `TokenMetadataSet`, `BridgeConfigured`, `BridgeEnabledSet`, `MintPolicySet`, `PauseGuardianSet`, `GuardianPaused`, `LightClientRepinned` and `TransferResolved`.
- AKT error codes run from 7000, so they never collide with ICS07 errors from 6000 that a failed membership CPI returns.

There is no refund instruction on Solana. Reporters must observe the permanent outcome and follow the Akash refund rules.

## Toolchain

| Tool                        | Version                   |
| --------------------------- | ------------------------- |
| Rust                        | 1.89.0                    |
| Solana / Agave CLI          | 2.3.13                    |
| Anchor                      | 0.32.1                    |
| Node                        | 24.14.1                   |
| Go, for IAVL proof fixtures | 1.23+; tested with 1.26.2 |

The Solana versions match the upstream light-client repository at commit `dc78ade828b766b218fb6bcb5417fdaf8e553021`. Rust and Anchor versions are pinned in this workspace. Keep both `Cargo.lock` and `package-lock.json` in version control.

The Rust lockfile uses upstream-compatible transitive versions. Solana's on-chain compiler is bundled separately from host Rust; regenerating the lockfile from scratch can select dependencies that its compiler cannot build.

The pinned Anchor JavaScript SDK currently brings npm audit advisories through `toml`, `stream-json` and `uuid`. A compatible `npm audit fix` pass does not resolve them; the workspace retains the SDK version used by upstream.

Use [setup.md](setup.md) for portable tool installation and `npm run setup` for all builds. The network launcher and contract-test wrapper create their own ignored development wallets and preload fixed program IDs. Neither needs the original machine's wallet or deployment key.

## Run locally

The [four-validator testnetify profile](testnetify.md) exercises validator-set changes, quorum loss, pause/retry and an actual governance-void proposal:

```bash
export AKT_LOCAL_RUN=dev
npm run testnet:up
npm run testnet:demo
npm run testnet:scenarios
npm run testnet:verify
```

Keep `AKT_LOCAL_RUN` set for every command. Use `npm run testnet:down` to stop that profile. It uses Akash port 27657 and Solana port 18999. Run names share ports, so stop an existing profile before selecting another.

The isolated [100-validator header benchmark](header-scaling.md) runs with `npm run bench:headers` and `npm run bench:headers -- weighted`. It measures the pinned verifier's account, compute and cost limits on a separate temporary local cluster.

For a single-validator alternative after setup:

```bash
npm run local:up
npm run local:demo
npm run local:status
```

Use `npm run local:down` to stop its processes while preserving state. See [local stack setup and limits](local-stack.md) for build prerequisites and logs. The standalone validator tests below use RPC port 19899 and ports 19800 to 19850, not port 8899.

Build the pinned upstream programs once. This downloads their source under the ignored `.deps/` directory and checks the generated interfaces against `idls/`:

```bash
npm run setup:light-client
```

The integration suite builds and deploys AKT alongside those programs, then executes real transactions against a fresh local validator:

```bash
npm test
```

The test wrapper starts and stops its own validator and creates a fresh ignored wallet. It preloads all programs at fixed IDs, so no original deployment key is needed. It also builds and loads upstream's `test-cpi-proxy` from the pinned source to check that mint and void reject CPI, plus two more copies of the verifier at other addresses for re-pin tests. One of them is initialized frozen. It listens on RPC port 19899 and refuses to start if that port or the next is taken; set `AKT_TEST_RPC_PORT` to move it. The suite checks genesis, burns its local tokens, then exercises bridge issuance, altered proofs, replay, voids, limits, the pause guardian, expiry and re-pinning, and decodes every event. `tests/idl.test.ts` checks the generated IDL. It takes about a minute because the expiry test waits for the local client's trusting period to elapse. It finishes re-pinned to the replacement client, with the expired original deployment closed. The wrapper creates a new isolated ledger on the next run and preserves earlier evidence.

The suite requires a fresh ledger because initialization is deliberately irreversible within this program version. Use `npm test` for isolated checks and `local:up` or `testnet:up` for persistent paired chains. The [reproduction guide](reproduce.md) gives the complete sequence, including restart and fault checks.

## Development commands

| Command                             | Purpose                                                        |
| ----------------------------------- | -------------------------------------------------------------- |
| `npm run build`                     | Compile the program and generate its IDL and TypeScript types. |
| `npm run setup:light-client`        | Fetch and build the pinned verifier and access manager.        |
| `npm run build:header-encoder`      | Build the host Tendermint/Borsh header encoder.                |
| `npm run local:up`                  | Start both chains, the relayer and three local reporters.      |
| `npm run local:demo`                | Exercise live mint, confirmation, void, refund and accounting. |
| `npm run local:check-header`        | Reject an altered header, then accept its authentic version.   |
| `npm run local:status`              | Read process status, chain heights and sink totals.            |
| `npm run local:down`                | Stop the stack while preserving both ledgers.                  |
| `npm run test:unit`                 | Check the wire format, client, policy, ATA and cap rules.      |
| `npm test`                          | Build and test with a fresh wallet and isolated validator.     |
| `npm run test:sink`                 | Execute an Akash deposit and verify its real proof on Solana.  |
| `npm run test:reporter`             | Check observation, reporting, RPC and signer boundaries.       |
| `npm run reporter -- --config PATH` | Preview reporter actions against configured nodes.             |
| `npm run logs`                      | Stream program logs from the `localnet` validator.             |
| `npm run local:admin`               | Admin commands, signed locally or `--emit`ted for a multisig.  |
| `npm run localnet`                  | Start an empty validator with Metaplex for deploy rehearsals.  |
| `npm run rehearse:launch`           | Rehearse the full launch, multisig steps included, locally.    |
| `npm run typecheck`                 | Check test types after building.                               |
| `npm run format`                    | Format Rust, tests and workspace documentation.                |
| `npm run format:check`              | Check formatting.                                              |

Use `npm ci` to restore JavaScript dependencies from the lockfile.

## Files

- `programs/akt-solana/src/lib.rs`: documented entry points in lifecycle order.
- `programs/akt-solana/src/instructions/`: one module per instruction group: genesis, token metadata, bridge admin, pause guardian, proof buffers, mint and void.
- `programs/akt-solana/src/state.rs`, `events.rs`, `errors.rs`, `constants.rs`: accounts, events, errors, seeds and limits.
- `programs/akt-solana/src/light_client.rs`: pinned client checks and membership CPI.
- `programs/akt-solana/src/wire.rs`: shared x/sink encoding and proof keys, pinned by unit tests.
- `programs/akt-solana/ics07-tendermint-interface/`: `declare_program!` bindings for the verifier, kept in their own crate so the verifier's errors stay out of AKT's IDL.
- `tests/genesis.test.ts`, `tests/metadata.ts` and `tests/bridge.ts`: local validator integration tests.
- `tests/idl.test.ts`: checks that the IDL keeps the interface the test network runs and lists only AKT errors.
- `tests/sink-proof.ts` and `tests/reporter.ts`: real Akash app proof, tampering, replay and signed return-report tests; require this node repository and its Go/native dependencies.
- `reporter/`: finalized observer, settlement decisions, Akash client and local runner. See [reporter setup](reporter.md).
- `local/`: persistent paired chains, signed-header/proof relay and live scenarios.
- `tools/header-encoder/`: host Rust encoder using the pinned upstream Tendermint and Borsh formats.
- `tools/proof-fixtures/`: Go generator using IAVL and ICS23.
- `idls/`: generated, pinned upstream interfaces used by Anchor's `declare_program!`.
- `Anchor.toml`: program address, local wallet, toolchain and test command.
- `rust-toolchain.toml`: Rust toolchain pin.
- `target/idl/` and `target/types/`: generated client interface and types.

## Remaining launch work

- Production metadata publication, wallet display checks, canonical address publication and launch tooling. The admin-controlled Metaplex instruction and local tooling are implemented; [metadata requirements](token-metadata.md) are a mainnet launch gate.
- Review and audit the [verifier patches](verifier-patches.md), mint program ([audit package](audit.md)), native sink and reporter path. Resolve reachable upstream stack-size diagnostics and test real 100-validator transitions and sustained throughput.
- Operate independent reporter nodes with protected signers, durable queues, alert delivery and indexed settlement monitoring. Retain complete burn history.
- Choose the production upgrade version, OCL source and preexisting-balance policy. The local-only handler and genuine binary-swap upgrade rehearsal now pass; they do not authorize a mainnet upgrade.
- Define checkpoint and re-pin ceremonies, and rehearse production policy under multisig/time locks. Expiry stops minting and header updates until the upgrade authority re-pins a reviewed new client.

There is no mock proof verifier or trusted-relayer mint bypass. Upstream source reference:

- Repository: <https://github.com/cosmos/ibc-contracts>
- Commit: `dc78ade828b766b218fb6bcb5417fdaf8e553021`
- Programs: `ibc-solana/programs/ics07-tendermint` and `ibc-solana/programs/access-manager`

The standalone test suites initialize explicitly trusted checkpoints. The persistent local stack goes further: after its initial trusted checkpoint, it verifies real signed headers through upstream ICS07 before relaying state proofs. This works without ICS26 or the upstream proof API. A reviewed production checkpoint, real full-set transitions and production operations remain.
