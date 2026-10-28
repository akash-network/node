# Production deployment

This guide covers what changes between a test network and an Akash mainnet
launch. The testnet deliberately runs on one host with one key.
Those shortcuts are listed here so nobody copies them. Nothing in this guide has
been executed on mainnet. Local and testnet values are not launch
recommendations. The 2026-10-18 and 2026-10-19 fixes are implemented and tested
locally and on a test network; they still need the external audit.

Akash mainnet values were read from `api.akashnet.net` on 2026-10-18. Recheck
them before launch.

## Testnet shortcuts

| Area                   | Test network                                                                                         | Production                                                                                       |
| ---------------------- | ---------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Solana                 | `solana-test-validator` on one host, 20M-shred retention                                             | mainnet-beta; the relayer uses two RPC sources; each reporter runs its own node                  |
| Akash                  | Four testnetify validators on one host, `akash-bridge-testnetify-team-1`, x/sink in genesis          | `akashnet-2`, x/sink added by a governance software upgrade                                      |
| Program IDs            | AKT `FfFAP…`; ICS07 `HqPc…` and access manager `4fMih…` use upstream's published localnet keypairs   | Fresh keypairs for all three programs, generated during the [deploy ceremony](deploy-program.md) |
| Solana authorities     | One wallet holds every upgrade authority, bridge admin, access-manager admin, relayer role and payer | Separate holders; see [authorities and keys](#authorities-and-keys)                              |
| Reporters              | Four processes, threshold 3, one host, one Solana node, keys in one directory                        | Independent operators with their own nodes and KMS/HSM keys                                      |
| Client trusting period | 24 hours                                                                                             | Up to 14 days                                                                                    |
| Challenge period       | 20 seconds                                                                                           | At least 72 hours                                                                                |
| Amount caps            | uint64 maximum on both chains                                                                        | 1,000,000 AKT per transfer, 10,000,000 AKT minted and 1,000,000 AKT refunded per day             |
| Relayers               | One config-driven `local/relay.ts` on the host; open relaying on                                     | Several independent relayers; open relaying or granted roles, a governance choice                |
| Monitoring             | `local/supervise.ts` restarts processes and logs disk, chain, client and payer alerts to the journal | The [alerts](#monitoring) below, routed to a person                                              |
| Metadata URI           | Private portal                                                                                       | Public, versioned HTTPS; see the [metadata gate](token-metadata.md#mainnet-launch-gate)          |

These audit findings come only from the shortcuts. Following this guide avoids
them without code changes:

- One hot key holds every Solana power, and all reporter keys share a directory
  with the portal user.
- Rebuilt testnets reuse the chain ID and validator keys, so record IDs repeat
  while the mint-once key is only `(id, chain_id)`. `akashnet-2` is never reset.
  Never point an existing Solana deployment at a reset or forked chain.
- The test host's `ExecStartPre` ran as root with bridge-owned directories first in
  `PATH`. It now runs with a clean environment and absolute paths. Production
  units must not run root steps that resolve binaries from service-writable
  paths.
- The 2026-10-18 outage: a full disk halted Akash for 29 hours and expired the
  24-hour client.

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

## Launch blockers

Each item says what the fixes changed and what remains.

### Settlement safety

1. **CPI-invoked resolutions.** Fixed. `mint_transfer` and `void_transfer`
   reject CPI, and only ComputeBudget instructions may precede them, so earlier
   instructions cannot truncate the event. Reporters attribute events by
   invocation frame and accept log truncation only after a completed top-level
   AKT frame.
2. **Fabricated future slots blocked honest vetoes.** Fixed. Reporters ignore
   slots beyond their fresh finalized slot plus `maxSlotLead`, alert on them and
   look up reported signatures directly. Remaining: plausibility
   uses the check time, not the report's block time, so a fabricated slot just
   inside the lead delays a veto by up to about 4 minutes.
3. **No minimum deposit.** Fixed: x/sink `min_transfer` and a per-transfer lamport budget in the relayer. Set the minimum well above the relayer's cost of about
   0.0035 SOL per mint.
4. **Evidence search window.** Fixed. Reporters read the block at the status
   account's `resolved_at_slot` instead of paging signatures. Their
   Solana node must serve finalized blocks at resolution slots.
5. **Reassigned recipient ATA.** Fixed. The transfer voids as
   `InvalidRecipient`, and the relayer checks this before paying.
6. **Mints over Akash refunds.** The honest relayer skips records that Akash
   shows as voided or refunded without a Solana status, rechecks Akash just
   before paying, and alerts. Remaining:
   `mint_transfer` is permissionless, so anyone else can still submit that
   proof. The reconciler flags the state.

### Chain upgrade

7. **Sink as a receiving module account.** Fixed. The sink is a blocked bank
   address, so it cannot be a withdraw address.
8. **Dust at the sink address halted the upgrade.** Fixed in the handler. It
   adopts an existing account at the address, keeping its account number, and
   moves the balance to the community pool, so sink starts
   with a zero balance; there is no launch reserve to fund. The old-to-new
   binary rehearsal passes with a dusted address. Remaining: the production
   upgrade name and height.

### Operations

9. **Development relayer.** Fixed:
   - It pages x/sink `OpenTransfers`, backed by an on-chain index, and falls
     back to a cursor scan on older nodes.
   - Failed transfers back off, pause and cap limits wait without paying, and a
     per-transfer lamport budget stops runaway spend.
   - Header updates happen only when a proof needs one or a fifth of the
     trusting period has passed. Commits above 100
     signatures are trimmed to a verifying quorum.
   - A patched verifier instruction closes expired consensus states and returns
     their rent.

   Remaining: production hosting with alert delivery.

10. **Emergency stop.** Fixed. Solana has a pause-only guardian and
    x/sink has a governance-set `pause_guardian` that can only turn deposits
    and refunds off. Remaining: choose
    who holds each.
11. **Client recovery.** Fixed. `repin_light_client` works at any time under
    the upgrade authority, and every tool follows the pin. The admin CLI fetches and compares checkpoints and emits the
    client and re-pin instructions for the multisig
    ([deploying](deploy-program.md#later-changes)). Remaining: the checkpoint
    ceremony and a rehearsal under the real multisig and time lock.
12. **Reproducible builds.** Fixed. A pinned linux/amd64 container builds all
    three programs at a fixed path with identical hashes from any checkout
    (`17238fe`, `npm run build:verifiable`), and `akash version` reports the
    commit. Remaining: publish the hashes at launch, built
    for the fresh production program keypairs
    ([launch commit](deploy-program.md#2-launch-commit)).
13. **Portal amount parsing.** Fixed. The amount field uses the
    browser locale's decimal mark only, so decimal-comma keypads work.

Also fixed: unsigned uint64 decoding for the sink SDK package (chain-sdk
`f85b5fff`), exact CLI amounts and `ValidateBasic`,
reconciler voids without a Solana status and paging beyond 99 transfers, a monotonic verifier latest height, the refund-driver
docs, a per-account daily amount cap and
reporters paging open transfers with a rolling audit of settled ones.

Still open at lower priority:

- Reporter slot plausibility uses check time (blocker 2).
- Full commits above about 100 signatures exceed compute; the relayer trims to
  a quorum, which fits up to at least 150 validators. Recheck before Akash's
  active set grows past 100.

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
| Akash governance                           | x/sink parameters, `RequestVoid`, `ResolveTransfer`                                                                                                       | `akashnet-2` governance                                                                      |
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
  `vault_allowlist`.

### x/sink

| Parameter                                          | Testnet    | Production                                                                                                                                                                                          |
| -------------------------------------------------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled`                                          | true       | false until activation                                                                                                                                                                              |
| `max_transfer`                                     | uint64 max | 1,000,000 AKT, at most the Solana `per_transfer_max`                                                                                                                                                |
| `solana_daily_mint_cap`                            | uint64 max | 10,000,000 AKT, equal to the Solana `daily_mint_cap`. Akash cannot read Solana, so keep them equal by procedure                                                                                     |
| `max_daily_per_account`, `max_pending_per_account` | defaults   | Bound per-account spam. Both count transfers, not amounts                                                                                                                                           |
| `vault_allowlist`                                  | empty      | The Solana `vaults` list                                                                                                                                                                            |
| `reporters`, `reporter_threshold`                  | 4, 3       | At least 7 independent operators with threshold 5. The threshold must exceed two thirds; the limit is 32                                                                                            |
| `challenge_period_seconds`                         | 20         | At least 259,200 (72 hours), which release builds enforce. Governance is the fallback when reporters cannot check evidence, so it must have time to act. The testnet's 20 s needs a `sinkdev` build |
| `daily_refund_cap`                                 | uint64 max | 1,000,000 AKT, at least `max_transfer`, and never lowered below an open transfer, as x/sink requires                                                                                                |
| `refunds_enabled`                                  | true       | true                                                                                                                                                                                                |
| `min_transfer`                                     | 1 AKT      | Well above the relayer's per-transfer cost; see blocker 3                                                                                                                                           |
| `max_daily_amount_per_account`                     | 0 (none)   | Bound one depositor's share of the daily cap; at least `min_transfer`                                                                                                                               |
| `pause_guardian`                                   | test key   | The on-call holder chosen by governance                                                                                                                                                             |

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
- Alert on disk use above 70% on every node. Never copy chain data into release
  or evidence directories on the same disk.
- Run NTP on every host.

## Launch sequence

### 0. Readiness

- Close the remaining blockers. Complete audits of the AKT program, the ICS07
  [patches](verifier-patches.md), x/sink, the reporter and the relayer.
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

1. Submit the software-upgrade proposal. Voting takes 7 days. State the
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
| Solana status present, Akash still pending  | Page. Reporters missed the evidence (blocker 1)                                                                                                                                                                                                     |
| Void in its challenge window                | Review every one until the reporters have a track record                                                                                                                                                                                            |
| Supply                                      | Page on any mismatch in `escrow behind mints = Solana supply + burns` or the x/sink bank invariant                                                                                                                                                  |
| Authority and configuration changes         | Any `ProgramData` change for the three programs; AKT `BridgeConfigured`, `BridgeEnabledSet`, `MintPolicySet` or `TokenMetadataSet`; access-manager `RoleGrantedEvent`, `RoleRevokedEvent` or `WhitelistedProgramsUpdatedEvent`; any x/sink proposal |
| Guardian and recovery actions               | AKT `PauseGuardianSet`, `GuardianPaused` or `LightClientRepinned`; `sink_guardian_pause`; open or close relaying, a role change for the light client's own ID                                                                                       |
| Relayer alerts                              | `relay-unbacked-settlement`, `relay-budget-exhausted`, `header-unsupported`, `light-client-changed`, repeated `relay-retry`, `consensus-prune-failed`                                                                                               |
| Fee balances                                | Relayer payer and each reporter account below a few days of spend                                                                                                                                                                                   |
| Reporter liveness                           | Each reporter's last vote age, any conflict hold, and each reporter's rate of conflicting votes and vetoes, so a reporter that holds many transfers is removed fast                                                                                 |
| Cap usage                                   | Daily mint, deposit and refund usage above 80%                                                                                                                                                                                                      |
| Infrastructure                              | Disk above 70%, stalled node height, oldest Solana history within the challenge period of now                                                                                                                                                       |

## Incident runbooks

| Incident                           | First action                                                                                         | Follow-up                                                                                                                                                                             |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Suspected bad mint or verifier bug | Solana guardian pauses minting; x/sink guardian pauses deposits                                      | Reconcile supply and prepare a fix upgrade or re-pin                                                                                                                                  |
| Suspicious void quorum             | Any honest reporter vetoes. If evidence cannot be checked, the x/sink guardian pauses refunds        | Governance settles it with `ResolveTransfer`                                                                                                                                          |
| Client close to expiry             | Restart the relayer, switch RPC and check the payer balance                                          | Find why updates stopped                                                                                                                                                              |
| Client expired or frozen           | Mints and voids stop while Akash still accepts deposits, so disable deposits                         | Re-pin a new client after a new checkpoint ceremony ([commands](deploy-program.md#later-changes))                                                                                     |
| Relayer key leaked                 | Multisig revokes its `RELAYER_ROLE` and grants a new key                                             | Loss is limited to the payer balance; it cannot forge headers                                                                                                                         |
| Reporter key leaked                | Governance replaces the reporter. The epoch increments, so earlier void authorizations cannot refund | Other reporters watch for false votes                                                                                                                                                 |
| One reporter holds many transfers  | The x/sink guardian pauses deposits, so no new transfers are held                                    | One proposal: `MsgUpdateParams` removing the reporter, then `MsgResolveTransfer` RESTART for each transfer from `akash query sink open-transfers --held-only`                         |
| A mint quorum contradicts Solana   | An honest reporter's voided vote holds the Minted transfer                                           | One proposal: `MsgRequestVoid` and `MsgResolveTransfer` CONFIRM_VOID. The transfer returns to Voided with a new challenge window, then refunds; replace the reporters that voted mint |
| Multisig member key leaked         | Rotate the member                                                                                    | Review pending multisig transactions                                                                                                                                                  |
