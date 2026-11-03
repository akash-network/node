# Deploying operator services

Relayers and reporters run independently against existing production chains.
They do not start validators, initialize programs, change authorities or enable
Sink. Complete the [program ceremony](deploy-program.md), checkpoint and bridge
configuration before starting the relayer. Start each reporter in preview.

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

Use separate hosts and Unix accounts for the relayer and each reporter. Install
Node 24 at `/usr/local/bin/node` (or adjust the unit's root-owned PATH). Verify
the archive hash against the release record before extracting it. Keep release
files owned by root and unwritable by service accounts.

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
Use the same layout for `akt-reporter` on each reporter host.

Copy the appropriate [relayer](../relayer/config.example.json) or
[reporter](../reporter/config.example.json) example to `/etc/akt-ROLE/config.json`.
Replace every placeholder with the deployed program/mint and operator identity.
RPCs may be loopback proxies to private nodes. The full mainnet genesis hash is
pinned independently of the RPC endpoint. Each reporter needs independent
sources and an external signer as described in [reporter custody](reporter.md#key-custody).

For the relayer, explicitly set:

- `headerEncoder`: `/opt/akt-bridge/current/bin/akt-header-encoder`.
- `stateDir`: `/var/lib/akt-relayer`; retain this across releases and restarts.
- `payerKeyFile`: the operator-provisioned key file, owned by `akt-relayer`, mode
  `0600`. Use a separate funded payer per relayer. Keys are never release assets.

Check configuration without reading a payer or calling a signer:

```bash
cd /opt/akt-bridge/current/contracts/solana
node --import tsx deploy/service-config.ts relayer /etc/akt-relayer/config.json
# on a reporter host
node --import tsx deploy/service-config.ts reporter-preview /etc/akt-reporter/config.json
```

These checks reject local network identities, development program IDs, implicit
encoder paths and cache-based relayer state. The services additionally check
live network identity, program state and freshness at startup.

## Supervise and activate

Install the matching unit from [`deploy/systemd`](../deploy/systemd). The units
run as service users, allow writes only to their state directories, and restart
failed processes with bounded restart attempts. Review any external signer's
socket, credentials and filesystem needs against the reporter unit's sandbox;
add only the required paths in a drop-in. Private systemd `/tmp` and signer-owned
paths are separate from the repository scratch policy.

```bash
install -m 0644 deploy/systemd/akt-relayer.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now akt-relayer
journalctl -u akt-relayer -f
```

On a reporter host install `akt-reporter.service` instead. It starts in preview
and neither reads signing keys nor invokes the configured signer. After
governance adds this reporter and the preview/alert gates pass, install a
reviewed drop-in:

```ini
[Service]
ExecStart=
ExecStart=/bin/bash /opt/akt-bridge/current/contracts/solana/deploy/service.sh reporter-broadcast /etc/akt-reporter/config.json
```

Run `systemctl daemon-reload` and restart the reporter. The broadcast entry point
requires `externalSigner`; it does not accept local test key files. Run exactly
one signing process per reporter account. Validate real signer access and fees
before activation; config validation alone does not prove either.

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
