# Deploying the Solana programs

This is the command sequence for a fresh deployment of the AKT program, the
patched ICS07 verifier and its access manager, with every later change made
by a time-locked multisig. [Production deployment](production-deployment.md)
decides who holds each power and which values to use; this page says how.
Nothing here has run on mainnet.

For a launch with no seed allocation, explicit open relaying, and setup before
the Squads authority handover, use [the setup-first ceremony](mainnet-ceremony.md).
That sequence replaces this page's early handover and seed-window steps.

`bash deploy/rehearse-launch.sh` runs steps 4 to 9 below, then a guardian
pause, a re-pin and a program upgrade, on a temporary local validator, with a
keypair standing in for the multisig vault. It executes each
multisig step through upstream's test CPI proxy, so instructions arrive by CPI
as they do from a multisig program, and checks that the access manager refuses
a CPI caller it does not list. It takes about ten minutes. Run it before the
ceremony, and again with the launch artifacts
([dress rehearsal](#dress-rehearsal)). It does not exercise a real multisig
program or its time lock, or the manifest checks.

## Roles

| Role               | What it is                                                                                                                         |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------- |
| Ceremony key       | A one-time key. It deploys the programs, initializes the access manager and genesis, then hands every authority to the vault.      |
| Multisig vault     | The address that signs for the multisig, for example a Squads v4 vault PDA. It becomes every upgrade authority and the AM admin.   |
| Multisig program   | The program that executes vault transactions by CPI. The access manager must list it; see [step 5](#5-access-manager-and-genesis). |
| Reviewers          | Multisig members. They decode each emitted file themselves before approving.                                                       |
| Checkpoint holders | At least three people with their own `akashnet-2` nodes.                                                                           |

The admin CLI is `local/bridge-admin.ts`. Every chain command needs the
cluster's genesis hash and refuses another cluster. On mainnet it signs
locally only the ceremony steps and the guardian pause; everything else must
be emitted for the multisig.

```bash
export RPC=https://your-mainnet-rpc.example
export GENESIS=5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d   # full mainnet genesis hash
export VAULT='REPLACE_WITH_MULTISIG_VAULT'
export MULTISIG_PROGRAM='REPLACE_WITH_MULTISIG_PROGRAM'       # check it against the deployed multisig
export CEREMONY=/secure/akash-launch/ceremony.json
export PROGRAM_KEYS=/secure/akash-launch/program-keys
export RELEASE_DIR="${RELEASE_DIR:-$(cd ../.. && pwd)/.cache/solana/launch}"
export MANIFEST="$RELEASE_DIR/manifest.json"
mkdir -p "$RELEASE_DIR"
admin() { node --import tsx local/bridge-admin.ts "$@" --genesis "$GENESIS" --url "$RPC"; }
```

Prove control of `VAULT` before step 6: execute a small SOL transfer from it
through the multisig. Step 6 cannot check the address, and a wrong one loses
the programs. Fund the vault with about 0.1 SOL for the accounts it creates:
the bridge, guardian, metadata and light-client accounts.

## 1. Program keypairs

Generate the three program keypairs on an offline machine:

```bash
umask 077
mkdir -p "$PROGRAM_KEYS"
for program in akt ics07_tendermint access_manager; do
  solana-keygen new --no-bip39-passphrase --silent \
    --outfile "$PROGRAM_KEYS/$program-keypair.json" > /dev/null
  echo "$program $(solana-keygen pubkey "$PROGRAM_KEYS/$program-keypair.json")"
done
```

Record the three IDs as `AKT_ID`, `ICS07_ID` and `AM_ID`. Generate the ceremony
key now so its public key can be reviewed in the manifest:

```bash
solana-keygen new --no-bip39-passphrase --silent --outfile "$CEREMONY"
solana-keygen pubkey "$CEREMONY"
```

A program keypair is
only needed for the first deploy. Delete all three after
[step 6](#6-upgrade-authorities-to-the-multisig). The ICS07 keypair matters most:
open relaying grants `RELAYER_ROLE` to the verifier's own ID, so whoever holds
that keypair holds the role ([IBC verifier](verifier.md)).

## 2. Launch commit

Program IDs are compiled in, so the launch build is a commit that changes only
them. Branch from the audited commit:

1. AKT: set `declare_id!` in `programs/akt/src/lib.rs` to `AKT_ID`.
2. Verifier and access manager: change their `declare_id!` values in the
   **IBC contracts repository**, update its `Anchor.toml` mappings under
   `[programs.mainnet]`,
   build with its pinned Anchor toolchain and review that commit there. Do not
   patch the downloaded cache. Pin both Cargo dependencies in node
   (`programs/akt/Cargo.toml` and `tools/header-encoder/Cargo.toml`) to
   that reviewed full commit and regenerate both Cargo lockfiles.
3. Update this workspace's `Anchor.toml` AKT mapping under `[programs.mainnet]`
   to `AKT_ID`.
   Regenerate the two IBC IDLs using the IBC repository's Anchor build and
   copy the generated files to `idls/`. Review that only the intended address
   fields changed. Preserve or move aside the previous cached IBC checkout,
   then run `npm run setup:light-client` to verify the pin and IDLs, followed
   by `npm run build` to generate the AKT IDL and TypeScript interface.
4. Review both repositories against their audited revisions. The launch
   changes should contain the program IDs, source pins, regenerated lockfiles
   and interfaces. Run `npm run test:unit`, `npm run typecheck`,
   `npm run test:build` and `npm run test:deployment`. The contract suite
   preloads development IDs: run it on the audited parent and separately
   rehearse the launch artifacts at their actual IDs.

The IBC source revision must be available to an independent builder. Record
both commits in the release evidence. Local source changes and mutable branch
names are not release inputs.

The launch mappings use `[programs.mainnet]`, selected explicitly by the build
scripts. The AKT script prepares Anchor's `target/deploy` directory before
configuration discovery: Anchor 0.32.1 otherwise generates and syncs new IDs
when the localnet mapping is absent. New `target` paths point into the workspace
cache; existing Anchor target directories are preserved. The provider default
and test-proxy mapping remain `localnet` for local testing. Compiling does not
send a transaction. The verifiable build rejects changes to committed
lockfiles or program identities.

## 3. Reproducible build and hashes

Commit the reviewed launch changes before running this build.

```bash
npm run build:verifiable -- --rev "$(git rev-parse HEAD)" "$RELEASE_DIR/build"
cat "$RELEASE_DIR/build/SHA256SUMS" "$RELEASE_DIR/build/build-info.txt"
```

At least two people build on different machines and compare `SHA256SUMS`.
The bundle contains all three ELF binaries, IDLs and generated TypeScript
interfaces. Publish the launch commit, `SHA256SUMS` and `build-info.txt` before
deploying. Every later check compares against these reviewed artifacts.

Copy [`deploy/manifest.example.json`](../deploy/manifest.example.json) to
`$MANIFEST`. Set the source commit, all three program IDs, ceremony
public key and multisig vault. `buildDirectory` resolves relative to the
manifest. Set `checksumsSha256` to the SHA-256 of `SHA256SUMS` and
`buildInfoSha256` to the SHA-256 of `build-info.txt`, after the independent
builders agree. Do not regenerate these pins merely to bypass a mismatch.

```bash
cp deploy/manifest.example.json "$MANIFEST"
# Edit the public manifest and record its approval.
npm run deploy:programs -- check --manifest "$MANIFEST"
npm run deploy:programs -- preflight --manifest "$MANIFEST" --url "$RPC"
```

`check` is offline. It rejects placeholder/development IDs, mismatched IDs/IDLs,
unreviewed hashes, missing artifacts and inconsistent source provenance.
`preflight` checks the full genesis hash and finalized program state without
loading a signer. Missing programs are listed; occupied addresses must hold the
exact reviewed bytes and the ceremony authority. RPC errors fail the check.
After authority handover, use `verify --authority multisig` instead.

Use the full RPC genesis hash above, not Solana's shortened CAIP-2 network
identifier. The distinction is documented by
[Solana](https://solana.com/docs/payments/agentic-payments/x402#solana-networks-and-assets).

### Dress rehearsal

Deploy the exact artifacts with the real program keypairs on a local validator:

```bash
AKT_REHEARSAL_PROGRAMS="$RELEASE_DIR/build" AKT_REHEARSAL_KEYPAIRS="$PROGRAM_KEYS" \
  bash deploy/rehearse-launch.sh
```

It checks `SHA256SUMS` and that each keypair matches its declared program ID,
then runs steps 4 to 9. The manifest pins the mainnet genesis, and `preflight`
and `verify` require a cluster that reports it; the rehearsal runs no
`deploy:programs` command. It checks finalized bytes and authorities with
`deploy/verify-program.sh` instead, and uses a synthetic checkpoint rather than
`admin checkpoint`. `preflight` and both `verify` phases therefore first run
against a real cluster on mainnet; `npm run test:deployment` exercises them only
with fixtures and a mock RPC.

## 4. Deploy

Fund the ceremony key created in step 1 from
an approved estimate of rent, buffer space and fees. Calculate funding from the
actual artifact sizes and the current cluster rent; historical testnet SOL costs
are not a launch budget. The deploy command reserves twice each binary's size.
Review the priority fee explicitly in micro-lamports per compute unit.

Use `solana program deploy` directly. For each name in preflight's `missing`
list (`access_manager`, `ics07_tendermint`, then `akt`), compare the
ceremony and program public keys with the approved manifest, then deploy:

```bash
PROGRAM=access_manager
solana-keygen pubkey "$CEREMONY"
solana-keygen pubkey "$PROGRAM_KEYS/$PROGRAM-keypair.json"
solana --config /dev/null --url "$RPC" --keypair "$CEREMONY" \
  --commitment finalized program deploy "$RELEASE_DIR/build/$PROGRAM.so" \
  --program-id "$PROGRAM_KEYS/$PROGRAM-keypair.json" \
  --upgrade-authority "$CEREMONY" \
  --max-len "$((2 * $(wc -c < "$RELEASE_DIR/build/$PROGRAM.so")))" \
  --with-compute-unit-price 0
```

After all three programs are present, verify their finalized bytes, loader
ownership and ceremony authority against the reviewed manifest:

```bash
npm run deploy:programs -- verify --manifest "$MANIFEST" --url "$RPC" --authority ceremony
```

`deploy/programs.ts` only checks artifacts and on-chain state. Signing, upload
buffers and transaction retries belong to the Solana CLI. Continue with
initialization and authority handover below after verification passes.

**Interrupted deployment.** Retain Solana CLI output privately: it may contain
buffer recovery material. Rerun preflight to identify missing programs; do not
redeploy a completed program as an implicit upgrade. A changed binary, occupied
address or unexpected authority fails preflight and needs investigation. Recover
or close only that ceremony's buffers using the
[Solana deployment procedure](https://solana.com/docs/programs/deploying), then
resume the affected deployment with the CLI's `--buffer` option. Do not bulk-close
unrelated buffers.

Keep the release directory immutable during the ceremony. Record each deployment
signature and finalized verification with the approved manifest.

## 5. Access manager and genesis

The access manager's `initialize` rejects CPI, so the multisig cannot call it.
Admin calls through the multisig arrive by CPI, which the access manager
accepts only from programs it lists. The ceremony key therefore initializes it
as the first admin, lists the multisig program, optionally grants the first
relayer keys, grants `ADMIN_ROLE` to the vault and renounces its own, all in
one transaction:

```bash
admin init-access-manager --keypair "$CEREMONY" --access-manager "$AM_ID" \
  --admin "$VAULT" --cpi-caller "$MULTISIG_PROGRAM" [--relayer <key>]...
admin initialize --keypair "$CEREMONY" --akt-program "$AKT_ID"
admin show --akt-program "$AKT_ID" --access-manager "$AM_ID"
```

Both commands read their result back and fail on any difference. Check in
`show`:

- `token`: `decimals` 6, `supply` `0`, `freezeAuthority` null,
  `mintAuthorityIsPda` true. Genesis mints nothing and takes no treasury; all
  AKT on Solana is minted for proven Akash deposits.
- `accessManager`: `admins` exactly `[VAULT]`, `cpiCallers` exactly
  `[MULTISIG_PROGRAM]`, `relayers` as granted.
- `bridgeEnabled` false.

Relayer roles can also be granted later through the multisig, but only after
[step 8](#8-checkpoint-and-bridge): `grant-relayer` finds the access manager
through the light client's state.

## 6. Upgrade authorities to the multisig

```bash
for id in "$AKT_ID" "$ICS07_ID" "$AM_ID"; do
  test "$(solana genesis-hash --url "$RPC")" = "$GENESIS" || exit 1
  solana program set-upgrade-authority "$id" --upgrade-authority "$CEREMONY" \
    --new-upgrade-authority "$VAULT" --skip-new-upgrade-authority-signer-check \
    --keypair "$CEREMONY" --url "$RPC" --commitment finalized
done
npm run deploy:programs -- verify --manifest "$MANIFEST" --url "$RPC" --authority multisig
```

The vault cannot sign this transfer, hence the skipped signer check. Then retire
the ceremony key: move its remaining SOL out and delete it and the program
keypairs. From here on, every change below is a multisig transaction.

## 7. Multisig transactions

Each step emits a file, reviewers check it, and the multisig executes it:

```bash
mkdir -p "$RELEASE_DIR/tx"
admin <command> [options] --emit "$RELEASE_DIR/tx/NN-name.json" --authority "$VAULT"
admin decode "$RELEASE_DIR/tx/NN-name.json"      # every reviewer, independently
```

`--emit` signs and sends nothing. It reads chain state first and refuses
anything the program would reject: a wrong upgrade authority, a missing
account, a bridge already configured or an invalid policy. Then it writes:

- `instructions`: each with `programId`, `accounts` (`pubkey`, `isSigner`,
  `isWritable`) and base64 `data`. The vault is the only signer.
- `summary`: the same instructions decoded with the IDL: names, arguments
  and named accounts.
- `checks`: the state it read, such as the current policy or client height.
- `notes`: execution constraints.

`decode` rebuilds the summary from the bytes and fails if the file's summary,
account flags or data differ from what the IDL describes. Create one vault
transaction with exactly these instructions, in order. With Squads v4, build a
`TransactionMessage` from them for `vaultTransactionCreate`. Before approving,
compare the program IDs, accounts and data the multisig shows with the file.
After the time lock, execute it and run the step's verification. Then list
the AKT events it emitted:

```bash
admin events --akt-program "$AKT_ID" --signature <execution signature>
```

The events are emitted inside the multisig's CPI, where Anchor's
`EventParser` ignores them, so the CLI attributes them by invocation frame.
Expect `TokenMetadataSet`, `PauseGuardianSet`, `BridgeConfigured`,
`MintPolicySet`, `BridgeEnabledSet` or `LightClientRepinned` for the matching
step.

### Metadata and pause guardian

Use the approved [`metadata/akt.json`](../metadata/akt.json): name
**Akash Network Token**, symbol **AKT**, and the existing Cosmos AKT logo.
The mint keeps **6 decimals**. Host the JSON at a permanent public HTTPS URL
and pass it below. See [mainnet metadata](token-metadata.md#mainnet-values)
for the description and logo source.

Complete the [metadata launch gate](token-metadata.md#mainnet-launch-gate)
first.

```bash
admin set-token-metadata --akt-program "$AKT_ID" --uri <versioned HTTPS URL> \
  --emit "$RELEASE_DIR/tx/07a-metadata.json" --authority "$VAULT"
admin set-guardian --akt-program "$AKT_ID" --guardian <on-call key> \
  --emit "$RELEASE_DIR/tx/07b-guardian.json" --authority "$VAULT"
```

Both can go in one vault transaction:
`admin bundle --emit "$RELEASE_DIR/tx/07-bundle.json" "$RELEASE_DIR/tx/07a-metadata.json" "$RELEASE_DIR/tx/07b-guardian.json"`.
Verify with `admin show --akt-program "$AKT_ID"`: `metadata.uri` and `guardian`.

## 8. Checkpoint and bridge

After the Akash upgrade, pick a post-upgrade `akashnet-2` height. Each
checkpoint holder fetches it from at least three independent nodes:

```bash
admin checkpoint --height <H> --akash-rpc <node 1> --akash-rpc <node 2> \
  --akash-rpc <node 3> --out "$RELEASE_DIR/checkpoint-<name>.json"
```

It reads each node's `/commit`, requires a canonical commit, and fails unless
every node reports the same chain ID, time, app hash, validator hashes and
block hash. Compare the holders' files without their `sources`:

```bash
python3 -c 'import json, sys
for f in sys.argv[1:]:
    c = json.load(open(f)); c.pop("sources", None); print(json.dumps(c, sort_keys=True))' \
  "$RELEASE_DIR"/checkpoint-*.json | uniq -c       # one line, counted once per holder
```

`configure_bridge` names the light client's consensus state at its latest
height when it executes. A relayer update during the time lock would move that
height and fail the transaction. So initialize the client and configure the
bridge in one vault transaction, from the agreed checkpoint:

```bash
admin init-client --program "$ICS07_ID" --access-manager "$AM_ID" \
  --checkpoint "$RELEASE_DIR/checkpoint.json" --trusting-period 1209600 \
  --unbonding-period 1814400 --max-clock-drift <measured> \
  --emit "$RELEASE_DIR/tx/08a-client.json" --authority "$VAULT"
admin configure-bridge --akt-program "$AKT_ID" --program "$ICS07_ID" \
  --chain-id akashnet-2 --per-transfer-max 1000000000000 --daily-mint-cap 10000000000000 \
  --vault "$TREASURY_VAULT" --checkpoint "$RELEASE_DIR/checkpoint.json" \
  --emit "$RELEASE_DIR/tx/08b-bridge.json" --authority "$VAULT"
admin bundle --emit "$RELEASE_DIR/tx/08-bridge.json" "$RELEASE_DIR/tx/08a-client.json" "$RELEASE_DIR/tx/08b-bridge.json"
```

`TREASURY_VAULT` is the reviewed treasury vault that receives the seed. The
Akash proposal that opens transfers lists the same address in
`allowed_destination_vaults`. Without `--vault` the policy has no vaults, and an
off-curve treasury vault is voided as an invalid recipient instead of minted.

The example policy, in base units, holds seed-window values: 1,000,000 AKT per
transfer and 10,000,000 AKT per UTC day admit the seed transfer and are not the
launch caps. After the seed mints and Akash has lowered its caps, the multisig
lowers this policy to the launch values with `set-mint-policy`
([mint policy](production-deployment.md#solana-mint-policy)). It replaces the
whole policy, including `vaults`, so pass the treasury vault and any other vault
again with `--vault`. Set the x/sink
`max_amount_per_transfer` and `max_transfer_amount_per_epoch` to independently
chosen sending capacity. Solana keeps its own `daily_mint_cap`; neither amount
cap schedules mints.

`init-client` refuses a checkpoint older than the trusting period and notes
when the client expires without an update. The trusting period runs from the
checkpoint's time, so the time lock must end well before that. Start the
relayer as soon as the transaction executes. Verify with
`admin show --akt-program "$AKT_ID"`: `client.status` active at height `H`,
`chainId`, `policy` and `bridgeEnabled` false.

Relayers, through the multisig:

```bash
admin grant-relayer --akt-program "$AKT_ID" --key <relayer> --emit ... --authority "$VAULT"
admin open-relaying --akt-program "$AKT_ID" --emit ... --authority "$VAULT"   # if governance chooses it
admin relayers --akt-program "$AKT_ID"
```

## 9. Activation

Emit, review and approve this transaction before the voting period of the Akash
proposal that opens transfers ends, so the multisig time lock has expired when
it passes. Execute it right after confirming that the proposal passed with the
reviewed parameters:

```bash
admin set-bridge-enabled --akt-program "$AKT_ID" --enabled true \
  --emit "$RELEASE_DIR/tx/09-enable.json" --authority "$VAULT"
```

Until it executes, the relayer logs `relay-wait` (`bridge-paused`) for new
deposits and pays nothing. Continue with
[activation](production-deployment.md#5-activation).

## Later changes

| Change           | Command                                                                                  | Signer                   |
| ---------------- | ---------------------------------------------------------------------------------------- | ------------------------ |
| Mint policy      | `set-mint-policy --per-transfer-max N --daily-mint-cap N [--vault KEY]...`               | Multisig                 |
| Pause minting    | `pause --keypair <guardian key>`                                                         | Pause guardian, directly |
| Resume minting   | `set-bridge-enabled --enabled true`                                                      | Multisig                 |
| Guardian         | `set-guardian --guardian KEY`, `clear-guardian`                                          | Multisig                 |
| Relayers         | `grant-relayer --key KEY`, `revoke-relayer --key KEY`, `open-relaying`, `close-relaying` | Multisig                 |
| Metadata         | `set-token-metadata --uri URL`                                                           | Multisig                 |
| New light client | Deploy, then `init-client` and `repin --program ID --checkpoint FILE` in one bundle      | Multisig                 |
| Program upgrade  | `upgrade-program --program ID --buffer KEY --expect-sha256 HASH`                         | Multisig                 |

Raise caps on Solana before Akash, and lower them on Akash first
([parameters](production-deployment.md#parameters)).

**New light client.** Deploy the new verifier as in step 4 with a fresh
ceremony key and hand its authority to the vault as in step 6. Hold a new
checkpoint ceremony, then bundle `init-client` for the new deployment with
`repin --program <new ID> --checkpoint <file>`. Like `configure_bridge`,
`repin_light_client` names the client's latest consensus state; without the
bundle, hold that client's updates until the multisig executes.

**Program upgrade.** Write the reproducible build to a buffer, give the buffer
to the vault and emit the loader's upgrade. `upgrade-program` checks the buffer
authority and its contents' SHA-256:

```bash
solana program write-buffer "$RELEASE_DIR/build/akt.so" --keypair <payer> --url "$RPC"
solana program set-buffer-authority <BUFFER> --new-buffer-authority "$VAULT" \
  --keypair <payer> --url "$RPC"
admin upgrade-program --program "$AKT_ID" --buffer <BUFFER> --spill "$VAULT" \
  --expect-sha256 <published hash> --emit "$RELEASE_DIR/tx/upgrade.json" --authority "$VAULT"
# after execution
bash deploy/verify-program.sh --url "$RPC" "$AKT_ID" "$RELEASE_DIR/build/akt.so" "$VAULT"
```

A larger build needs `solana program extend` first.

## Instruction reference

| Program        | Instruction                 | Command                                                              | Signer                         |
| -------------- | --------------------------- | -------------------------------------------------------------------- | ------------------------------ |
| AKT            | `initialize`                | `initialize`                                                         | Ceremony key or vault          |
| AKT            | `set_token_metadata`        | `set-token-metadata`                                                 | Upgrade authority              |
| AKT            | `configure_bridge`          | `configure-bridge`                                                   | Upgrade authority              |
| AKT            | `set_bridge_enabled`        | `set-bridge-enabled`                                                 | Upgrade authority              |
| AKT            | `set_mint_policy`           | `set-mint-policy`                                                    | Upgrade authority              |
| AKT            | `set_pause_guardian`        | `set-guardian`                                                       | Upgrade authority              |
| AKT            | `repin_light_client`        | `repin`                                                              | Upgrade authority              |
| AKT            | `guardian_pause`            | `pause`                                                              | Pause guardian                 |
| ICS07          | `initialize`                | `init-client`                                                        | ICS07 upgrade authority        |
| Access manager | `initialize` and handover   | `init-access-manager`                                                | AM upgrade authority, directly |
| Access manager | `grant_role`, `revoke_role` | `grant-relayer`, `revoke-relayer`, `open-relaying`, `close-relaying` | AM admin                       |
| Loader         | `Upgrade`                   | `upgrade-program`                                                    | Program upgrade authority      |

The instructions are built from the IDLs (`../../.cache/solana/target/idl/akt.json` and
`idls/`), so a rebuilt program changes them only where its interface
changes. `local/admin-instructions.test.ts` checks each against Anchor's own
builder and decodes it back.
