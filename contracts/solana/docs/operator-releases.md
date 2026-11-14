# Run a relayer or reporter

Relayer and reporter source lives in `node/contracts/solana` and the node
repository. Each service has its own release and Docker image. Neither requires
installing the Akash node, Node.js, npm or Rust. Configuration selects a
compatible bridge deployment; changing its addresses does not require rebuilding
the service.

The reporter, `akt-reporter`, is a static Go binary and a multi-arch (amd64 and
arm64) distroless image that runs as UID 10001. Moving from the TypeScript
reporter? Follow the [migration guide](reporter-migration.md). The relayer image
contains the pinned Node runtime, production dependencies, the header encoder and
a minimal Python runtime for its process lock. Both run as an unprivileged user.

- [Docker Compose](#docker-compose)
- [Existing wallets](#existing-wallets)
- [Download without Docker](#download-without-docker)
- [Upgrade](#upgrade)
- [Configuration reference](operator-configuration.md)
- [Release a service](#release-a-service)

## Before you start

Use the deployment handoff's Akash chain ID, Solana genesis hash, program/mint
addresses and RPC endpoints. The relayer needs a Linux amd64 host. The contracts
must implement the release's supported interfaces. An image being available does
not establish that a mainnet deployment is ready.

The relayer needs SOL and permission to update the light client. A reporter
needs AKT for fees and admission to the Akash reporter policy. Each reporter
must use independent Akash and replaying Solana nodes with sufficient history
for the challenge period and expected outages. The reporter's Akash node must
have gRPC enabled and a transaction index, and `node` and `grpc-addr` must reach
that one node ([node requirements](reporter.md#akash-node)). Run one active
signing process per key; a warm standby is safe but wasteful
([two processes on one key](reporter.md#two-processes-on-one-key)).

## Docker Compose

Install Docker Engine and Compose v2.24 or later. Choose an available version
from [node releases](https://github.com/akash-network/node/releases).

### Reporter

Use a directory and a Compose project of their own for the Go reporter. If the
TypeScript reporter runs from `$HOME/akt-reporter`, its project and service names
are the same as the Go reporter's, so setting up the Go files there would recreate
the TypeScript container as a Go preview and overwrite its `.env`. Never do that.
Leave the TypeScript directory alone until the
[migration](reporter-migration.md#steps) is complete. A fresh install with no
TypeScript reporter can use any directory.

```bash
VERSION=0.2.0
mkdir -p "$HOME/akt-reporter-go"
cd "$HOME/akt-reporter-go"
BASE=https://github.com/akash-network/node/releases/download/reporter%2Fv$VERSION
curl -fLO "$BASE/akt-reporter-compose.yaml"
curl -fLO "$BASE/akt-reporter.env.example"
curl -fLO "$BASE/reporter.env.example"
curl -fLO "$BASE/reporter.example.toml"
curl -fLO "$BASE/akt-reporter_${VERSION}_checksums.txt"
sha256sum --ignore-missing --check "akt-reporter_${VERSION}_checksums.txt"
mv akt-reporter-compose.yaml docker-compose.yaml
mv akt-reporter.env.example .env
echo COMPOSE_PROJECT_NAME=akt-reporter-go >> .env
cp reporter.env.example reporter.env
```

`.env` holds the Compose substitutions: `OPERATOR_IMAGE`, `OPERATOR_UID`,
`OPERATOR_GID` and `OPERATOR_HOME`, plus the project name that keeps this
deployment apart from any other. `reporter.env` is optional and holds
commented `AKT_REPORTER_*` overrides. Do not put keys or seed phrases in either
file. Inside Docker, `localhost` means the reporter container, not your host or
your nodes: set `node`, `grpc-addr` and `solana.rpc` to addresses the container
can reach.

The reporter's home is mounted read-only at `/home/akt-reporter`. Create it as the
administrator, owned by the container user (UID/GID `10001` by default; set
`OPERATOR_UID` and `OPERATOR_GID` in `.env` to use a dedicated account). The
TypeScript reporter kept its config and key in `/etc/akt-reporter`; give the Go
reporter a different home, such as `/etc/akt-reporter-go`, and set it as
`OPERATOR_HOME` in `.env`, so that its container never mounts the old key. Edit a
copy of the example, then install it:

```bash
REPORTER_HOME=/etc/akt-reporter-go
cp reporter.example.toml reporter.toml
"${EDITOR:-vi}" reporter.toml
sudo install -d -o 10001 -g 10001 -m 0700 "$REPORTER_HOME" "$REPORTER_HOME/config"
sudo install -o 10001 -g 10001 -m 0600 reporter.toml "$REPORTER_HOME/config/reporter.toml"
```

Set `reporter`, `chain-id`, `node`, `grpc-addr` and the `solana.*` keys. Set
`network = "testnet"` for an isolated deployment and replace both mainnet
identities. Then import the signing key into the home (see
[existing wallets](#existing-wallets)), or configure `signer.remote.*`, and set
`from = "reporter"` for a keyring key. The commands that write to the home need it
mounted read-write, so they run outside Compose:

```bash
IMAGE=ghcr.io/akash-network/akt-reporter:$VERSION
docker run --rm -i -v "$REPORTER_HOME":/home/akt-reporter "$IMAGE" \
  keys import-hex reporter < reporter.key
```

Back up the key securely. The files must be owned by the container's UID. Then
validate and start in preview:

```bash
docker compose pull
docker compose run --rm reporter keys show reporter --address
docker compose run --rm reporter check
docker compose up -d
docker compose logs -f
```

The Compose file has no `command`: the image's default command, `start`, runs.
`check` validates the configuration offline. It does not read the key, contact
RPC or prove chain health. Watch runtime logs for verified chain identities,
fresh blocks and successful scans; missing history is not evidence of a void.

The reporter starts in **preview**. After governance admits its address, preview
scans pass and, when replacing the TypeScript reporter, the
[switch-over wait](reporter-migration.md#steps) is over, enable signing: set
`broadcast = true` in `reporter.toml`, or `AKT_REPORTER_BROADCAST=true` in
`reporter.env`, then recreate the container:

```bash
docker compose up -d
```

Set it back to `false` to return to preview. After a successful migration the
TypeScript directory can be removed. Compose gives the reporter 60
seconds to stop, which covers its 30-second confirmation wait.

### Relayer

```bash
VERSION=0.1.0
mkdir -p "$HOME/akt-relayer"
cd "$HOME/akt-relayer"
BASE=https://github.com/akash-network/node/releases/download/relayer%2Fv$VERSION
curl -fLO "$BASE/akt-relayer-compose.yaml"
curl -fLO "$BASE/akt-relayer.env.example"
curl -fLO "$BASE/akt-relayer-config.example.json"
curl -fLO "$BASE/akt-relayer_${VERSION}_checksums.txt"
sha256sum --ignore-missing --check "akt-relayer_${VERSION}_checksums.txt"
mv akt-relayer-compose.yaml docker-compose.yaml
mv akt-relayer.env.example .env
mv akt-relayer-config.example.json config.json
```

Set the deployment identities and RPC URLs in `config.json`. Inside Docker, `127.0.0.1` means the
operator container, not your host or a different node container.

Create the host directories as the administrator. The default container user
is UID/GID `10001`; use `.env` to select a different dedicated account:

```bash
sudo install -d -o 10001 -g 10001 -m 0700 /etc/akt-relayer
sudo install -o 10001 -g 10001 -m 0600 config.json /etc/akt-relayer/config.json
# Retain this directory across upgrades and container recreation.
sudo install -d -o 10001 -g 10001 -m 0700 /var/lib/akt-relayer
```

Use an existing key as described below, or generate a new one:

```bash
docker run --rm --user 10001:10001 \
  --mount type=bind,src=/etc/akt-relayer,dst=/keys \
  ghcr.io/akash-network/akt-relayer:$VERSION keys create --out /keys/payer-keypair.json
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
docker compose run --rm relayer keys address --key-file /run/keys/key
docker compose run --rm relayer check --config /etc/akt/config.json --network mainnet
docker compose up -d
docker compose logs -f
```

Use `--network testnet` in the check command when `.env` selects testnet. The
check validates configuration without reading the key or contacting RPC. It
does not prove chain health. A relayer submits transactions in its normal run mode.

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
home, key name and backend and skip recovery. `akt-reporter keys import-hex`
reads a 32-byte lowercase hex private key from stdin, not a mnemonic or
encrypted armor. Transfer the file securely, import it as shown above, and
remove the file afterwards. The reporter's keyring then holds the key, owned by
the service UID with mode `0600`.

**Relayer:** use the existing Solana CLI JSON keypair. Check it with
`solana-keygen pubkey /secure/payer-keypair.json`, then install it as
`/etc/akt-relayer/payer-keypair.json` with the service UID and mode `0600`.

For the reporter, run `keys show reporter --address` as above; for the relayer,
the image's `keys address` command. Compare the result with the existing wallet.
Remove temporary transfer copies. Only signed transactions leave the process; the
private key is not sent to either chain.

## Download without Docker

**Reporter.** The archives hold a static binary, so the host needs no Node.js,
Python or libc version. Releases cover Linux and macOS on amd64 and arm64 (macOS
is for development: its builds lack the `os` keyring backend):

```bash
VERSION=0.2.0
BASE=https://github.com/akash-network/node/releases/download/reporter%2Fv$VERSION
ASSET=akt-reporter_${VERSION}_linux_amd64.tar.gz
curl -fLO "$BASE/$ASSET"
curl -fLO "$BASE/akt-reporter_${VERSION}_checksums.txt"
sha256sum --ignore-missing --check "akt-reporter_${VERSION}_checksums.txt"
mkdir "akt-reporter-$VERSION"
tar -xzf "$ASSET" -C "akt-reporter-$VERSION"
"$PWD/akt-reporter-$VERSION/akt-reporter" version
```

Install the binary root-owned at `/usr/local/bin/akt-reporter`, and run it as a
dedicated unprivileged account. Create the home, `/var/lib/akt-reporter`, as
`akt-reporter`, and copy `reporter.example.toml` from the archive to
`/var/lib/akt-reporter/config/reporter.toml`. Replace Docker addresses with real
ones. Then run `akt-reporter check --home /var/lib/akt-reporter` and install the
[systemd unit](deploy-services.md#supervise-and-activate), whose command is
`akt-reporter start --home /var/lib/akt-reporter`. Add `network = "testnet"` for
an isolated deployment.

**Relayer.** Use Ubuntu 24.04 amd64 or a compatible glibc 2.39+ host with Bash,
Python 3, CA certificates and libstdc++6. Python supplies the process lock:

```bash
VERSION=0.1.0
BASE=https://github.com/akash-network/node/releases/download/relayer%2Fv$VERSION
ASSET=akt-relayer_${VERSION}_linux_amd64.tar.gz
curl -fLO "$BASE/$ASSET"
curl -fLO "$BASE/akt-relayer_${VERSION}_checksums.txt"
sha256sum --ignore-missing --check "akt-relayer_${VERSION}_checksums.txt"
mkdir "akt-relayer-$VERSION"
tar -xzf "$ASSET" -C "akt-relayer-$VERSION"
chmod -R a+rX,go-w "akt-relayer-$VERSION"
"$PWD/akt-relayer-$VERSION/bin/akt-relayer" --version
```

Install the complete extracted directory as root-owned files under
`/opt/akt-relayer/releases/$VERSION`. Use a dedicated unprivileged service account.
Copy `deploy/config.example.json`; replace Docker paths with absolute host
paths. Set `headerEncoder` to the extracted `bin/akt-header-encoder`, and keep
`stateDir` outside the release directory. Run
`bin/akt-relayer check --config /absolute/config.json`, then
`bin/akt-relayer run --config /absolute/config.json --watch`. Add
`--network testnet` for an isolated deployment.

## Upgrade

Stop the service before changing its image or installation path. Back up the
configuration, the reporter home or the relayer config and journals; keep them
outside the installation. Set `OPERATOR_IMAGE` to the reviewed version/digest, run
`docker compose pull`, then `up -d` with the same Compose files. Verify chain
identities and logs again. Rollback selects the previous compatible image with
the same key and journal or home mounts. Do not run old and new instances with
the same signing key. A reporter that stops with transactions in flight loses
nothing: the next start plans the same messages again, and the chain executes at
most one transaction per message and window
([two processes on one key](reporter.md#two-processes-on-one-key)). The
TypeScript reporter is different; see the [migration guide](reporter-migration.md).

## Release a service

Merging the PR does not publish images. Tag and release each service separately.

**Reporter.** The `release-reporter` workflow runs on `reporter/v*` tags. It
verifies that the tag names a commit on `main` with no published release, runs the
reporter tests, builds a GoReleaser snapshot and smoke-tests the image, then
publishes the archives and checksums, the Compose file and examples, and the
multi-arch image `ghcr.io/akash-network/akt-reporter:VERSION`. A real `kms` binary
must pass the remote-signer gate, and the performance benchmarks must pass.
Reporter releases never become node's latest release. Tag the reviewed commit:

```bash
git fetch origin main --tags
git tag -s reporter/v0.2.0 origin/main -m 'akt-reporter v0.2.0'
git push origin reporter/v0.2.0
```

A tag ending in `-rc.1`, for example `reporter/v0.2.0-rc.1`, produces a
prerelease; use this until the deployment is qualified. For the first image
publication, set the GHCR package visibility to **Public**. The workflow checks
anonymous image access before publishing the GitHub release; if that check stops
the first run, change visibility and rerun the draft release.

GoReleaser's free edition does not support monorepo tag prefixes, so the workflow
passes the version in `GORELEASER_CURRENT_TAG` and publishes the exact namespaced
tag itself. The dedicated `.goreleaser-reporter.yaml` leaves the node's
GoReleaser configuration unchanged. Native version lookups select only
`v[0-9]*` tags so operator tags cannot become node versions. To build the
archives and images locally, run `make reporter-snapshot`.

**Relayer.** The separate `release-operators` workflow packages the relayer. It
does not invoke node's release workflow, build `akash`, publish a node image or
change contracts. It builds generated interfaces from the checkout, tests the
service, builds an installable package, uses GoReleaser for its archive/checksums,
and exercises the extracted package and Docker Compose mounts before publishing.

```bash
git fetch origin main --tags
git tag -s relayer/v0.1.0 origin/main -m 'akt-relayer v0.1.0'
git push origin relayer/v0.1.0
```

Pushing the tag automatically starts the workflow. It verifies that the commit
is on `main`, prepares the [pinned SDK](sdk-integration.md), runs the checks
above, then publishes the download, Compose files and
`ghcr.io/akash-network/akt-relayer:VERSION` image. The relayer package includes the
Rust header encoder. No manual workflow inputs are needed. Operator releases
never become node's latest release. A tag ending in `-rc.1` produces a
prerelease.

### Dependency review

The relayer release disables npm install scripts and rejects packages containing
the native `bigint-buffer` binding. Zod includes the patched collection validator.
These inherited advisory dispositions apply only to the packaged relayer paths:

| Dependency/advisories                                                                                           | Operator use                                                                                    |
| --------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `bigint-buffer`: GHSA-3gc7-fjrx-p6mg                                                                            | Uses the JavaScript fallback; the vulnerable native binding is not shipped.                     |
| `toml`: GHSA-82x6-q7mm-w9cf, GHSA-v5mp-jgw5-2x6j, GHSA-4qx8-w7c3-pj3p, GHSA-9j65-hcg3-gc64, GHSA-9qmh-m7c6-f92p | Anchor's TOML workspace loader is not used; services construct programs from bundled JSON IDLs. |
| `stream-json`: GHSA-528h-pc64-c93x, GHSA-hqr4-qq8f-hg3x, GHSA-mjw6-4jj6-33hc                                    | Uses web3's HTTP Jayson client, not its streaming server, filters or assembler.                 |
| `uuid`: GHSA-w5hq-g745-h8pq                                                                                     | HTTP request IDs use `v4()`; affected caller-supplied v3/v5/v6 buffers are not used.            |

These dependencies are not claimed to be fixed. The workflow records current
advisories and fails on an ID absent from this review. Reassess the dispositions
when dependencies or service call paths change. The Go reporter ships no npm
dependencies.
