# Verifier patches

The ICS07 Tendermint client and access manager are built from
`cosmos/ibc-contracts` at `dc78ade828b766b218fb6bcb5417fdaf8e553021`.
`scripts/apply-light-client-patches.sh` applies these patches in order:

| Patch                                 | Change                                                                     |
| ------------------------------------- | -------------------------------------------------------------------------- |
| `ics07-signature-batches.patch`       | Compact signature receipts; see [signature batches](signature-batches.md)  |
| `ics07-monotonic-latest-height.patch` | An accepted header older than the latest no longer lowers `latest_height`  |
| `ics07-consensus-pruning.patch`       | Adds `prune_consensus_state` to reclaim rent from expired consensus states |
| `ics07-open-relaying.patch`           | An access-manager marker lets any signer relay headers and misbehaviour    |

The patches form a series and must match exactly (`--fuzz=0`). A later patch may
change lines an earlier one added: the open relaying patch edits
`signature_batch.rs`, which the signature-batch patch creates. Setup copies the
files the series touches to a scratch tree, finds how many patches the tree
holds by reversing them in order to the base, checks that the whole series then
applies, and applies the rest. A tree that holds no exact prefix of the series,
such as a partial or edited one, fails without changes. A host with the first
three patches applied gains only the fourth. Setup records the patch hashes in
`.deps/ibc-contracts/.akt-patch-sha256`.

## Monotonic latest height

Upstream `assemble_and_update_client` set `latest_height` to the height of every
accepted header. A relayer could submit an authentic header older than the
latest, verified from an older trusted state. Its consensus state was stored and
`latest_height` moved backwards. A lagging second relayer can do this by
accident.

ibc-go stores that consensus state but raises `LatestHeight` only when the new
height is greater (`UpdateState`, `if height.GT(cs.LatestHeight)`). The patch
does the same, comparing revision number, then height.

The AKT program checks expiry against the consensus state at `latest_height` and
rejects proof heights above it. A lowered height could falsely expire the client
or reject proofs at heights that were already verified.

`npx tsx local/scale.ts latest` authenticates height 5, then an older header at
height 3, both trusted from height 1. It checks that height 3 is stored,
`latest_height` stays 5 and the client stays active. The unpatched program fails
this run with `latest_height` 3.

## Consensus-state pruning

Each accepted header creates an 88-byte `consensus_state` PDA holding 1,503,360
lamports of rent. Upstream never closes them. The test network created 536 in
12.5 hours, about 1.55 SOL a day.

