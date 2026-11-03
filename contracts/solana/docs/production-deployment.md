# Production deployment

This is the launch and operating runbook for Akash Sink on `akashnet-2` and
Solana mainnet-beta. It is a deployment process, not a record of a mainnet launch.
Use reviewed schema-4 node, SDK, Solana and service artifacts together. Read
[canonical launch and recovery](sink-migration-v2.md) before preparing a new
program/mint namespace; never reuse one against a reset or forked Akash chain.

## Deployment workflow

1. Agree the authority holders, checkpoint procedure, network parameters,
   upgrade plan and activation criteria below.
2. Produce the launch commit, reproducible program artifacts and reviewed
   manifest. Run offline checks, cluster preflight and the
   [program deployment ceremony](deploy-program.md).
3. Build and install the immutable [service release](deploy-services.md).
   Keep reporters in preview until governance authorizes them.
4. Complete the Akash upgrade, checkpoint, bridge configuration and activation
   sequence below. Record signatures and finalized verification after each step.
5. Retain the artifacts, manifest, service configuration and acceptance evidence
   for recovery and future upgrades.

[Hosting](hosting.md) specifies infrastructure and custody boundaries.
[Development setup](setup.md), [local stack](local-stack.md) and
[testnet scenarios](testnetify.md) are separate rehearsal workflows. Their
keys, timing overrides, historical results and amount limits are not production
configuration. Recheck chain parameters and runtime versions before each launch.

## Operator neutrality

No part of the stack is tied to whoever operates it. Governance can replace each
operator role without code changes:

- **Light client.** The bridge account pins the light-client deployment.
  `repin_light_client` moves it to any compatible deployment at any time, under
  the upgrade authority. Relayer, reporters, monitor, portal and
  admin CLI read the pin from the bridge account every pass and alert when it
  changes.
- **Relayers.** Minting and voiding are permissionless. Header updates need
  `RELAYER_ROLE` or open relaying: granting the role to the light-client
  program's own ID lets any signer update and submit misbehaviour, while
  pruning stays with members. The relayer runs from a config file
  with its own nodes and payer ([operators](operators.md)).
- **Reporters.** Governance sets the reporter set and threshold. Each reporter
  runs from its own config, nodes and key custody, including an external
  KMS/HSM signer ([reporter](reporter.md)).
- **Pauses.** The Solana guardian and the x/sink `pause_guardian` are set by
  their governance authorities and can only turn things off.

## Build ownership and release inputs

| Artifact                    | Existing build path                               | Release input                                          |
| --------------------------- | ------------------------------------------------- | ------------------------------------------------------ |
| Akash node                  | Root `make bins` and existing GoReleaser workflow | Committed Go pins, `GOWORK=off`, no `sinkdev`          |
| AKT program                 | Anchor and Cargo in this workspace                | Reviewed node commit and Cargo lockfile                |
| IBC verifier/access manager | Anchor in `akash-network/ibc-contracts`           | Full commit pinned by both Cargo manifests             |
| Header encoder              | `cargo build --locked --release`                  | Same IBC commit and its own committed lockfile         |
| Services                    | npm lockfile and service archive builder          | Same node commit, reviewed IDLs and locked SDK archive |

