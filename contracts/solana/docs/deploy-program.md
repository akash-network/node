# Deploy and administer the Solana contracts

This is the canonical contract deployment and administration guide. Mainnet is
already deployed; **do not rerun initialization on the existing programs**.
Use the workspace and multisig sections for current mainnet administration.
Steps 1–7 describe a fresh deployment with new program IDs and an unused native
Sink namespace, not an in-place upgrade of the existing bridge.

- [Mainnet workspace](#mainnet-workspace)
- [Fresh deployment](#1-program-keypairs)
- [Multisig transactions](#8-multisig-transactions)
- [Later changes and upgrades](#later-changes)
- [Monitoring](#monitoring)
- [Incident runbooks](#incident-runbooks)

The [artifact index](../deployments/mainnet/README.md) identifies builds, IDLs,
source pins and historical evidence. [Operator guides](https://akash.network/docs/node-operators/akash-sink/)
own reporter/relayer installation; the [native upgrade guide](../../../_docs/sink-deployment.md)
owns the Akash upgrade. Neither duplicates this contract procedure.

## Mainnet workspace

Run from `contracts/solana` in a checkout of the reviewed launch branch. Install
locked dependencies using [setup](setup.md) and [SDK integration](sdk-integration.md).
Prepare the admin CLI from the published interfaces without rebuilding programs:

```bash
export NODE_ROOT="$(git rev-parse --show-toplevel)"
export ARTIFACTS="$PWD/deployments/mainnet/upgrades/20261110-token-metadata"
mkdir -p "$NODE_ROOT/.cache/solana/target/idl" "$NODE_ROOT/.cache/solana/target/types"   "$NODE_ROOT/.cache/solana/deps/ibc-contracts/ibc-solana/target/types"
cp "$ARTIFACTS/build/akt.json" "$NODE_ROOT/.cache/solana/target/idl/akt.json"
cp "$ARTIFACTS/build/akt.ts" "$NODE_ROOT/.cache/solana/target/types/akt.ts"
cp "$ARTIFACTS/build/ics07_tendermint.ts" "$ARTIFACTS/build/access_manager.ts"   "$NODE_ROOT/.cache/solana/deps/ibc-contracts/ibc-solana/target/types/"
cmp idls/ics07_tendermint.json "$ARTIFACTS/build/ics07_tendermint.json"
cmp idls/access_manager.json "$ARTIFACTS/build/access_manager.json"
export RELEASE_DIR="$(mktemp -d "$NODE_ROOT/.cache/solana/admin.XXXXXX")"
mkdir -p "$RELEASE_DIR/tx"
export RPC=https://api.mainnet-beta.solana.com
export GENESIS=5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d
export AKT_ID=98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME
export ICS07_ID=2dTK4NwEkv3gyY5dZnG4TuDYFMoNYQYrBqEGwTSmEmKr
export AM_ID=HntLc7xrcRMRk9X38uMytCnweFJhVVCuzqhkVPe1RkCq
export VAULT=H2sXJRVW3SnTdFGjTXpGSsVKN2HnDaKc6xLRomS36H3U
export MULTISIG_PROGRAM=SQDS4ep65T869zMMBKyuUq6aD6EgTu8psMjkvj52pCf
export METADATA_URI=https://raw.githubusercontent.com/akash-network/net/130ca0ac6fab3bc1999e3612a1fbca83d57b439c/solana/mainnet/akt.json
admin() {
  node --import tsx local/bridge-admin.ts "$@"     --genesis "$GENESIS" --url "$RPC" --akt-program "$AKT_ID"
}
admin show --access-manager "$AM_ID"
```

Use an operator RPC for sustained work; public RPC can rate-limit. Only public
configuration belongs in source control. Keep keypairs, private RPC credentials,
runtime `.env` files and deployment recovery logs outside the repository.
Keypair JSON is private signing material; generated IDL JSON is a public interface.

The Squads account is `Hx92ZBfSJ7kLaDpmRYvh5TcnrSLWMnh5eDdpQqC9B3ke`, vault
index `0`. The vault above controls all three programs, the access-manager admin
role and the Solana pause guardian. On November 10 it required 2-of-5 approvals
with no time lock. Verify current settings before proposing changes. The vault
is a PDA: it has no private key to import.

## Fresh-deployment prerequisites

For a new deployment, use new program and ceremony keys, a new release directory
and a reviewed manifest. Set `CEREMONY` and `PROGRAM_KEYS` to protected paths
outside the repository and `MANIFEST="$RELEASE_DIR/manifest.json"`. Replace the
mainnet program IDs above with the generated IDs before compiling. Prove control
of the intended vault before handing over authority.

Initialization creates zero supply. There is no seed allocation, treasury funding
step or special first mint. This source-supported ceremony completes setup while
paused, hands over atomically, then activates through Squads.

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

Export the three IDs before compiling:

```bash
export AKT_ID="$(solana-keygen pubkey "$PROGRAM_KEYS/akt-keypair.json")"
export ICS07_ID="$(solana-keygen pubkey "$PROGRAM_KEYS/ics07_tendermint-keypair.json")"
export AM_ID="$(solana-keygen pubkey "$PROGRAM_KEYS/access_manager-keypair.json")"
```

Generate the ceremony
key now so its public key can be reviewed in the manifest:

```bash
solana-keygen new --no-bip39-passphrase --silent --outfile "$CEREMONY"
export CEREMONY_PUB="$(solana-keygen pubkey "$CEREMONY")"
```

A program keypair is
only needed for the first deploy. Retire them under the custody plan after verified
[handover](#7-upgrade-authorities-to-the-multisig). The ICS07 keypair matters most:
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
then exercises upload, setup, handover and activation with a test CPI proxy. The real Squads rehearsal is `deploy/rehearse-ceremony.sh`; its usage lists the required manifest, key directory and local validator settings. The manifest pins the mainnet genesis, and `preflight`
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
are not a launch budget. The example allocates exactly the reviewed binary size; future growth needs an explicit extension.
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
  --max-len "$(wc -c < "$RELEASE_DIR/build/$PROGRAM.so" | tr -d ' ')" \
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

Initialize directly with the ceremony wallet; access-manager initialization rejects
CPI. Allow Squads as a CPI caller. Grant the light client's own ID as a relayer
for explicit open header relaying. Add approved pruning payers with additional
`--relayer` flags; pruning stays restricted to members.

```bash
admin init-access-manager --keypair "$CEREMONY" --access-manager "$AM_ID"   --admin "$CEREMONY_PUB" --cpi-caller "$MULTISIG_PROGRAM" --relayer "$ICS07_ID"
admin initialize --keypair "$CEREMONY"
admin show --access-manager "$AM_ID"
```

Require supply `0`, decimals `6`, no freeze authority, the canonical mint-authority
PDA and `bridgeEnabled: false`. The ceremony wallet temporarily holds administration.
Prepare metadata and guardian setup, decode it, then execute after review:

```bash
admin set-token-metadata --uri "$METADATA_URI"   --emit "$RELEASE_DIR/tx/metadata.json" --authority "$CEREMONY_PUB"
admin set-guardian --guardian "$VAULT"   --emit "$RELEASE_DIR/tx/guardian.json" --authority "$CEREMONY_PUB"
admin bundle --emit "$RELEASE_DIR/tx/metadata-guardian.json"   "$RELEASE_DIR/tx/metadata.json" "$RELEASE_DIR/tx/guardian.json"
admin decode "$RELEASE_DIR/tx/metadata-guardian.json"
# After review:
admin ceremony-execute "$RELEASE_DIR/tx/metadata-guardian.json"   --manifest "$MANIFEST" --keypair "$CEREMONY"
```

Complete [checkpoint setup](#6-checkpoint-and-bridge) before handover. Start the
relayer immediately after initialization and reporters in preview after admission.
Relayer normal mode sends transactions; reporter preview signs nothing.

## 6. Checkpoint and bridge

During fresh setup, before handover, pick a post-upgrade `akashnet-2` height. Each
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
bridge in one ceremony transaction, from the agreed checkpoint:

```bash
admin init-client --program "$ICS07_ID" --access-manager "$AM_ID" \
  --checkpoint "$RELEASE_DIR/checkpoint.json" --trusting-period 1209600 \
  --unbonding-period 1814400 --max-clock-drift 60 --trust-level 1/3 \
  --emit "$RELEASE_DIR/tx/08a-client.json" --authority "$CEREMONY_PUB"
admin configure-bridge --akt-program "$AKT_ID" --program "$ICS07_ID" \
  --chain-id akashnet-2 --per-transfer-max 1000000000000 --daily-mint-cap 10000000000000 \
  --checkpoint "$RELEASE_DIR/checkpoint.json" \
  --emit "$RELEASE_DIR/tx/08b-bridge.json" --authority "$CEREMONY_PUB"
admin bundle --emit "$RELEASE_DIR/tx/08-bridge.json" "$RELEASE_DIR/tx/08a-client.json" "$RELEASE_DIR/tx/08b-bridge.json"
```

The example uses 14 days trusting, 21 days unbonding, 60 seconds clock drift and
trust level 1/3. Recheck native staking parameters and clock observations for a
new deployment. Never reuse the historical mainnet checkpoint. The example caps
are 1,000,000 AKT per transfer and 10,000,000 AKT per UTC day, in base units;
compare the current native governance limits. There is no seed or treasury step.

```bash
admin decode "$RELEASE_DIR/tx/08-bridge.json"
# After checkpoint and instruction review:
admin ceremony-execute "$RELEASE_DIR/tx/08-bridge.json"   --manifest "$MANIFEST" --keypair "$CEREMONY"
```

`init-client` refuses a checkpoint older than the trusting period and notes
when the client expires without an update. The trusting period runs from the
checkpoint's time, so the time lock must end well before that. Start the
relayer as soon as the transaction executes. Verify with
`admin show --akt-program "$AKT_ID"`: `client.status` active at height `H`,
`chainId`, `policy` and `bridgeEnabled` false.

Start the relayer now. Confirm its chain identity, access and client freshness;
headers refresh on demand or at the configured trust-period fraction, not every
poll. Reporters must complete preview after admission and switch to broadcast
before activation. Follow the [operator guides](https://akash.network/docs/node-operators/akash-sink/),
then return to [handover](#7-upgrade-authorities-to-the-multisig).

## 7. Upgrade authorities to the multisig

Require completed setup, a fresh active client, open relaying, the vault guardian,
zero supply and paused minting. Confirm member access and the squad's settings.
The launch CLI also requires an empty initial recipient-vault policy.

```bash
admin ceremony-handover --manifest "$MANIFEST"   --emit "$RELEASE_DIR/tx/handover.json" --authority "$CEREMONY_PUB"
admin decode "$RELEASE_DIR/tx/handover.json"
# After reviewing all five instructions:
admin ceremony-execute "$RELEASE_DIR/tx/handover.json"   --manifest "$MANIFEST" --keypair "$CEREMONY"
npm run deploy:programs -- verify --manifest "$MANIFEST" --url "$RPC" --authority multisig
admin show --access-manager "$AM_ID"
```

This atomically grants vault admin, removes ceremony admin and transfers all three
upgrade authorities. Require the vault as the only admin and every upgrade authority.
Retire ceremony and program keys under the custody plan after verification. Do not
reuse this launch-only command for an active bridge.

## 8. Multisig transactions

Each step emits a file, reviewers check it, and the multisig executes it:

```bash
mkdir -p "$RELEASE_DIR/tx"
admin <command> [options] --emit "$RELEASE_DIR/tx/NN-name.json" --authority "$VAULT"
admin decode "$RELEASE_DIR/tx/NN-name.json"      # every reviewer, independently
```

`--emit` signs and sends nothing. It reads chain state first and refuses
known invalid setup such as: a wrong upgrade authority, a missing
account, a bridge already configured or an invalid policy. Then it writes:

- `instructions`: each with `programId`, `accounts` (`pubkey`, `isSigner`,
  `isWritable`) and base64 `data`. The vault is the only signer.
- `summary`: the same instructions decoded with the IDL: names, arguments
  and named accounts.
- `checks`: the state it read, such as the current policy or client height.
- `notes`: execution constraints.

For web import, encode the reviewed file as an unsigned base58 transaction.
This container uses the vault as its placeholder fee payer, which makes that
account writable in the compiled message. Instruction data, account order and
intended authority remain the reviewed values. Squads builds the actual execution
transaction; this unsigned container is only for import:

```bash
export TX_FILE="$RELEASE_DIR/tx/NN-name.json"
node --import tsx --input-type=module - "$TX_FILE" <<'JS'
import { readFile } from "node:fs/promises";
import { web3 } from "@coral-xyz/anchor";
import bs58 from "bs58";
import { readEmitFile } from "./local/admin-instructions.ts";
const { authority, instructions } = readEmitFile(JSON.parse(await readFile(process.argv[2], "utf8")));
const tx = new web3.Transaction({
  feePayer: authority,
  recentBlockhash: web3.PublicKey.default.toBase58(),
}).add(...instructions);
console.log(bs58.encode(tx.serialize({ requireAllSignatures: false, verifySignatures: false })));
JS
```

In the [Squads app](https://app.squads.so/squads/H2sXJRVW3SnTdFGjTXpGSsVKN2HnDaKc6xLRomS36H3U/transactions?tab=Vault),
open **Developers → TX Builder → Create transaction → Add instruction → Import**.
Paste base58, review every instruction, simulate, then initiate the proposal.
Another member reviews and approves independently. A member with execution
permission executes after the threshold and configured time lock. Approval is
not execution. See the [official builder guide](https://docs.squads.so/main/navigating-your-squad/developers-assets/transaction-builder).
Record the proposal URL and finalized execution signature.

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
**Akash**, symbol **AKT**, and the existing Cosmos AKT logo.
The mint keeps **6 decimals**. Host the JSON at a permanent public HTTPS URL
and pass it below. See [mainnet metadata](token-metadata.md#mainnet-values)
for the description and logo source.

Complete the [metadata launch gate](token-metadata.md#mainnet-launch-gate)
first.

```bash
admin set-token-metadata --akt-program "$AKT_ID" --uri "$METADATA_URI" \
  --emit "$RELEASE_DIR/tx/07a-metadata.json" --authority "$VAULT"
admin set-guardian --akt-program "$AKT_ID" --guardian "$VAULT" \
  --emit "$RELEASE_DIR/tx/07b-guardian.json" --authority "$VAULT"
```

Both can go in one vault transaction:
`admin bundle --emit "$RELEASE_DIR/tx/07-bundle.json" "$RELEASE_DIR/tx/07a-metadata.json" "$RELEASE_DIR/tx/07b-guardian.json"`.
Verify with `admin show --akt-program "$AKT_ID"`: `metadata.uri` and `guardian`.

## 9. Activation

Require healthy relaying, the current governance-listed reporter quorum broadcasting,
funded signers, backing checks and working alert delivery. On November 10, five
reporters at threshold `0.667` required four votes, separate from Squads approvals.
Query the current policy before activation.

```bash
admin set-bridge-enabled --enabled true   --emit "$RELEASE_DIR/tx/enable.json" --authority "$VAULT"
admin decode "$RELEASE_DIR/tx/enable.json"
# After Squads approval and execution:
admin show --access-manager "$AM_ID"
```

Require `bridgeEnabled: true`. Send an agreed small ordinary deposit using the
[akt CLI](https://akash.network/docs/developers/akt/sink/); the portal need not be
available. Confirm a successful Akash receipt, finalized Solana mint with the
correct recipient and amount, native Minted settlement and reconciled backing.
Save both transaction links and the full owner/height/sequence ID. Do not repeat
a deposit to compensate for missing evidence.

## Later changes

| Change           | Command                                                                                  | Signer                          |
| ---------------- | ---------------------------------------------------------------------------------------- | ------------------------------- |
| Mint policy      | `set-mint-policy --per-transfer-max N --daily-mint-cap N [--vault KEY]...`               | Multisig                        |
| Pause minting    | `pause --emit FILE --authority "$VAULT"`                                                 | Squads vault (mainnet guardian) |
| Resume minting   | `set-bridge-enabled --enabled true`                                                      | Multisig                        |
| Guardian         | `set-guardian --guardian KEY`, `clear-guardian`                                          | Multisig                        |
| Relayers         | `grant-relayer --key KEY`, `revoke-relayer --key KEY`, `open-relaying`, `close-relaying` | Multisig                        |
| Metadata         | `set-token-metadata --uri URL`                                                           | Multisig                        |
| New light client | Deploy, then `init-client` and `repin --program ID --checkpoint FILE` in one bundle      | Multisig                        |
| Program upgrade  | `upgrade-program --program ID --buffer KEY --expect-sha256 HASH`                         | Multisig                        |

Raise caps on Solana before Akash, and lower them on Akash first
(the caps are governed independently).

**New light client.** Deploy a compatible verifier with a fresh ID and transfer
its upgrade authority to the vault using the loader. The launch handover command
is not a single-program migration procedure. Hold a new
checkpoint ceremony, then bundle `init-client` for the new deployment with
`repin --program <new ID> --checkpoint <file>`. Like `configure_bridge`,
`repin_light_client` names the client's latest consensus state; without the
bundle, hold that client's updates until the multisig executes.

**Program upgrade.** Select the independently reviewed new build as `UPGRADE_BUILD` and the funded
upload payer as `UPGRADE_PAYER` (a protected file outside the repository). Write
that build to a buffer, give the buffer to the vault and emit the loader's upgrade. `upgrade-program` checks the buffer
authority and its contents' SHA-256:

```bash
: "${UPGRADE_BUILD:?Set the reviewed new build directory}"
: "${UPGRADE_PAYER:?Set the protected upload-payer keypair path}"
solana --config /dev/null program write-buffer "$UPGRADE_BUILD/akt.so" \
  --keypair "$UPGRADE_PAYER" --url "$RPC"
solana --config /dev/null program set-buffer-authority <BUFFER> --new-buffer-authority "$VAULT" \
  --keypair "$UPGRADE_PAYER" --url "$RPC"
admin upgrade-program --program "$AKT_ID" --buffer <BUFFER> --spill "$VAULT" \
  --expect-sha256 <published hash> --emit "$RELEASE_DIR/tx/upgrade.json" --authority "$VAULT"
# after execution
bash deploy/verify-program.sh --url "$RPC" "$AKT_ID" "$UPGRADE_BUILD/akt.so" "$VAULT"
```

A larger build needs `solana program extend` first.

Review source scope, binary hashes and account-layout compatibility before an
upgrade. Upgrade only changed programs; preserving addresses alone does not prove
compatibility. After a token-name or symbol code change, a separate metadata
instruction updates the existing metadata account. See the [metadata artifact record](../deployments/mainnet/upgrades/20261110-token-metadata/README.md).

`set-mint-policy` replaces the whole policy, including `vaults`; preserve intended
entries. An empty list removes off-curve exceptions, not validation. Off-curve
recipients also need Akash `allowed_destination_vaults` approval. On November 10,
Solana listed the Squads vault but the native list was empty. Administrative
control does not make a vault an eligible recipient. Ordinary wallets need no
vault allowlisting.

## Monitoring

Route alerts to an on-call operator and test delivery. The separate
[akash-sink-monitor](https://github.com/akash-network/akash-sink-monitor) watches
both chains and can attach unsigned Squads pause import data. Its deployment and
Slack settings belong to the monitoring host. Alerts do not execute a pause:
members must approve and execute it. A Solana pause stops new mints; it does not
freeze issued tokens or pause native deposits/refunds. Use the native guardian
for Akash pauses; governance resumes them.

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

| Incident                           | First action                                                                                                                                           | Follow-up                                                                                                                                                                                                                                                                               |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Suspected bad mint or verifier bug | Solana guardian pauses minting; x/sink guardian pauses deposits                                                                                        | Reconcile supply and prepare a fix upgrade or re-pin                                                                                                                                                                                                                                    |
| Suspicious void quorum             | Any honest reporter vetoes. If evidence cannot be checked, the x/sink guardian pauses refunds                                                          | Governance uses `MsgConfirmMint` or `MsgConfirmVoid` with current `expected_round` and checked evidence                                                                                                                                                                                 |
| Client close to expiry             | Restart the relayer, switch RPC and check the payer balance                                                                                            | Find why updates stopped                                                                                                                                                                                                                                                                |
| Client expired or frozen           | Mints and voids stop while Akash still accepts deposits, so disable deposits                                                                           | Re-pin a new client after a new checkpoint ceremony ([commands](#later-changes))                                                                                                                                                                                                        |
| Relayer key leaked                 | Stop the service, protect its remaining SOL, replace the payer and revoke pruning membership                                                           | The key cannot forge headers. With open relaying, membership revocation does not block permissionless header submission                                                                                                                                                                 |
| Reporter key leaked                | Governance replaces the reporter. The policy version advances; unpaid quorum refunds need new authorization while governance authorization is retained | Other reporters watch for false votes                                                                                                                                                                                                                                                   |
| One reporter holds many transfers  | The x/sink guardian pauses deposits, so no new transfers are held                                                                                      | One proposal: `MsgUpdateParams` removing the reporter, then `MsgRestartTransfer` with `expected_round` for each reviewed unpaid transfer from `akash query sink disputes`                                                                                                               |
| A mint quorum contradicts Solana   | An honest reporter's voided vote holds the Minted transfer                                                                                             | Replace the reporters that voted mint. The transfer stays Minted: AEP-95 treats Minted as final. Do not refund it with `MsgRequestVoid` and `MsgConfirmVoid`; x/sink still accepts that path for a conflict-held Minted transfer, which is a known defect (AEP-95 implementation notes) |
| Multisig member key leaked         | Rotate the member                                                                                                                                      | Review pending multisig transactions                                                                                                                                                                                                                                                    |

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
