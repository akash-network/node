# Mainnet ceremony: setup before Squads handover

This sequence deploys AKT, the Tendermint light client, and its access manager
on existing Solana mainnet. The ceremony key completes setup while minting is
paused, then transfers all three upgrade authorities and the access-manager
admin role to the Squads vault in one transaction. Squads performs activation.

There is no new blockchain genesis file and no initial AKT allocation. The
AKT `initialize` instruction creates a mint with zero supply, six decimals,
the program's mint-authority PDA, and no freeze authority. Initial AKT follows
the same portal deposit and proof process as every later transfer.

## 1. Prepare the reviewed release

The prepared launch binaries, manifest, checksums and rebuild instructions are
in [`deployments/mainnet`](../deployments/mainnet/README.md). Copy that directory
outside the source tree before recording ceremony transactions, and complete
the independent build comparison before deployment.

Use the existing launch keypairs. A program ID is the public address of its
program keypair, compiled into the binary; it is different from the AKT mint
address. Do not generate replacements after reviewing or building the release.

Follow [launch source and reproducible build](deploy-program.md#2-launch-commit)
through the offline manifest check. The IBC dependency must be a fetchable full
commit, and the node source must also be committed. Keep the final binaries,
IDLs, TypeScript interfaces, `SHA256SUMS`, and `build-info.txt` together. Compare
the hashes with an independent builder before deploying.

Use the CLI checkout at the manifest's `sourceCommit`. Copy the release's
`akt.json` and `akt.ts` to `.cache/solana/target/idl/` and `types/` in the node
repository; copy the IBC TypeScript interfaces to
`.cache/solana/deps/ibc-contracts/ibc-solana/target/types/`. The committed IBC
IDLs must match the release. These generated interfaces belong to the build;
do not edit them to change addresses.

From `contracts/solana`, set paths and the RPC endpoint:

```bash
export RPC='https://your-mainnet-rpc.example'
export RELEASE_DIR='/absolute/path/to/reviewed-release'
export MANIFEST="$RELEASE_DIR/manifest.json"
export CEREMONY='/secure/path/ceremony.json'
export PROGRAM_KEYS='/secure/path/program-keys'
export GENESIS=5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d
export MULTISIG_PROGRAM=SQDS4ep65T869zMMBKyuUq6aD6EgTu8psMjkvj52pCf
export METADATA_URI=https://raw.githubusercontent.com/akash-network/net/84b94d96287d3507830a1d9832f55925f24b8dc8/solana/mainnet/akt.json

manifest_value() {
  node -e 'let v=require(process.argv[1]); for(const k of process.argv[2].split(".")) v=v[k]; process.stdout.write(v)' "$MANIFEST" "$1"
}
export VAULT=$(manifest_value upgradeAuthority)
export CEREMONY_PUB=$(manifest_value ceremonyAuthority)
export AKT_ID=$(manifest_value programs.akt)
export ICS07_ID=$(manifest_value programs.ics07_tendermint)
export AM_ID=$(manifest_value programs.access_manager)
export BUILD_DIR=$(node -e 'const p=require("node:path"),m=require(process.argv[1]); process.stdout.write(p.resolve(p.dirname(process.argv[1]),m.buildDirectory))' "$MANIFEST")
mkdir -p "$RELEASE_DIR/tx"
admin() {
  node --import tsx local/bridge-admin.ts "$@" \
    --genesis "$GENESIS" --url "$RPC" --akt-program "$AKT_ID"
}
```

Confirm the vault against the Squads account and vault index, and prove control
through a Squads transaction before handing anything over. Record its members,
threshold, and current time lock. Using the vault as pause guardian means a
pause also needs the Squads approvals and time lock. The Akash `x/sink` guardian
is a separate Akash address configured through Akash governance.

The metadata is hosted in `akash-network/net`, with the JSON and image pinned
to immutable commits. Fetch both raw HTTPS URLs, check their contents, and complete
the [metadata launch gate](token-metadata.md#mainnet-launch-gate).

Run the local rehearsal with the exact release and program IDs:

```bash
npm run rehearse:ceremony -- "$MANIFEST" "$PROGRAM_KEYS"
```

It uses disposable local authority keys, the real Squads v4 program, a 2-of-5
test multisig, and a synthetic checkpoint. The local validator uses eight ticks
per slot and waits for finalized blocks before setup. It exercises setup, the atomic
handover, rejection of the retired signer, Squads activation, and guardian
pause. It never uses the funded ceremony wallet or writes to mainnet. Evidence
remains under `.cache/solana/run/`; disposable keys and the ledger are removed.
The separate suite exercises valid header relay, misbehaviour submission, and
rejection of pruning by an ungranted signer and the marker's own keypair:

```bash
AKT_OPEN_RELAYING_PROGRAM_KEYPAIR="$PROGRAM_KEYS/ics07_tendermint-keypair.json" \
  npm run test:open-relaying
```

## 2. Fund and deploy the programs

Calculate rent from the actual final binaries and current mainnet RPC. Most
deployment SOL is the rent-exempt balance held in program accounts, rather
than transaction fees. Budget for initialized state accounts, upload fees, and
a retry margin as well. The commands below allocate each binary's current
size; a later larger upgrade needs additional program space and rent.

```bash
npm run deploy:programs -- check --manifest "$MANIFEST"
npm run deploy:programs -- preflight --manifest "$MANIFEST" --url "$RPC"
test "$(solana-keygen pubkey "$CEREMONY")" = "$CEREMONY_PUB"
```

For each program reported **missing**, in order `access_manager`,
`ics07_tendermint`, `akt`, run:

```bash
PROGRAM=access_manager
test "$(solana-keygen pubkey "$PROGRAM_KEYS/$PROGRAM-keypair.json")" = "$(manifest_value programs.$PROGRAM)"
test "$(solana --config /dev/null --url "$RPC" genesis-hash)" = "$GENESIS"
solana --config /dev/null --url "$RPC" --keypair "$CEREMONY" \
  --commitment finalized program deploy "$BUILD_DIR/$PROGRAM.so" \
  --program-id "$PROGRAM_KEYS/$PROGRAM-keypair.json" \
  --upgrade-authority "$CEREMONY" \
  --max-len "$(( $(wc -c < "$BUILD_DIR/$PROGRAM.so") ))" \
  --with-compute-unit-price 0
```

Keep deployment output private: interrupted uploads can print buffer recovery
material. After interruption, rerun preflight and use the existing
[buffer recovery procedure](deploy-program.md#4-deploy). Do not implicitly
upgrade a completed program by rerunning the deploy loop.

```bash
npm run deploy:programs -- verify --manifest "$MANIFEST" --url "$RPC" --authority ceremony
```

## 3. Initialize with open relaying and zero supply

Set `RELAYER_PUB` to the service payer's public key. Open relaying lets any
signer submit valid headers and misbehaviour; the service payer also needs an
explicit role for pruning. It must have its own SOL balance.

```bash
export RELAYER_PUB='REPLACE_WITH_SERVICE_PAYER_PUBLIC_KEY'
admin init-access-manager --keypair "$CEREMONY" --access-manager "$AM_ID" \
  --admin "$CEREMONY_PUB" --cpi-caller "$MULTISIG_PROGRAM" \
  --relayer "$ICS07_ID" --relayer "$RELAYER_PUB"
admin initialize --keypair "$CEREMONY"
admin show --access-manager "$AM_ID"
```

The access manager must still list only the ceremony key as admin. The
verifier's own program ID in `RELAYER_ROLE` is the explicit open-relaying
marker. The mint supply must be `0` and `bridgeEnabled` must be `false`.

## 4. Set metadata and the Squads guardian

```bash
admin set-token-metadata --uri "$METADATA_URI" \
  --emit "$RELEASE_DIR/tx/metadata.json" --authority "$CEREMONY_PUB"
admin set-guardian --guardian "$VAULT" \
  --emit "$RELEASE_DIR/tx/guardian.json" --authority "$CEREMONY_PUB"
admin bundle --emit "$RELEASE_DIR/tx/metadata-guardian.json" \
  "$RELEASE_DIR/tx/metadata.json" "$RELEASE_DIR/tx/guardian.json"
admin decode "$RELEASE_DIR/tx/metadata-guardian.json"
admin ceremony-execute "$RELEASE_DIR/tx/metadata-guardian.json" \
  --manifest "$MANIFEST" --keypair "$CEREMONY"
```

`ceremony-execute` verifies the manifest, all three deployed binaries and
authorities, the ceremony's sole admin role, zero supply, and paused minting.
It re-decodes the reviewed instructions and permits initial setup or the full
handover. It cannot enable minting, replace code, or transfer only some of the
authorities. Never use `--rehearsal` on mainnet.

## 5. Agree the checkpoint and launch limits

After the Akash upgrade, checkpoint holders select one post-upgrade
`akashnet-2` height and independently fetch it from at least three independent
nodes. Use the [checkpoint comparison procedure](deploy-program.md#8-checkpoint-and-bridge)
and save the agreed file as `$RELEASE_DIR/checkpoint.json`.

Approve these public inputs before constructing the transaction:

- `TRUSTING_PERIOD`, `UNBONDING_PERIOD`, `MAX_CLOCK_DRIFT`: seconds, consistent
  with Akash's consensus parameters and measured clock behaviour.
- `PER_TRANSFER_MAX`, `DAILY_MINT_CAP`: ordinary launch limits in base units;
  1 AKT is 1,000,000 base units. Choose the Akash sending limits separately.

There is no treasury destination, seed transaction, or temporary seed policy.
The policy's vault allowlist is empty. Squads controls administration; that
does not make it a special token recipient.

```bash
: "${TRUSTING_PERIOD:?Set the approved value}"
: "${UNBONDING_PERIOD:?Set the approved value}"
: "${MAX_CLOCK_DRIFT:?Set the approved value}"
: "${PER_TRANSFER_MAX:?Set the approved value}"
: "${DAILY_MINT_CAP:?Set the approved value}"
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
admin ceremony-execute "$RELEASE_DIR/tx/client-bridge.json" \
  --manifest "$MANIFEST" --keypair "$CEREMONY"
admin show --access-manager "$AM_ID" > "$RELEASE_DIR/before-handover.json"
```

Client initialization and bridge configuration execute atomically, so a public
relayer cannot move the checkpoint height between them. Check the chain,
checkpoint, limits, metadata, guardian, open-relaying marker, zero supply, and
paused bridge. Start the relayer promptly to keep the client fresh; it waits
on minting while the bridge is paused. Use the Linux ARM64 service archive on
the ARM64 host, following [service deployment](deploy-services.md).

## 6. Hand all authority to Squads

```bash
admin ceremony-handover --manifest "$MANIFEST" \
  --emit "$RELEASE_DIR/tx/handover.json" --authority "$CEREMONY_PUB"
admin decode "$RELEASE_DIR/tx/handover.json"
admin ceremony-execute "$RELEASE_DIR/tx/handover.json" \
  --manifest "$MANIFEST" --keypair "$CEREMONY"
npm run deploy:programs -- verify --manifest "$MANIFEST" --url "$RPC" --authority multisig
admin show --access-manager "$AM_ID" > "$RELEASE_DIR/after-handover.json"
```

Review the five instructions: grant the vault admin, renounce the ceremony
admin, and transfer each of the three program upgrade authorities to the
vault. All five succeed together or the transaction rolls back. The preflight
requires an active matching client, open relaying, Squads CPI permission, the
vault as guardian, metadata, and an empty recipient-vault allowlist.

Verify the vault is now the sole access-manager admin and every upgrade
authority. Setup values, zero supply, and paused minting must be unchanged.
After all release rehearsals and verification, retire the ceremony and program
keys according to the approved custody plan. The verifier keypair represents
the open-relaying marker's address, so do not use it as an operator wallet.

## 7. Activate through Squads

Confirm the reviewed Akash governance settings, relayer health, reporter
operation, and backing checks before opening transfers. Emit activation for
the vault:

```bash
admin set-bridge-enabled --enabled true \
  --emit "$RELEASE_DIR/tx/enable.json" --authority "$VAULT"
admin decode "$RELEASE_DIR/tx/enable.json"
```

Create a Squads vault transaction containing exactly the emitted instructions,
using the [multisig submission procedure](deploy-program.md#7-multisig-transactions).
Members compare the bytes, accounts, and program IDs, approve it, and execute
after the configured time lock. Record the signature and verify `admin show`.
The first AKT arrives through a normal portal transfer.

For a later pause, emit `admin pause --emit FILE --authority "$VAULT"` and
execute it through Squads the same way. There is no private key for the vault
PDA; do not try to pass it as a `--keypair`.
