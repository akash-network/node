# Run a relayer or reporter

Relayer and reporter source lives in `node/contracts/solana`. Each service has
its own release and Docker image. Neither requires installing the Akash node,
Node.js, npm or Rust. Configuration selects a compatible bridge deployment;
changing its addresses does not require rebuilding the service.

The images contain the pinned Node runtime and production dependencies. Only
the relayer includes the header encoder and minimal Python runtime for its
process lock. Both run as an unprivileged user.

- [Docker Compose](#docker-compose)
- [Existing wallets](#existing-wallets)
- [Download without Docker](#download-without-docker)
- [Upgrade](#upgrade)
- [Configuration reference](operator-configuration.md)
- [Release a service](#release-a-service)

## Before you start

Use a Linux amd64 host and the deployment handoff's Akash chain ID, Solana
genesis hash, program/mint addresses and RPC endpoints. The contracts must
implement the release's supported interfaces. An image being available does
not establish that a mainnet deployment is ready.

The relayer needs SOL and permission to update the light client. A reporter
needs AKT for fees and admission to the Akash reporter policy. Each reporter
must use independent Akash and replaying Solana nodes with sufficient history
for the challenge period and expected outages. Run one signing process per key.

## Docker Compose

Install Docker Engine and Compose v2. Choose one role and an available version
from [node releases](https://github.com/akash-network/node/releases):

```bash
ROLE=reporter # or relayer
VERSION=0.1.0
mkdir -p "$HOME/akt-$ROLE"
cd "$HOME/akt-$ROLE"
BASE=https://github.com/akash-network/node/releases/download/$ROLE%2Fv$VERSION
curl -fLO "$BASE/akt-$ROLE-compose.yaml"
curl -fLO "$BASE/akt-$ROLE.env.example"
curl -fLO "$BASE/akt-$ROLE-config.example.json"
curl -fLO "$BASE/akt-${ROLE}_${VERSION}_checksums.txt"
sha256sum --ignore-missing --check "akt-${ROLE}_${VERSION}_checksums.txt"
mv "akt-$ROLE-compose.yaml" docker-compose.yaml
mv "akt-$ROLE.env.example" .env
mv "akt-$ROLE-config.example.json" config.json
```

Set the deployment identities and RPC URLs in `config.json`. Inside Docker, `127.0.0.1` means the
operator container, not your host or a different node container.

Create the host directories as the administrator. The default container user
is UID/GID `10001`; use `.env` to select a different dedicated account:

```bash
sudo install -d -o 10001 -g 10001 -m 0700 /etc/akt-$ROLE
sudo install -o 10001 -g 10001 -m 0600 config.json /etc/akt-$ROLE/config.json
# Relayer only: retain this directory across upgrades and container recreation.
sudo install -d -o 10001 -g 10001 -m 0700 /var/lib/akt-$ROLE
```

Use an existing key as described below, or generate a new one. Set `KEY` to
`reporter.key` for a reporter or `payer-keypair.json` for a relayer:

```bash
KEY=reporter.key
docker run --rm --user 10001:10001 \
  --mount type=bind,src=/etc/akt-$ROLE,dst=/keys \
  ghcr.io/akash-network/akt-$ROLE:$VERSION keys create --out /keys/$KEY
```

Creation refuses to overwrite a file and prints only the public address.
Back up the private key securely. The file must be owned by the container's
UID with mode `0600`. Do not put keys or seed phrases in `.env` or JSON config.

Edit `.env` to select the image, network and host paths. Set
`OPERATOR_NETWORK=testnet` for an isolated deployment; replace both mainnet
identities in `config.json`. Keep the example's **container** key path
`/run/keys/key`; the host path goes in `.env`. The relayer uses `/data` and
`/opt/akt/bin/akt-header-encoder` inside the container.

```bash
docker compose pull
docker compose run --rm "$ROLE" keys address --key-file /run/keys/key
docker compose run --rm "$ROLE" check --config /etc/akt/config.json --network mainnet
docker compose up -d
docker compose logs -f
```

Use `--network testnet` in the check command when `.env` selects testnet. The
check validates configuration without reading the key or contacting RPC. It
does not prove chain health. Watch runtime logs for verified chain identities,
fresh blocks and successful scans; missing history is not evidence of a void.

The reporter starts in **preview** mode. After governance admits its address
and preview scans pass, enable signing explicitly:

```bash
curl -fL "$BASE/akt-reporter-broadcast.yaml" -o akt-reporter-broadcast.yaml
sha256sum --ignore-missing --check "akt-reporter_${VERSION}_checksums.txt"
mv akt-reporter-broadcast.yaml broadcast.yaml
docker compose -f docker-compose.yaml -f broadcast.yaml up -d
```

Use both files for subsequent reporter restarts/upgrades. Omit the override to
return to preview. A relayer submits transactions in its normal run mode.

## Existing wallets

Use dedicated service wallets, never validator consensus keys.

**Reporter:** recover the existing wallet on a secure workstation only if it is
not already in your Akash CLI keyring. Use its actual derivation path and
verify the recovered address before exporting:

```bash
KEY_HOME="$HOME/.akash-reporter-export"
KEY_NAME=existing-reporter
akash keys add "$KEY_NAME" --recover --no-backup --keyring-backend file \
  --home "$KEY_HOME" --hd-path "m/44'/118'/0'/0/0"
akash keys show "$KEY_NAME" --address --keyring-backend file --home "$KEY_HOME"
(
  umask 077
  set -o noclobber
  akash keys export "$KEY_NAME" --unsafe --unarmored-hex --yes \
    --keyring-backend file --home "$KEY_HOME" > "$KEY_HOME/reporter.key"
)
```

Enter the mnemonic only at the recovery prompt. For a wallet with a BIP39
passphrase, add `--interactive`. Existing keyring users should use their own
home, key name and backend and skip recovery. The reporter reads a 32-byte
lowercase hex private key, not a mnemonic or encrypted armor. Transfer the file
securely and install it with the service UID and mode `0600`.

**Relayer:** use the existing Solana CLI JSON keypair. Check it with
`solana-keygen pubkey /secure/payer-keypair.json`, then install it as
`/etc/akt-relayer/payer-keypair.json` with the service UID and mode `0600`.

For either role, run the image's `keys address` command above and compare the
result with the existing wallet. Remove temporary transfer copies. Only signed
transactions leave the process; the private key is not sent to either chain.

## Download without Docker

Use Ubuntu 24.04 amd64 or a compatible glibc 2.39+ host with Bash, Python 3,
CA certificates and libstdc++6. Python supplies the relayer's process lock.
For the selected `ROLE`, `VERSION` and `BASE` above:

```bash
ASSET=akt-${ROLE}_${VERSION}_linux_amd64.tar.gz
curl -fLO "$BASE/$ASSET"
curl -fLO "$BASE/akt-${ROLE}_${VERSION}_checksums.txt"
sha256sum --ignore-missing --check "akt-${ROLE}_${VERSION}_checksums.txt"
mkdir "akt-$ROLE-$VERSION"
tar -xzf "$ASSET" -C "akt-$ROLE-$VERSION"
chmod -R a+rX,go-w "akt-$ROLE-$VERSION"
"$PWD/akt-$ROLE-$VERSION/bin/akt-$ROLE" --version
```

Install the complete extracted directory as root-owned files under
`/opt/akt-$ROLE/releases/$VERSION`. Use a dedicated unprivileged service account.
Copy `deploy/config.example.json`; replace Docker paths with absolute host
paths. For the relayer, set `headerEncoder` to the extracted
`bin/akt-header-encoder`, and keep `stateDir` outside the release directory.
Run `bin/akt-$ROLE check --config /absolute/config.json`, then
`bin/akt-$ROLE run --config /absolute/config.json --watch`. A reporter needs
`--broadcast` to sign. Add `--network testnet` for an isolated deployment.

## Upgrade

Stop the service before changing its image or installation path. Back up the
config and relayer journals; keep them outside the installation. Set
`OPERATOR_IMAGE` to the reviewed version/digest, run `docker compose pull`, then
`up -d` with the same Compose files. Verify chain identities and logs again.
Rollback selects the previous compatible image with the same key and journal
mounts. Do not run old and new instances with the same signing key.

## Release a service

Merging the PR does not publish images. Tag and release each operator separately.

The separate `release-operators` workflow packages **one** role. It does not
invoke node's release workflow, build `akash`, publish a node image or change
contracts. It builds generated interfaces from the checkout, tests the services,
builds an installable package, uses GoReleaser for its archive/checksums, and
exercises the extracted package and Docker Compose mounts before publishing.

Merge the reviewed changes into `main`, then tag that commit:

```bash
git fetch origin main --tags
git tag -s relayer/v0.1.0 origin/main -m 'akt-relayer v0.1.0'
git push origin relayer/v0.1.0
```

Use `reporter/v0.1.0` to release the reporter independently. A tag ending in
`-rc.1`, for example `relayer/v0.1.0-rc.1`, produces a prerelease; use this until
the deployment is qualified.

Pushing the tag automatically starts `release-operators`. It verifies that the
commit is on `main`, prepares the [pinned SDK](sdk-integration.md), runs the
checks above, then publishes the download, Compose files and
`ghcr.io/akash-network/akt-ROLE:VERSION` image. The relayer package includes the
Rust header encoder. No manual workflow inputs are needed. Operator releases
never become node's latest release.

For the first image publication, set the GHCR package visibility to **Public**.
The workflow checks anonymous image access before publishing the GitHub release;
if that check stops the first run, change visibility and rerun the draft release.

GoReleaser's free edition does not support monorepo tag prefixes. The dedicated
config uses its archive/snapshot mode with the validated service version;
the workflow publishes the exact namespaced tag. No temporary `v*` tag is created.
The existing node GoReleaser configuration remains unchanged. Native version
lookups select only `v[0-9]*` tags so operator tags cannot become node versions.

### Dependency review

The operator release disables npm install scripts and rejects packages containing
the native `bigint-buffer` binding. Zod includes the patched collection validator.
These inherited advisory dispositions apply only to the packaged operator paths:

| Dependency/advisories                                                                                           | Operator use                                                                                    |
| --------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `bigint-buffer`: GHSA-3gc7-fjrx-p6mg                                                                            | Uses the JavaScript fallback; the vulnerable native binding is not shipped.                     |
| `toml`: GHSA-82x6-q7mm-w9cf, GHSA-v5mp-jgw5-2x6j, GHSA-4qx8-w7c3-pj3p, GHSA-9j65-hcg3-gc64, GHSA-9qmh-m7c6-f92p | Anchor's TOML workspace loader is not used; services construct programs from bundled JSON IDLs. |
| `stream-json`: GHSA-528h-pc64-c93x, GHSA-hqr4-qq8f-hg3x, GHSA-mjw6-4jj6-33hc                                    | Uses web3's HTTP Jayson client, not its streaming server, filters or assembler.                 |
| `uuid`: GHSA-w5hq-g745-h8pq                                                                                     | HTTP request IDs use `v4()`; affected caller-supplied v3/v5/v6 buffers are not used.            |

These dependencies are not claimed to be fixed. The workflow records current
advisories and fails on an ID absent from this review. Reassess the dispositions
when dependencies or service call paths change.