The program build wrapper fixes the toolchain and paths for both Anchor
workspaces and collects the artifacts. It does not replace Anchor's compiler
or Solana's deployment CLI. Public source verification must independently
reproduce the deployed bytes; see [verifier build details](verifier.md#verifiable-builds).

The SDK archive is an explicit temporary packaging dependency until the matching
TypeScript SDK is published. Never rebuild it silently during installation.
Likewise, a local Go workspace is a development choice, not evidence that the
committed node dependencies are ready for release.

`npm run test:build` checks that setup delegates to the node Makefile, fixture
tests ignore stale modfiles, and IBC checkouts preserve local changes and reject
mismatched pins. These checks run in the existing node CI workflow. They use
local fixtures; real locked builds and the launch rehearsal are separate gates.

## Launch acceptance gates

Keep deposits and minting disabled until these gates have recorded owners and
evidence for the final release:

- The normal node release build and native Sink tests pass with committed Go
  dependencies and `GOWORK=off`. The locked service SDK agrees with the deployed
  node's query responses; run the native proof and reporter integration tests
  with that exact pair.
- External review of the AKT program, pinned IBC verifier, x/sink and services;
  independent program builds with matching hashes and IDLs.
- Fresh production program IDs, proven control of the multisig vault, reviewed
  time lock and members, and successful finalized authority handover for all
  three programs. No hot service key holds upgrade authority.
- The final Akash upgrade name/height and a rehearsal against populated state,
  including a preexisting balance at the sink address. Verify the zero-reserve
  installation, escrow invariant, native epochs and restart behavior.
- Independent checkpoint agreement and client initialization, then continuous
  relaying before the trusting period expires. Rehearse expiry/freeze recovery
  through the real multisig and time lock.
- Independent reporter operators, funded accounts, tested external signers,
  preview observations and archive retention covering the challenge period.
- Production monitoring/reconciliation, proven alert delivery, on-call guardian
  access, backups and a service restart/rollback exercise.
- Production portal and public versioned token metadata. `local/team.ts` is a
  testnet adapter with a faucet; it is not the production API.
- Canary mint, void/refund after the production challenge period, cap boundaries
  and supply reconciliation using the exact deployed artifacts.

The manifest checks artifact identity and cluster identity, not these operational
or protocol gates. Rehearsal evidence must name the source and artifact hashes
it covers. Earlier scalar-ID testnet results do not accept schema 4.

Remaining protocol considerations belong in the launch review: reporter slot
plausibility is measured at observation time; permissionless proof submission
must be reconciled against Akash refund/dispute state; large validator sets need
the supported verifying signature subset. See [audit](audit.md),
[reporters](reporter.md) and [header scaling](header-scaling.md).

## Authorities and keys

Whoever holds any of the three upgrade authorities can mint without limit, by
replacing the AKT program or the verifier it trusts. Treat all three as one
security tier.

| Power                                      | Controls                                                                                                                                                  | Production holder                                                                            |
| ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| AKT upgrade authority                      | Program code; `initialize`, `configure_bridge`, `set_bridge_enabled`, `set_mint_policy`, `set_token_metadata`, `set_pause_guardian`, `repin_light_client` | Multisig with a time lock; members use hardware wallets                                      |
| Pause guardian                             | `guardian_pause` only; it can never enable minting                                                                                                        | A key held by on-call operators, outside the time lock                                       |
| ICS07 upgrade authority                    | The verifier code the mint trusts; client `initialize`, which sets the checkpoint                                                                         | The same multisig and time lock                                                              |
| Access-manager upgrade authority and admin | Grants and revokes `RELAYER_ROLE`, opens or closes relaying; ICS07 access-manager transfer                                                                | The same multisig, listed as the only CPI caller. This access manager must govern only ICS07 |
| `RELAYER_ROLE`                             | Header updates, misbehaviour submission and pruning; it cannot forge headers. Open relaying admits any signer except for pruning                          | One hot key per relayer; also granted to misbehaviour watchers                               |
| x/sink pause guardian                      | `MsgGuardianPause`: turns deposits and/or refunds off; only governance turns them on                                                                      | An on-call key, multisig or group policy set by governance                                   |
| Relayer fee payer                          | Proof buffers, status and ATA rent, header updates                                                                                                        | Hot key holding a few days of spend, with a top-up alert                                     |
| Reporters                                  | `ReportOutcome`, `VetoVoid` and `FinalizeRefund` on Akash                                                                                                 | One per independent operator, in KMS/HSM, each with its own fees                             |
| Akash governance                           | x/sink parameters, `RequestVoid`, `RestartTransfer`, `ConfirmMint`, `ConfirmVoid`                                                                         | `akashnet-2` governance                                                                      |
| Metadata update authority                  | Mint-authority PDA; changes only through `set_token_metadata`                                                                                             | Unchanged                                                                                    |

Keep the upgrade authorities. `repin_light_client` needs the AKT authority, and
a verifier fix needs the ICS07 authority. Making a program immutable removes
those recovery paths.

## Parameters

### Light client

| ICS07 `initialize` field | Testnet                          | Production                                                                                         |
| ------------------------ | -------------------------------- | -------------------------------------------------------------------------------------------------- |
| `chainId`                | `akash-bridge-testnetify-team-1` | `akashnet-2`                                                                                       |
| Trust level              | 1/3                              | 1/3                                                                                                |
| `trustingPeriod`         | 86,400 s                         | At most 1,209,600 s (two thirds of unbonding). This is also the longest outage before recovery     |
| `unbondingPeriod`        | 1,814,400 s                      | 1,814,400 s, mainnet `unbonding_time`                                                              |
| `maxClockDrift`          | 10 s                             | Measure first. Solana's Clock sysvar read about 1 s behind wall time on 2026-10-18 (three samples) |
| Checkpoint               | Latest local header              | The [checkpoint ceremony](#3-checkpoint-and-bridge-configuration)                                  |

An idle relayer refreshes the client after `AKT_RELAY_REFRESH_FRACTION` of the
trusting period, one fifth by default. Each update needs more than a third of
the trusted validator power to have signed the new header, so lower the
fraction if the validator set changes quickly.

### Solana mint policy

- `per_transfer_max` must not exceed `daily_mint_cap`. The launch policy is
  1,000,000 AKT per transfer and 10,000,000 AKT per UTC day; the tooling and
  the launch rehearsal use these values.
- Leave `vaults` empty unless a vault has been reviewed. It must match the x/sink
  `allowed_destination_vaults`.

### x/sink

Amount parameters use `uakt` Coins. Zero disables the account epoch cap. The minimum may be zero while deposits are disabled; the other limits remain positive. Account and aggregate caps may be below the transfer maximum. Choose launch values through governance; local test values are not recommendations.

| Parameter                                                       | Meaning                                                                                                                                     |
| --------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled`, `refunds_enabled`                                    | Independent switches; keep deposits off until validation completes                                                                          |
| `min_amount_per_transfer`, `max_amount_per_transfer`            | Positive transfer bounds; align maximum with Solana's independent per-transfer cap                                                          |
| `max_epoch_amount_per_account`, `max_transfer_amount_per_epoch` | Account and aggregate sending caps; zero disables only the account cap, and usage remains tracked                                           |
| `max_epoch_refund`                                              | Independent positive refund cap within the native quota epoch                                                                               |
| `reporters`, `reporters_threshold`                              | Members and decimal ratio; ceil(ratio × members) must exceed two thirds of the members. State supplies the effective current ReporterPolicy |
| `reporter_challenge_period`                                     | Protobuf duration with the production build's required minimum                                                                              |
| `pause_guardian`                                                | Governance-selected account that may only disable switches                                                                                  |

Policy rotation advances its version. Terminal evidence and governance refund authorization remain historical; unpaid quorum refunds require current-policy authorization. Lowered caps do not reset consumption.

Sink reads the existing `x/epochs` record named `day`. Configure its duration and start in epochs genesis, and preserve that record on recovery. A started epoch advances in the first block strictly after its end time. After downtime it catches up one epoch per block, so successive recovery blocks can each open a new allowance. Do not calculate Sink usage from UTC dates or replace the native record on a populated chain.

The reporter challenge period remains an elapsed duration from authorization. An epoch boundary does not shorten it, and changing the parameter does not change an existing deadline.

Change caps in this order:

- **Raising:** Solana first, then Akash.
- **Lowering:** Akash first. Wait for pending transfers above the new limit to
  settle, then lower Solana. Lowering the Solana per-transfer maximum first
  stalls those transfers.

## Infrastructure

- Run the relayer, each reporter, monitoring and the portal on separate hosts.
  Keep no keys on the portal host.
- Each reporter runs its own Akash node and its own Solana RPC node. Finalized
  commitment does not authenticate a dishonest node, so reporters must not share
  one.
- Solana transaction history must cover at least the challenge period plus
  reporting lag. Pruned history broke health checks for old testnet transfers.
  Keep an archival source for older evidence.
- Alert on disk use above 70% on every node. Never copy chain data into artifact
  or evidence directories on the same disk.
- Run NTP on every host.

## Launch sequence

### 0. Readiness

- Close the remaining blockers. Complete audits of the AKT program, the ICS07
  [IBC verifier](verifier.md), x/sink, the reporter and the relayer.
- Rehearse the whole sequence on a fresh testnet with production settings: the
  multisig and time lock, real trusting and challenge periods, separate reporter
  hosts, and the production upgrade handler run against a dusted sink address.
  Drill every [runbook](#incident-runbooks). `deploy/rehearse-launch.sh`
  rehearses the Solana steps locally, with a keypair standing in for the vault.
- Prepare the [metadata gate](token-metadata.md#mainnet-launch-gate) items.

### 1. Solana deployment

Minting stays off throughout. [Deploying the programs](deploy-program.md) has
the commands and the check after each step; `deploy/rehearse-launch.sh` runs
them on a local validator.

1. Generate keypairs for the three programs and make a launch commit that
   changes only their IDs. Publish the verifiable build hashes
   ([steps 1 to 3](deploy-program.md#1-program-keypairs)).
2. Deploy all three programs with a one-time ceremony key as upgrade authority
   ([step 4](deploy-program.md#4-deploy)).
3. With the ceremony key, in one transaction, initialize the access manager,
   list the multisig program as an allowed CPI caller, grant `ADMIN_ROLE` to
   the vault and renounce the ceremony key's own. The access manager's
   `initialize` rejects CPI, and it rejects admin calls by CPI from any
   unlisted program, so the multisig could do neither. Then run AKT
   `initialize`. Verify six decimals, no freeze authority, the mint-authority
   PDA and a supply of zero
   ([step 5](deploy-program.md#5-access-manager-and-genesis)).
4. Transfer all three upgrade authorities to the multisig. Verify each
   program's bytes and authority, then retire the ceremony key
   ([step 6](deploy-program.md#6-upgrade-authorities-to-the-multisig)).
5. Through the multisig, set token metadata and the pause guardian. Every
   multisig step uses unsigned instructions from the admin CLI's `--emit`,
   which each reviewer decodes ([step 7](deploy-program.md#7-multisig-transactions)).

### 2. Akash upgrade

1. Submit the software-upgrade proposal. Confirm the current governance voting period and upgrade timing. State the
   preexisting-balance disposition.
2. The handler adds the store with a zero balance and leaves deposits disabled
   with no reporters.
3. Verify the totals, parameters and bank invariant from several nodes.

### 3. Checkpoint and bridge configuration

1. Pick a post-upgrade `akashnet-2` height. At least three people fetch its
   header, commit and validator set from at least three independent nodes. They
   compare the block hash, app hash and validator hash, and record the result
   (`bridge-admin checkpoint`, [step 8](deploy-program.md#8-checkpoint-and-bridge)).
2. Through the multisig, in one transaction, run ICS07 `initialize` with that
   checkpoint and the production client parameters, then
   `configure_bridge("akashnet-2", policy)`. `configure_bridge` names the
   client's consensus state at its latest height, so a header update between
   proposal and execution would fail it; the shared transaction pins that
   height. It also fails on an expired client, so the time lock must be much
   shorter than the trusting period. Minting stays off.
3. Start the relayer as soon as it executes, because the trusting period runs
   from the checkpoint's timestamp. Grant relayer roles, or open relaying,
   through the multisig.

### 4. Reporters

1. Reporter operators run against their production nodes in preview mode,
   without `--broadcast`.
2. Submit a governance proposal for the reporters, threshold, challenge period,
   caps, refund cap, vault allowlist and minimum transfer. Leave `enabled`
   false.
3. Run with monitoring for several days, until every alert stays clear.

### 5. Activation

1. Through the multisig, run `set_bridge_enabled(true)`. Nothing mints until
   Akash accepts deposits.
2. Pass a governance proposal setting `enabled` to true with low caps.
3. Make team canary transfers: a normal transfer, an invalid recipient (void,
   then refund after the challenge period) and one at the per-transfer cap. Check
   supply after each.
4. Publish the canonical mint on official Akash channels, then raise caps in
   steps.

## Monitoring

Route every alert to a person on call. Logs alone do not count.

Read program events by invocation frame, not with Anchor's `EventParser`. The
multisig executes admin actions by CPI, so their events appear inside the
multisig's frame, where `EventParser` ignores them. The reporters' parser
(`programEvents` in `reporter/solana.ts`) attributes each event to the program
that emitted it; `bridge-admin events --signature SIG` uses it to list the AKT
events of an executed multisig transaction.

| Signal                                      | Alert                                                                                                                                                                                                                                               |
| ------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Client age, now minus latest consensus time | Warn at 25% of the trusting period, page at 50%                                                                                                                                                                                                     |
| Client frozen                               | Page                                                                                                                                                                                                                                                |
| Pending transfer age                        | Page well past the normal 2 to 3 minutes                                                                                                                                                                                                            |
| Solana status present, Akash still pending  | Page and investigate missing reporter evidence                                                                                                                                                                                                      |
| Void in its challenge window                | Review every one until the reporters have a track record                                                                                                                                                                                            |
| Supply                                      | Page on any mismatch in `escrow behind mints = Solana supply + burns` or the x/sink bank invariant                                                                                                                                                  |
| Authority and configuration changes         | Any `ProgramData` change for the three programs; AKT `BridgeConfigured`, `BridgeEnabledSet`, `MintPolicySet` or `TokenMetadataSet`; access-manager `RoleGrantedEvent`, `RoleRevokedEvent` or `WhitelistedProgramsUpdatedEvent`; any x/sink proposal |
| Guardian and recovery actions               | AKT `PauseGuardianSet`, `GuardianPaused` or `LightClientRepinned`; `akash.sink.v1.EventGuardianPaused`; open or close relaying, a role change for the light client's own ID                                                                         |
| Relayer alerts                              | `relay-unbacked-settlement`, `relay-discovery-failed`, `relay-budget-exhausted`, `header-unsupported`, `light-client-changed`, repeated `relay-retry`, `consensus-prune-failed`                                                                     |
| Fee balances                                | Relayer payer and each reporter account below a few days of spend                                                                                                                                                                                   |
| Reporter liveness                           | Each reporter's last vote age, `audit-unavailable`, any conflict hold, and each reporter's rate of conflicting votes and vetoes                                                                                                                     |
| Cap usage                                   | Solana daily mint and Akash native-epoch deposit/refund usage above 80%                                                                                                                                                                             |
| Infrastructure                              | Disk above 70%, stalled node height, oldest Solana history within the challenge period of now                                                                                                                                                       |

## Incident runbooks

| Incident                           | First action                                                                                                                                           | Follow-up                                                                                                                                                                                                                |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Suspected bad mint or verifier bug | Solana guardian pauses minting; x/sink guardian pauses deposits                                                                                        | Reconcile supply and prepare a fix upgrade or re-pin                                                                                                                                                                     |
| Suspicious void quorum             | Any honest reporter vetoes. If evidence cannot be checked, the x/sink guardian pauses refunds                                                          | Governance uses `MsgConfirmMint` or `MsgConfirmVoid` with current `expected_round` and checked evidence                                                                                                                  |
| Client close to expiry             | Restart the relayer, switch RPC and check the payer balance                                                                                            | Find why updates stopped                                                                                                                                                                                                 |
| Client expired or frozen           | Mints and voids stop while Akash still accepts deposits, so disable deposits                                                                           | Re-pin a new client after a new checkpoint ceremony ([commands](deploy-program.md#later-changes))                                                                                                                        |
| Relayer key leaked                 | Multisig revokes its `RELAYER_ROLE` and grants a new key                                                                                               | Loss is limited to the payer balance; it cannot forge headers                                                                                                                                                            |
| Reporter key leaked                | Governance replaces the reporter. The policy version advances; unpaid quorum refunds need new authorization while governance authorization is retained | Other reporters watch for false votes                                                                                                                                                                                    |
| One reporter holds many transfers  | The x/sink guardian pauses deposits, so no new transfers are held                                                                                      | One proposal: `MsgUpdateParams` removing the reporter, then `MsgRestartTransfer` with `expected_round` for each reviewed unpaid transfer from `akash query sink disputes`                                                |
| A mint quorum contradicts Solana   | An honest reporter's voided vote holds the Minted transfer                                                                                             | One proposal: `MsgRequestVoid` and `MsgConfirmVoid` with `expected_round` and finalized evidence. The transfer returns to RefundPending with a new challenge window, then refunds; replace the reporters that voted mint |
| Multisig member key leaked         | Rotate the member                                                                                                                                      | Review pending multisig transactions                                                                                                                                                                                     |
