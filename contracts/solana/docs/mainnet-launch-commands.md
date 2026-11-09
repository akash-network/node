# AKT mainnet: command-by-command launch

Run one block at a time. Review each result before continuing. Commands marked
**write** send transactions; `--emit` and `decode` only prepare/review files.
Use the existing keys and program IDs. There is no initial AKT allocation.

## Readiness

Checked on 2026-11-09: the ceremony wallet holds **20.1 SOL**; all three program
addresses are undeployed; the build and exact-byte local Squads rehearsal passed.
The [full local verification](mainnet-local-verification.md) also passed open
header relay, proof-backed minting, replay rejection and pause/resume with no
time lock.
Current program deposits total **14.80073748 SOL**. Including the measured state
allowance gives **14.90032660 SOL** before fees; the preparation target is 16 SOL.
Recheck balance and rent immediately before deployment.

Before the first write: independent build/hash comparison and human launch review,
including the [recorded compiler diagnostics](../deployments/mainnet/README.md#verification).
Before checkpoint setup: approved trusting period/drift, three independent Akash
RPCs, an agreed height, and a ready relayer with its public payer address.
Before handover: confirm Squads control, members, threshold and access to its
custom transaction builder. Observed threshold: **2 of 5**. The confirmed launch
time lock is **0 seconds (none)**, matching the current on-chain setting.
Before activation: the separately managed relayer, reporter and backing checks
must be ready. Service builds are outside this procedure.

## When the Linux services come up

These services remain owned by the separate service branch/PR. Their commands
run on the Linux hosts, not in the Mac deployment directory.

- **Before Step 3:** prepare the relayer host, install its header encoder, choose
  and fund its separate Solana payer, and provide `RELAYER_PUB`. The services
  do not need to run yet for Step 2's program uploads.
- **Immediately after Step 5:** start the relayer with the configured header
  encoder and confirm the light-client height advances while minting is still
  paused. Start reporters in preview and resolve configuration, RPC and alert
  errors. The header encoder is invoked by the relayer; it is not another
  Solana contract to deploy.
- **Before Step 7:** complete reporter preview checks, verify governance-listed
  reporter identities, signer access and Akash fee funding, then switch the
  intended reporters to broadcast. Confirm the required reporter quorum,
  relayer health, backing checks and monitoring before enabling minting.

Follow the [Linux service commands](deploy-services.md#supervise-and-activate).
Do not initialize the checkpoint and then leave the relayer offline indefinitely:
the trusting period runs from the trusted header's timestamp.

## 0. Set local paths and public settings

**Already prepared with direnv?** Enter the operator directory and let direnv
load. `MANIFEST`, `BUILD_DIR`, `CEREMONY`, `PROGRAM_KEYS`, the public IDs and
`admin` / `manifest_value` / `deploy_one` are already available. Keep using that
same directory so transaction records persist. Skip this setup block; start at
Step 2 after the read-only checks and release review are complete.

For a new manual setup, use the block below.

**Needs:** the prepared node checkout, Node 24/dependencies, Solana CLI 2.3.13,
and the existing launch-key directory. Replace the two local paths below.

```bash
cd /path/to/node/contracts/solana
export KEY_DIR='/secure/path/to/existing-launch-keys'
export RPC='https://api.mainnet-beta.solana.com'
mkdir -p "$PWD/../../.cache/launch"
export RELEASE_DIR=$(mktemp -d "$PWD/../../.cache/launch/mainnet-09005e02.XXXXXX")
mkdir -p "$RELEASE_DIR/tx"
cp -R deployments/mainnet/build "$RELEASE_DIR/"
cp deployments/mainnet/manifest.json "$RELEASE_DIR/manifest.json"
export MANIFEST="$RELEASE_DIR/manifest.json"
export BUILD_DIR="$RELEASE_DIR/build"
export CEREMONY="$KEY_DIR/ceremony.json"
export PROGRAM_KEYS="$KEY_DIR/program-keys"
export GENESIS=5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d
export MULTISIG_PROGRAM=SQDS4ep65T869zMMBKyuUq6aD6EgTu8psMjkvj52pCf
export METADATA_URI='https://raw.githubusercontent.com/akash-network/net/84b94d96287d3507830a1d9832f55925f24b8dc8/solana/mainnet/akt.json'

export PATH="$PWD/deploy/operator-bin:$PATH"
export VAULT=$(manifest_value upgradeAuthority)
export CEREMONY_PUB=$(manifest_value ceremonyAuthority)
export AKT_ID=$(manifest_value programs.akt)
export ICS07_ID=$(manifest_value programs.ics07_tendermint)
export AM_ID=$(manifest_value programs.access_manager)
```

The command creates a fresh release directory; keep its path for this ceremony.
The prepared checkout already has its generated interfaces and dependencies.
For a fresh machine, follow [workspace preparation](mainnet-ceremony.md#1-prepare-the-reviewed-release)
first. Production deployment commands match build source
`431953bb96a3cfdd1e1888210cae6dbf7d05a4ad`; the later artifact commit adds
documentation, artifacts and the local rehearsal-clock fix.

## 1. Verify the release and mainnet — read only

**Does:** checks file hashes, program IDs, network, existing accounts and funding.
**Needs:** step 0; independent comparison completed before proceeding to step 2.

```bash
(cd "$BUILD_DIR" && shasum -a 256 -c SHA256SUMS)
npm run deploy:programs -- check --manifest "$MANIFEST"
npm run deploy:programs -- preflight --manifest "$MANIFEST" --url "$RPC"
test "$(solana-keygen pubkey "$CEREMONY")" = "$CEREMONY_PUB"
test "$(solana --config /dev/null --url "$RPC" genesis-hash)" = "$GENESIS"
solana --config /dev/null --url "$RPC" --commitment finalized balance "$CEREMONY_PUB"
```

Stop on any mismatch. The expected preflight result currently lists all three
programs as missing. Another person must follow the
[independent rebuild commands](../deployments/mainnet/README.md#source-and-rebuild)
on another machine and compare all nine hashes. The local rehearsal has already
passed; it does not replace that comparison.

## 2. Upload the three programs — write

**Does:** creates the program accounts and stores the compiled code.
**Needs:** reviewed build, funded ceremony key, the three existing program keys,
and a successful mainnet preflight. Deploy only programs reported missing.

The prepared `deploy_one` command checks release hashes, signer/program IDs,
mainnet genesis and existing deployments before uploading. It refuses an
already-deployed program and verifies the uploaded bytes and ceremony authority
on success. Optional read-only previews are:

```bash
deploy_one --check access_manager
deploy_one --check ics07_tendermint
deploy_one --check akt
```

Run each separately, checking success before the next. Each command below
**sends mainnet transactions** and takes time to upload the binary.

**Access manager:** deploys the role and administration program.

```bash
deploy_one access_manager
```

**Tendermint verifier:** deploys the program that verifies Akash headers and proofs.

```bash
deploy_one ics07_tendermint
```

**AKT mint:** deploys the token and bridge program; it does not mint tokens yet.

```bash
deploy_one akt
```

Then verify the uploaded bytes and ceremony authority:

```bash
npm run deploy:programs -- verify --manifest "$MANIFEST" --url "$RPC" --authority ceremony
```

The helper captures upload output in owner-only files under `$RELEASE_DIR/logs`
and prints their paths. Keep those files private: failed uploads may include
buffer recovery material. It prints public verification results on success.
After a failure, rerun preflight and follow
[buffer recovery](deploy-program.md#4-deploy); do not blindly redeploy completed programs.

## 3. Initialize administration and the mint — write

**Does:** creates a paused mint with zero supply and enables explicit open relaying.
**Needs:** verified deployments and `RELAYER_PUB` from the separate service team.
That payer also receives the pruning role; it needs its own SOL for service work.
In the prepared direnv workspace, put this public address in `inputs.env` so it
persists when reopening the terminal. Do not put the private payer key there.

```bash
: "${RELAYER_PUB:?Set the service payer public key first}"
admin init-access-manager --keypair "$CEREMONY" --access-manager "$AM_ID" \
  --admin "$CEREMONY_PUB" --cpi-caller "$MULTISIG_PROGRAM" \
  --relayer "$ICS07_ID" --relayer "$RELAYER_PUB"
admin initialize --keypair "$CEREMONY"
admin show --access-manager "$AM_ID"
```

Expect sole ceremony admin, supply `0`, six decimals, no freeze authority and
`bridgeEnabled: false`. The verifier ID as a relayer is the explicit open marker;
confirm it in the access-manager roles now and `relaying.openRelaying` after step 5.

## 4. Set metadata and the pause guardian — prepare, review, write

**Does:** attaches the published AKT metadata and makes the Squads vault guardian.
**Needs:** step 3 and the [metadata checks](token-metadata.md#mainnet-launch-gate).
Upgrade/admin authority stays with the ceremony key until step 6.

```bash
admin set-token-metadata --uri "$METADATA_URI" \
  --emit "$RELEASE_DIR/tx/metadata.json" --authority "$CEREMONY_PUB"
admin set-guardian --guardian "$VAULT" \
  --emit "$RELEASE_DIR/tx/guardian.json" --authority "$CEREMONY_PUB"
admin bundle --emit "$RELEASE_DIR/tx/metadata-guardian.json" \
  "$RELEASE_DIR/tx/metadata.json" "$RELEASE_DIR/tx/guardian.json"
admin decode "$RELEASE_DIR/tx/metadata-guardian.json"
```

After reviewing the two decoded instructions:

```bash
admin ceremony-execute "$RELEASE_DIR/tx/metadata-guardian.json" \
  --manifest "$MANIFEST" --keypair "$CEREMONY"
admin show --access-manager "$AM_ID"
```

## 5. Set the checkpoint and bridge limits — read, prepare, review, write

**Does:** starts the Akash light client and connects it to the mint, still paused.
**Needs:** an agreed post-upgrade height `H`, three independent Akash RPC URLs,
approved `TRUSTING_PERIOD` and `MAX_CLOCK_DRIFT` in seconds, and the relayer ready
to keep the new client current immediately afterward. These remain unset inputs.

Recheck governance limits and staking unbonding before using the values below:

```bash
curl -fsSL https://api.akashnet.net/akash/sink/v1/params | jq .params
curl -fsSL https://akash-api.polkachu.com/akash/sink/v1/params | jq .params
curl -fsSL https://api.akashnet.net/cosmos/staking/v1beta1/params | jq .params.unbonding_time
```

Both Sink endpoints agreed on the values below at the readiness check. Stop if
the current results differ. The reporter challenge period is not the light-client
trusting period.

```bash
export PER_TRANSFER_MAX=1000000000000
export DAILY_MINT_CAP=10000000000000
export UNBONDING_PERIOD=1814400
: "${H:?Set the agreed checkpoint height}"
: "${AKASH_RPC_1:?Set independent RPC 1}"
: "${AKASH_RPC_2:?Set independent RPC 2}"
: "${AKASH_RPC_3:?Set independent RPC 3}"
admin checkpoint --height "$H" \
  --akash-rpc "$AKASH_RPC_1" --akash-rpc "$AKASH_RPC_2" --akash-rpc "$AKASH_RPC_3" \
  --out "$RELEASE_DIR/checkpoint.json"
```

Checkpoint holders must independently agree on the same result using the
[checkpoint comparison procedure](deploy-program.md#8-checkpoint-and-bridge).
Then prepare both instructions for one atomic transaction:

```bash
: "${TRUSTING_PERIOD:?Set the approved seconds}"
: "${MAX_CLOCK_DRIFT:?Set the approved seconds}"
admin init-client --program "$ICS07_ID" --access-manager "$AM_ID" \
  --checkpoint "$RELEASE_DIR/checkpoint.json" \
  --trusting-period "$TRUSTING_PERIOD" --unbonding-period "$UNBONDING_PERIOD" \
  --max-clock-drift "$MAX_CLOCK_DRIFT" \
  --emit "$RELEASE_DIR/tx/client.json" --authority "$CEREMONY_PUB"
admin configure-bridge --program "$ICS07_ID" --chain-id akashnet-2 \
  --per-transfer-max "$PER_TRANSFER_MAX" --daily-mint-cap "$DAILY_MINT_CAP" \
  --checkpoint "$RELEASE_DIR/checkpoint.json" \
  --emit "$RELEASE_DIR/tx/bridge.json" --authority "$CEREMONY_PUB"
admin bundle --emit "$RELEASE_DIR/tx/client-bridge.json" \
  "$RELEASE_DIR/tx/client.json" "$RELEASE_DIR/tx/bridge.json"
admin decode "$RELEASE_DIR/tx/client-bridge.json"
```

After reviewing the checkpoint, chain, limits and both instructions:

```bash
admin ceremony-execute "$RELEASE_DIR/tx/client-bridge.json" \
  --manifest "$MANIFEST" --keypair "$CEREMONY"
admin show --access-manager "$AM_ID" > "$RELEASE_DIR/before-handover.json"
```

Expect the agreed checkpoint, open relaying, supply `0` and minting still paused.
**Start the relayer now on its Linux host**, with its configured header encoder.
Run `admin show --access-manager "$AM_ID"` again after it has relayed a fresh
header: require `client.status: active` and an advancing `client.latestHeight`,
with `bridgeEnabled: false`. Start reporters in preview and review their results.
Keep the relayer running through handover and afterward. No treasury/seed
transfer or recipient-vault allowlist is configured.

## 6. Hand authority to Squads — prepare, review, write

**Does:** transfers all three upgrade authorities and sole access-manager admin
in one five-instruction transaction.
**Needs:** correct completed setup, a fresh active client, confirmed Squads
control, the approved threshold and the confirmed zero-second time lock. Do not
hand over an unfinished setup.

```bash
admin ceremony-handover --manifest "$MANIFEST" \
  --emit "$RELEASE_DIR/tx/handover.json" --authority "$CEREMONY_PUB"
admin decode "$RELEASE_DIR/tx/handover.json"
```

After reviewing all five instructions:

```bash
admin ceremony-execute "$RELEASE_DIR/tx/handover.json" \
  --manifest "$MANIFEST" --keypair "$CEREMONY"
npm run deploy:programs -- verify --manifest "$MANIFEST" --url "$RPC" --authority multisig
admin show --access-manager "$AM_ID" > "$RELEASE_DIR/after-handover.json"
```

Expect only the Squads vault as admin and every upgrade authority. Supply stays
zero and minting stays paused. Retire keys only under the agreed custody plan.

## 7. Activate through Squads — prepare, then member approvals/execution

**Does:** enables ordinary portal-backed minting.
**Needs:** step 6, healthy relayer/reporter, backing checks, available Squads
members for the required approvals. Reporter preview, signer access and fee
funding checks must have passed; the intended reporters must be broadcasting
and meet the governed quorum. The confirmed time lock is zero seconds.

```bash
admin set-bridge-enabled --enabled true \
  --emit "$RELEASE_DIR/tx/enable.json" --authority "$VAULT"
admin decode "$RELEASE_DIR/tx/enable.json"
```

These commands do not activate anything. In Squads, create a custom vault
transaction for account `Hx92ZBfSJ7kLaDpmRYvh5TcnrSLWMnh5eDdpQqC9B3ke`, vault
index `0`, containing exactly the emitted instructions. Review the program,
accounts and data, simulate, obtain the required approvals, and execute. No
additional time-lock wait is configured for this launch. Use the [Squads transaction builder](https://docs.squads.so/main/navigating-your-squad/developers-assets/transaction-builder)
and the [repository submission procedure](deploy-program.md#7-multisig-transactions).
The vault is a PDA; there is no vault keypair to pass to the ceremony CLI.

## 8. Verify activation and an ordinary portal transfer — read only here

**Does:** confirms activation and the first real transfer through the normal flow.
**Needs:** the Squads execution signature and a separately agreed portal test.

```bash
: "${ACTIVATION_SIGNATURE:?Set the Squads execution signature}"
admin events --signature "$ACTIVATION_SIGNATURE"
admin show --access-manager "$AM_ID"
npm run deploy:programs -- verify --manifest "$MANIFEST" --url "$RPC" --authority multisig
```

Expect `BridgeEnabledSet`, `bridgeEnabled: true`, correct metadata, open relaying
and vault authorities. Verify the agreed portal transfer's proof, minted amount
and destination. There is no separate initial mint or treasury funding step.