`prune_consensus_state(height)` closes the state at `height` and returns its
rent to the signer. The signer must itself hold `RELAYER_ROLE`, checked by the
access manager's `require_role` with its direct-call requirement; the
[open relaying](#open-relaying) marker does not count. The instruction fails
with:

- `CannotPruneLatestConsensusState` when `height` is the client's latest height.
- `ConsensusStateNotExpired` unless `timestamp + trusting_period <= now` on the
  Solana clock. This is ibc-go's `IsExpired` rule. ibc-go prunes the same states
  during updates (`pruneOldestConsensusState`) and in its
  `PruneAllExpiredConsensusStates` migration.

Pruning works for active, expired and frozen clients. Existing instructions,
accounts and error codes keep their encoding; the IDL gains one instruction and
error codes 6034 and 6035.

### Why expired-only pruning is safe

- Verification uses a stored state as a trust anchor only while
  `timestamp + trusting_period > now`. `assemble_and_update_client` enforces this
  in tendermint-rs `is_within_trust_period`. Misbehaviour submission enforces it
  for both trusted states in ibc-rs `verify_misbehaviour_header`. An expired state
  can no longer authenticate a header or misbehaviour, so closing it removes no
  verification path. Misbehaviour detection is unchanged.
- A header at or below a pruned height can only be verified from a trusted state
  at a lower height. Block times increase with height, an assumption the light
  client already makes, so that state is older and also expired. A pruned height
  therefore cannot be filled again, and the conflicting-state freeze in
  `assemble_and_update_client` loses nothing.
- The latest state is never pruned. `client_status` and the AKT program's expiry
  check keep reading it and report an expired client rather than a missing
  account.
- Proofs against a pruned height fail. `verify_membership` returns Anchor
  `AccountNotInitialized`, and the AKT program's `mint_transfer` and
  `void_transfer` reject the account as `InvalidLightClient` before any CPI. The
  relayer proves against the latest height. A record written at height H can be
  proved at any later verified height while it remains in Akash state.

At steady state the client keeps one trusting period of states. On the test
network that is about one day, roughly 1.55 SOL at the observed update rate;
fewer updates lower it.

### Relayer helper

`pruneExpiredConsensusStates(wallet, { maxPerTick = 20, perTransaction = 8 })`
in `local/consensus-prune.ts` is meant for each relayer tick. One call:

- returns `skipped` without sending anything if the payer lacks `RELAYER_ROLE`;
- reads the client state and the Solana clock, then lists consensus states with
  one `getProgramAccounts` call filtered by discriminator and size, fetching 16
  bytes of each;
- selects expired states other than the latest, oldest first, at most
  `maxPerTick`;
- sends `perTransaction` prunes per transaction and stops at the first failure.

It returns `latestHeight`, `scanned`, `expired`, `pruned`, `failed`,
`remaining`, `reclaimedLamports`, `feeLamports` and `signatures`. The program
enforces every rule, so a stale selection costs at most one failed transaction
fee per call. The scan reads every consensus state; an indexed production
relayer should track the heights it created instead.

`npx tsx local/scale.ts prune` uses a 300-second trusting period with four
validators. It checks that pruning:

- rejects a non-latest state that has not expired;
- rejects a signer without `RELAYER_ROLE` and leaves the account unchanged;
- closes an expired state while the client is active, returning exactly
  1,503,360 lamports net of the fee;
- makes `verify_membership` at that height fail with `AccountNotInitialized`,
  and a second prune of it fail;
- leaves later updates working;
- rejects the latest height after it has expired.

It then runs the helper: a payer without the role is skipped, a two-state
limit prunes two states in one transaction, the next call prunes the last
expired state, and an idle call sends nothing. Reclaimed rent and fees reconcile
exactly with the payer balance, and the latest state remains.

The run passed on 2026-10-18 (`.cache/scale/run-ywjzKy`). One prune used 19,434
compute units; two in one transaction used 47,868. The host unit test
`expiry_boundary_matches_ibc_go` checks the boundary. With
`test_cpi_target` built, all 89 upstream `ics07-tendermint` library tests pass
against the patched program.

## Open relaying

Upstream checks `RELAYER_ROLE` on every relaying instruction, and the
access-manager admin grants it key by key. Header updates were therefore
limited to approved operators, and the client's liveness depended on them.
ibc-go has no such permission: any signer may send `MsgUpdateClient` or
`MsgSubmitMisbehaviour`.

`ics07-open-relaying.patch` adds a switch. The relaying role check passes if
the signer holds `RELAYER_ROLE` (role 1), or if the access manager lists the
ICS07 program's own ID as a `RELAYER_ROLE` member. That entry is the open
relaying marker:

| Action | Access-manager instruction, signed by its admin     |
| ------ | --------------------------------------------------- |
| Open   | `grant_role(1, <ICS07 program ID>)`                 |
| Close  | `revoke_role(1, <ICS07 program ID>)`                |
| Status | The ID is in the role 1 members of `access_manager` |

The program ID is the one the binary was built for (`crate::ID`): `HqPc…` on
the test network, the ceremony ID in production. `grant_role` rejects the
all-zero key, so the marker needs another reserved key; a program ID belongs to
no relayer. The check reads the access manager the client is configured with,
so each deployment opens or closes independently.

The new `require_relayer` helper replaces `require_role` in
`upload_header_chunk`, `initialize_signature_batch`,
`pre_verify_signature_batch`, `pre_verify_signature`,
`assemble_and_update_client`, `upload_misbehaviour_chunk` and
`assemble_and_submit_misbehaviour`. It keeps every other check of
`require_role`: the submitter signs, an instruction invoked by CPI fails with
`CpiNotAllowed` (instructions sysvar and stack height), and the access manager
is the PDA under the program in the client's app state. A refused signer still
gets `Unauthorized` (6000). The cleanup instructions were never gated, so a
relayer can recover upload rent after relaying closes. No account, instruction
or error changes, and the IDL is byte-identical.

`prune_consensus_state` keeps the member-only `require_role` and also rejects
the program ID as submitter. That second check is required. The testnet ID uses
upstream's published localnet keypair, and Agave 2.3 lets an executable account
receive lamports when `remove_accounts_executable_flag_checks` is active, as on
`solana-test-validator`. On a build without the check, with the marker granted,
a transaction signed with that keypair and listing the upgradeable loader, which
keeps the program account writable, pruned an expired state and moved its rent
into the program account (`.cache/scale/run-h3Ufs7`).

A program ID can sign whenever its keypair is known, so the marker is a real
`RELAYER_ROLE` member to any other program that checks that role on the same
access manager. Here the access manager governs only the ICS07 client; keep it
that way while relaying is open.

### Why open updates are safe

- A header is stored only after verification against a stored consensus state
  within its trusting period: validators with more than the trust level (1/3) of
  the trusted power signed it, and validator hashes, times and header binding
  match. Opening changes who may submit, not what is accepted.
- `latest_height` only increases ([monotonic latest height](#monotonic-latest-height)),
  so an old authentic header cannot move it back.
- A second, different authentic header at a stored height freezes the client.
  That is Akash validator misbehaviour, which anyone can now also prove with
  `assemble_and_submit_misbehaviour`.
- Header chunks, misbehaviour chunks and signature batches are keyed by
  submitter, and assembly reads only the submitter's chunks. A relayer cannot
  use or close another relayer's uploads. A batch passed by another submitter
  only proves that a signature was verified.
- Submitters pay their own fees and rent. Only a member can prune, so rent from
  expired states goes to members.

Remaining cost: anyone can create consensus states at authentic heights, at
1,503,360 lamports each, paid by them and never refunded to them. Members prune
them once expired, and each one lengthens the prune helper's scan. Anyone can
also create a legacy `sig_verify` receipt with `is_valid` false; the verifier
reads one only when the submitter passes it, and the relayer uses batches.

`npx tsx local/scale.ts open` runs on the real SBF runtime with four
validators. It checks that:

- by default a funded non-member gets `Unauthorized` from the six relaying
  instructions it can reach;
- with the marker granted, the non-member creates and fills a signature batch,
  writes a legacy receipt, updates the client to height 4 through the relayer's
  uploader and pays the new state's rent;
- while open, prune refuses the non-member, refuses the program ID as signer
  (`ConstraintMut` when the account is demoted, `Unauthorized` when the loader
  keeps it writable), and lets a member reach the expiry check;
- after the marker is revoked, all seven instructions refuse the non-member,
  its cleanup still returns its rent, and a member still updates;
- with the marker granted again, the non-member submits conflicting headers and
  freezes the client.

It passed on 2026-10-18 (`.cache/scale/run-dHYQ8v`). The receipts, recovery,
misbehaviour, expiry, latest-height, prune and split runs also passed with this
build (`run-cRLUFp`, `run-gIdkCR`, `run-donLTA`, `run-UnuKfX`, `run-B41eI8`,
`run-NX20eZ`, `run-j1UB2e`), and so did all 89 upstream `ics07-tendermint`
library tests. The check costs one more role scan. The split run's two-batch,
100-signature assembly used 1,090,855 CU against 1,079,492 before; PDA bump
searches for each run's random payer move that figure by thousands of CU. One
prune used 19,467 CU against 19,434.

## Builds and in-place upgrade

SBF output depends on the absolute build path. The verifier crates in
`.deps/ibc-contracts/packages` sit outside the `ibc-solana` Cargo workspace, so
their crate metadata hashes, and with them symbol order and code layout, include
that path. The same sources built at the workstation checkout and at a scratch
path gave the same size and strings but different code layout.

### Verifiable builds

`scripts/verifiable-build.sh` fixes the build path and toolchain, which closes
the build-path part of [blocker 12](production-deployment.md#operations):

```bash
bash scripts/verifiable-build.sh --rev <commit> [output-dir]
```

It archives the commit with `git archive`, so uncommitted changes never reach
the build, and builds `tools/verifiable-build/Dockerfile` from that commit for
linux/amd64, because Agave publishes x86_64 Linux binaries only. The image pins:

- `rust:1.89.0-slim-trixie` by digest, for Rust 1.89.0 and the glibc 2.39 that
  Anchor's release binary needs;
- Debian packages (`bzip2`, `curl`, `libudev1`, `make`, `patch`, `perl`,
  `protobuf-compiler`) from snapshot.debian.org at 2025-10-01;
- the Agave 2.3.13 release, platform tools v1.48 and the Anchor 0.32.1 release
  binary, each checked against a SHA-256 in the Dockerfile.

The container unpacks the archive at `/build`, fetches the pinned upstream
revision and checks its content hash (`6dd68161…`, files and symlinks, so a
recompressed GitHub tarball still matches), applies the patch series, and runs
`anchor build` for `ics07_tendermint`, `access_manager` and `akt_solana`. It
fails if a build changes a committed `Cargo.lock` or if a generated IDL differs
from `idls/`. The output directory, by default
`.cache/verifiable-build/<commit>`, receives the three `.so` files,
`SHA256SUMS`, and `build-info.txt` with the commit, local image ID, upstream
content hash, patch hashes and tool versions. The image is tagged
`akt-solana-verifiable-build:<commit prefix>` locally and must never be pushed.

On 2026-10-18, commit `17238fe` built from this checkout and from a
`git worktree` under a scratch path produced identical files
(`.cache/verifiable-build/17238fe-*`). An earlier build of the same files from
an uncommitted copy matched too.

| File                  |     Bytes | SHA-256                                                            |
| --------------------- | --------: | ------------------------------------------------------------------ |
| `akt_solana.so`       |   578,696 | `157de9b647bcaf740e429d0cc5dcd4c633a8bc377b0673cc3713f592c688ef22` |
| `ics07_tendermint.so` | 1,442,552 | `75aad2305e16e8f2d5b42cdcb32f184f4645a93140ac71e6cb8a180d3f188b6b` |
| `access_manager.so`   |   429,928 | `c334e3d6ff81da93e8faba8f23593efe66790f5971e30020e77dacb364d83ed6` |

The ICS07 and access-manager hashes hold for any commit with the same patches,
upstream revision and Dockerfile; `akt_solana.so` also depends on `programs/`,
`Cargo.lock` and `idls/`. The verifiable ICS07 and access-manager files also
passed `npx tsx local/scale.ts open` (`.cache/scale/run-2CyyD4`). On the Apple
Silicon workstation under Docker Desktop's x86_64 emulation, the image took
about two minutes to build and each run about ten, mostly Anchor's host IDL
builds.

To check a deployment, build its commit and compare `SHA256SUMS` with
`solana program dump` truncated to the file's length, as in the upgrade below.

### Workstation builds

`npm run setup:light-client` still builds in place for local tests. Its hashes
depend on the checkout path. Builds on one development
workstation, for attribution of test runs only:

| Patches applied                | `ics07_tendermint.so` SHA-256                                      |
| ------------------------------ | ------------------------------------------------------------------ |
| Signature batches (2026-10-12) | `06546f1c8a2bf794312972d5fe364b4eb017349df315a758d292348550d565c9` |
| Plus monotonic latest height   | `a81c91bc15ef39a52266f07600ae71652730cc25490caebc3c4a9eb43575f3df` |
| Plus consensus pruning         | `39ce504cb7adfdc522ddd3623b7f563afeb4c1f7ee6a45a9b2c32152bf7b875e` |
| Plus open relaying             | `c63107495aa1a8c2ea02dea5cc73c46b3ac82e3becd5bba2a5171548ca4861e1` |

`access_manager.so` is unchanged at
`23a5674d60a2097446d7bbe26b2a93dc06049344f35bd1cbc2bcba452060fb7a`.
