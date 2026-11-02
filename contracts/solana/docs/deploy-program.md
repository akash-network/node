# Deploying the Solana programs

This is the command sequence for a fresh deployment of the AKT program, the
patched ICS07 verifier and its access manager, with every later change made
by a time-locked multisig. [Production deployment](production-deployment.md)
decides who holds each power and which values to use; this page says how.
Nothing here has run on mainnet.

`bash deploy/rehearse-launch.sh` runs every step below on a temporary local
validator, with a keypair standing in for the multisig vault. It executes each
multisig step through upstream's test CPI proxy, so instructions arrive by CPI
as they do from a multisig program, and checks that the access manager refuses
a CPI caller it does not list. It takes about ten minutes. Run it before the
ceremony, and again with the launch artifacts
([dress rehearsal](#dress-rehearsal)). It does not exercise a real multisig
program or its time lock.

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
export GENESIS=5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp   # mainnet-beta
export VAULT=<multisig vault address>
export MULTISIG_PROGRAM=<multisig program ID>       # check it against the deployed multisig
export CEREMONY=launch/keys/ceremony.json
admin() { npx tsx local/bridge-admin.ts "$@" --genesis "$GENESIS" --url "$RPC"; }
```

Prove control of `VAULT` before step 6: execute a small SOL transfer from it
through the multisig. Step 6 cannot check the address, and a wrong one loses
the programs. Fund the vault with about 0.1 SOL for the accounts it creates:
the bridge, guardian, metadata and light-client accounts.

## 1. Program keypairs

Generate the three program keypairs on an offline machine:

```bash
umask 077
mkdir -p launch/keys
for program in akt_solana ics07_tendermint access_manager; do
  solana-keygen new --no-bip39-passphrase --silent \
    --outfile "launch/keys/$program-keypair.json" > /dev/null
  echo "$program $(solana-keygen pubkey "launch/keys/$program-keypair.json")"
done
```

Record the three IDs as `AKT_ID`, `ICS07_ID` and `AM_ID`. A program keypair is
only needed for the first deploy. Delete all three after
[step 6](#6-upgrade-authorities-to-the-multisig). The ICS07 keypair matters most:
open relaying grants `RELAYER_ROLE` to the verifier's own ID, so whoever holds
that keypair holds the role ([verifier patches](verifier-patches.md)).

## 2. Launch commit

Program IDs are compiled in, so the launch build is a commit that changes only
them. Branch from the audited commit:

1. AKT: set `declare_id!` in `programs/akt-solana/src/lib.rs` to `AKT_ID`.
2. Verifier and access manager: their IDs live in the pinned upstream source.
   Add a patch that changes both `declare_id!` lines and append it to the
   series in `scripts/apply-light-client-patches.sh`:

   ```bash
   up=.deps/ibc-contracts
   for program in ics07-tendermint:HqPcGpVHxNNFfVatjhG78dFVMwjyZixoKPdZSt3d3TdD:$ICS07_ID \
                  access-manager:4fMih2CidrXPeRx77kj3QcuBZwREYtxEbXjURUgadoe1:$AM_ID; do
     IFS=: read -r dir old new <<< "$program"
     file=ibc-solana/programs/$dir/src/lib.rs
     sed "s/declare_id!(\"$old\")/declare_id!(\"$new\")/" "$up/$file" > "$up/$file.new"
     diff -u --label "a/$file" --label "b/$file" "$up/$file" "$up/$file.new" || true
     mv "$up/$file.new" "$up/$file"
   done > patches/program-ids.patch
   ```

   Start from a tree that `npm run setup:light-client` just patched. The diff
   must contain exactly two changed lines.

3. Regenerate the CPI interfaces. The setup's `cmp` fails once, because the
   IDL `address` fields change; copy the new IDLs and run it again:

   ```bash
   npm run setup:light-client || true
   cp .deps/ibc-contracts/ibc-solana/target/idl/{ics07_tendermint,access_manager}.json idls/
   git diff --stat idls/          # only the two "address" lines
   npm run setup:light-client
   npm run build                  # target/idl/akt_solana.json now names AKT_ID
   ```

4. Check and commit. `git diff` against the audited commit must show only the
   IDs, the patch and its entry in the series. Then run
   `cargo test -p akt-solana --lib`, `npm run typecheck` and
   `npx tsx --test local/admin-instructions.test.ts`. The contract suite
   (`npm test`) preloads the development IDs, so run it on the audited parent
   commit.

## 3. Reproducible build and hashes

```bash
npm run build:verifiable -- --rev <launch commit> launch/build
cat launch/build/SHA256SUMS launch/build/build-info.txt
```

At least two people build on different machines and compare `SHA256SUMS`.
Publish the launch commit, `SHA256SUMS` and `build-info.txt` before deploying.
Every later check compares on-chain bytes with these files.

### Dress rehearsal

Deploy the exact artifacts with the real program keypairs on a local validator,
through every step below:

```bash
AKT_REHEARSAL_PROGRAMS=launch/build AKT_REHEARSAL_KEYPAIRS=launch/keys \
  bash deploy/rehearse-launch.sh
```

## 4. Deploy

Generate the ceremony key and fund it. At their exact sizes the three programs
hold about 17 SOL of rent. `--max-len` reserves room for larger fixes; at twice
the build size they hold about 34 SOL. The largest deploy buffer, about 10 SOL,
is refunded after each deploy. `solana program extend` can grow a program
later instead.

```bash
solana-keygen new --no-bip39-passphrase --silent --outfile "$CEREMONY"
for program in akt_solana:$AKT_ID ics07_tendermint:$ICS07_ID access_manager:$AM_ID; do
  name=${program%%:*}
  solana program deploy "launch/build/$name.so" \
    --program-id "launch/keys/$name-keypair.json" \
    --upgrade-authority "$CEREMONY" --keypair "$CEREMONY" --url "$RPC" \
    --max-len $((2 * $(wc -c < "launch/build/$name.so"))) --commitment finalized
  bash deploy/verify-program.sh --url "$RPC" "${program#*:}" "launch/build/$name.so" \
    "$(solana-keygen pubkey "$CEREMONY")"
done
```

`verify-program.sh` dumps the program, compares the build's SHA-256 with the
first bytes, checks that the rest is zero padding and checks the upgrade
authority. Add `--with-compute-unit-price` to the deploys if mainnet is busy.

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
  solana program set-upgrade-authority "$id" --upgrade-authority "$CEREMONY" \
    --new-upgrade-authority "$VAULT" --skip-new-upgrade-authority-signer-check \
    --keypair "$CEREMONY" --url "$RPC"
done
bash deploy/verify-program.sh --url "$RPC" "$AKT_ID" launch/build/akt_solana.so "$VAULT"
bash deploy/verify-program.sh --url "$RPC" "$ICS07_ID" launch/build/ics07_tendermint.so "$VAULT"
bash deploy/verify-program.sh --url "$RPC" "$AM_ID" launch/build/access_manager.so "$VAULT"
```

The vault cannot sign this transfer, hence the skipped signer check. Then retire
the ceremony key: move its remaining SOL out and delete it and the program
keypairs. From here on, every change below is a multisig transaction.

## 7. Multisig transactions

Each step emits a file, reviewers check it, and the multisig executes it:

```bash
admin <command> [options] --emit launch/tx/NN-name.json --authority "$VAULT"
admin decode launch/tx/NN-name.json      # every reviewer, independently
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

Complete the [metadata launch gate](token-metadata.md#mainnet-launch-gate)
first.

```bash
admin set-token-metadata --akt-program "$AKT_ID" --uri <versioned HTTPS URL> \
  --emit launch/tx/07a-metadata.json --authority "$VAULT"
admin set-guardian --akt-program "$AKT_ID" --guardian <on-call key> \
  --emit launch/tx/07b-guardian.json --authority "$VAULT"
```

Both can go in one vault transaction:
`admin bundle --emit launch/tx/07-bundle.json launch/tx/07a-metadata.json launch/tx/07b-guardian.json`.
Verify with `admin show --akt-program "$AKT_ID"`: `metadata.uri` and `guardian`.

## 8. Checkpoint and bridge

After the Akash upgrade, pick a post-upgrade `akashnet-2` height. Each
checkpoint holder fetches it from at least three independent nodes:

```bash
admin checkpoint --height <H> --akash-rpc <node 1> --akash-rpc <node 2> \
  --akash-rpc <node 3> --out launch/checkpoint-<name>.json
```

It reads each node's `/commit`, requires a canonical commit, and fails unless
every node reports the same chain ID, time, app hash, validator hashes and
block hash. Compare the holders' files without their `sources`:

```bash
python3 -c 'import json, sys
for f in sys.argv[1:]:
    c = json.load(open(f)); c.pop("sources", None); print(json.dumps(c, sort_keys=True))' \
  launch/checkpoint-*.json | uniq -c       # one line, counted once per holder
```

`configure_bridge` names the light client's consensus state at its latest
height when it executes. A relayer update during the time lock would move that
height and fail the transaction. So initialize the client and configure the
bridge in one vault transaction, from the agreed checkpoint:

```bash
admin init-client --program "$ICS07_ID" --access-manager "$AM_ID" \
  --checkpoint launch/checkpoint.json --trusting-period 1209600 \
  --unbonding-period 1814400 --max-clock-drift <measured> \
  --emit launch/tx/08a-client.json --authority "$VAULT"
admin configure-bridge --akt-program "$AKT_ID" --program "$ICS07_ID" \
  --chain-id akashnet-2 --per-transfer-max 1000000000000 --daily-mint-cap 10000000000000 \
  --checkpoint launch/checkpoint.json \
  --emit launch/tx/08b-bridge.json --authority "$VAULT"
admin bundle --emit launch/tx/08-bridge.json launch/tx/08a-client.json launch/tx/08b-bridge.json
```

The policy shown is the launch default in base units: 1,000,000 AKT per
transfer and 10,000,000 AKT per UTC day. Set the x/sink `max_transfer` and
`solana_daily_mint_cap` to the same values.

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

```bash
admin set-bridge-enabled --akt-program "$AKT_ID" --enabled true \
  --emit launch/tx/09-enable.json --authority "$VAULT"
```

Nothing mints until Akash accepts deposits. Continue with
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
solana program write-buffer launch/build/akt_solana.so --keypair <payer> --url "$RPC"
solana program set-buffer-authority <BUFFER> --new-buffer-authority "$VAULT" \
  --keypair <payer> --url "$RPC"
admin upgrade-program --program "$AKT_ID" --buffer <BUFFER> --spill "$VAULT" \
  --expect-sha256 <published hash> --emit launch/tx/upgrade.json --authority "$VAULT"
# after execution
bash deploy/verify-program.sh --url "$RPC" "$AKT_ID" launch/build/akt_solana.so "$VAULT"
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

The instructions are built from the IDLs (`target/idl/akt_solana.json` and
`idls/`), so a rebuilt program changes them only where its interface
changes. `local/admin-instructions.test.ts` checks each against Anchor's own
builder and decodes it back.
