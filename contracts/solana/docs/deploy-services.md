# Deploying operator services

Relayers and reporters run independently against existing production chains.
They do not start validators, initialize programs, change authorities or enable
Sink. Complete the [program ceremony](deploy-program.md), checkpoint and bridge
configuration before starting the relayer. Start each reporter in preview.

For independent downloads and Docker Compose, use [operator releases](operator-releases.md).
The manifest-based source build below remains available for deployment ceremonies.
It builds the relayer. The reporter is the separate Go binary `akt-reporter`, which
is not part of this archive: see [reporter hosts](#reporter-hosts).

## Build one reviewed release

Use a clean checkout of the launch commit on a Linux builder matching the
production CPU architecture and libc/distribution, with Node 24, npm, Rust and the pinned tools from
[setup](setup.md). Obtain the reviewed program build and
[manifest](deploy-program.md#3-reproducible-build-and-hashes). Prepare the exact
chain-sdk archive recorded by `package-lock.json` at node's
`.cache/solana/chain-sdk.tgz`; [SDK integration](sdk-integration.md) describes how
it is built. `npm ci` checks its lockfile integrity. Do not refresh the lockfile
as part of a deployment.

From `contracts/solana`, with absolute paths:

```bash
npm ci
npm run deploy:services -- /srv/launch/manifest.json /srv/releases/akt-services.tar.gz
```

The builder checks the reviewed commit and program bundle, then builds the
header encoder from the commit-pinned Cargo dependency with its lockfile. It installs the locked JS dependencies and
runs typechecking and deployment tests in an isolated staging tree, then removes
development dependencies with `npm prune --omit=dev`. The archive
contains the runtime source, dependencies (including `tsx`), generated IDLs and
types, SDK archive, native header encoder and `service-build-info.txt`.

Record the printed SHA-256, source commit, Node version, build platform and SDK
archive hash in the release record. The copied `launch-manifest.json` is
provenance; its build path belongs to the ceremony workspace. Program binaries
stay in that separately retained build bundle. The service archive contains no
operator config, keys, ledgers or relayer journals.

Staging uses a unique directory under node's `.cache/solana` and is removed on
success and failure. The final archive uses an adjacent temporary file and an
atomic hard link on its destination filesystem; an existing release is never
replaced. The adjacent file is removed on exit. Cargo/npm may also use their own
user caches. Program
reproducibility comes from the pinned program build; this host-specific service
archive is checksummed, not claimed to be reproducible across platforms.

## Install on each operator's host

Use separate hosts and Unix accounts for the relayer and each reporter. For the
relayer, install Node 24 at `/usr/local/bin/node` (or adjust the unit's root-owned
PATH). Verify the archive hash against the release record before extracting it.
Keep release files owned by root and unwritable by service accounts.

Example for a relayer, as the host administrator:

```bash
useradd --system --home-dir /var/lib/akt-relayer --shell /usr/sbin/nologin akt-relayer
install -d -o root -g root -m 0755 /opt/akt-bridge/releases/RELEASE
install -d -o root -g akt-relayer -m 0750 /etc/akt-relayer
tar -xzf /srv/releases/akt-services.tar.gz -C /opt/akt-bridge/releases/RELEASE
chown -R root:root /opt/akt-bridge/releases/RELEASE
chmod -R go-w /opt/akt-bridge/releases/RELEASE
ln -s releases/RELEASE /opt/akt-bridge/current.next
mv -Tf /opt/akt-bridge/current.next /opt/akt-bridge/current
```

Replace `RELEASE` with the reviewed commit or your immutable release identifier.
Create the user once; a subsequent release uses a new directory. Before a
replacement, stop the service and back up its configuration and durable state.

Copy the [relayer example](../relayer/config.example.json) to
`/etc/akt-relayer/config.json`. Replace every placeholder with the deployed
program/mint and operator identity. RPCs may be loopback proxies to private nodes.
The full mainnet genesis hash is pinned independently of the RPC endpoint.

For the relayer, explicitly set:

- `headerEncoder`: `/opt/akt-bridge/current/bin/akt-header-encoder`.
- `stateDir`: `/var/lib/akt-relayer`; retain this across releases and restarts.
- `payerKeyFile`: the operator-provisioned key file, owned by `akt-relayer`, mode
  `0600`. Use a separate funded payer per relayer. Keys are never release assets.

Check configuration without reading a payer:

```bash
cd /opt/akt-bridge/current/contracts/solana
node --import tsx deploy/service-config.ts relayer /etc/akt-relayer/config.json
```

This check rejects local network identities, development program IDs, implicit
encoder paths and cache-based relayer state. The services additionally check
live network identity, program state and freshness at startup.

### Reporter hosts

Each reporter runs `akt-reporter` on its own host and Unix account, with its own
Akash and Solana nodes. Its Akash node needs gRPC and a transaction index (see
[node requirements](reporter.md#akash-node)). Download and verify the binary as
[operator releases](operator-releases.md#download-without-docker) describes, then,
as the host administrator:

```bash
useradd --system --home-dir /var/lib/akt-reporter --shell /usr/sbin/nologin akt-reporter
install -m 0755 -o root -g root akt-reporter /usr/local/bin/akt-reporter
install -d -o akt-reporter -g akt-reporter -m 0700 /var/lib/akt-reporter /var/lib/akt-reporter/config
install -o akt-reporter -g akt-reporter -m 0600 reporter.toml /var/lib/akt-reporter/config/reporter.toml
```

Start from `reporter.example.toml` in the release archive. Replace every required
key with the deployed program, mint and operator identity (the
[configuration reference](operator-configuration.md#reporter-configuration) lists
them). RPCs may be loopback proxies to private nodes. The full mainnet genesis
hash is pinned independently of the RPC endpoint. Provide the signer as
[reporter custody](reporter.md#key-custody) describes: import a key into the
keyring with `keys import-hex` as the service account, or configure
`signer.remote.*`. Then validate offline, without opening the keyring or calling a
signer:

```bash
sudo -u akt-reporter /usr/local/bin/akt-reporter check --home /var/lib/akt-reporter
```

The check rejects local network identities, development program IDs and invalid
signer settings. The service additionally checks live network identity, program
state and freshness at startup.

## Supervise and activate

Install the matching unit from [`deploy/systemd`](../deploy/systemd). The units
run as service users, allow writes only to their state directories, and restart
failed processes with bounded restart attempts. The reporter unit runs
`/usr/local/bin/akt-reporter start --home /var/lib/akt-reporter`. Review a remote
signer's CA and certificate files and the `file` keyring backend's passphrase input
against the unit's sandbox; add only the required paths or settings in a drop-in.
Private systemd `/tmp` and signer-owned paths are separate from the repository
scratch policy.

```bash
install -m 0644 deploy/systemd/akt-relayer.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now akt-relayer
journalctl -u akt-relayer -f
```

On a reporter host install `akt-reporter.service` instead. It starts in preview
and neither opens the keyring nor calls the configured signer. Until
governance lists this reporter, each pass stops with `not a current reporter`,
so preview confirms only configuration and connectivity. After
governance adds this reporter and the preview/alert gates pass, set
`broadcast = true` in `/var/lib/akt-reporter/config/reporter.toml`, or install a
reviewed drop-in:

```ini
[Service]
Environment=AKT_REPORTER_BROADCAST=true
```

When replacing the TypeScript reporter, first follow the
[switch-over step](reporter-migration.md#steps): stop it, wait 21 blocks and two
more minutes, and never run both on one key. Run `systemctl daemon-reload` and
restart the reporter. Broadcast requires exactly one signer: `from` or
`signer.remote.address`. Run one active signing process per reporter account. A
warm standby is safe but wastes signatures and node load. Validate real signer
access and fees before activation; config validation alone does not prove either.
The unit gives the process `TimeoutStopSec=120`, which covers its 30-second
confirmation wait.

Route JSON alerts to paging and verify delivery. Inspect light-client age,
relayer progress and spend, reporter observations, finalized resolution history,
and reconciliation before accepting the release. See
[monitoring](production-deployment.md#monitoring). The existing local monitor,
reconciler and faucet/receipt server remain testnet tools; production monitoring
and the separate production portal are explicit launch prerequisites.

## Upgrade, rollback and recovery

1. Verify and unpack the new immutable archive; retain the previous release.
2. Stop the affected service. Back up its config and, for the relayer, the full
   state directory including upload journals and the spend ledger.
3. Check compatibility with the deployed program IDL, SDK and persisted state.
   Run config validation against the candidate release.
4. Atomically replace `current` as above, restart, and observe the acceptance
   signals. Change one operator at a time.
5. If startup or operation fails, stop the service, restore the previous
   compatible release/config and restart using the current durable state. Do
   not roll back journals/spend to an older snapshot after transactions have
   been sent: reconcile them first. Never delete `payer.lock` while its process
   is running or reset spend budgets simply to make a deployment pass.

A service rollback cannot undo on-chain transactions. Program upgrades and
light-client re-pins require the separate reviewed multisig procedure. An
expired or frozen client requires the [recovery runbook](production-deployment.md#incident-runbooks).
