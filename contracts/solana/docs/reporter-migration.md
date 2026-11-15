# Migrate from the TypeScript reporter

`akt-reporter` replaces the TypeScript reporter (`reporter/v0.1.x`, the
`ghcr.io/akash-network/akt-reporter` image of those releases) from `reporter/v0.2.0`.
It makes the same decisions and runs the same evidence checks. It reads and writes
differently: configuration is a TOML file, environment variables and flags, and
transactions are batched and unordered. The event and alert schema is unchanged.

Preview mode never signs, so you can prepare and test the Go reporter next to the
running TypeScript one. The only dangerous step is enabling broadcast; step 5 says
when.

## Steps

1. **Enable gRPC on the Akash node** and check its other requirements. In
   `app.toml`, set `[grpc] enable = true`. Set `grpc-addr` to its address.
   - The node serves gRPC without TLS. A loopback address works as is. For a node
     on another host, set `grpc-insecure = true` on a private network, or put TLS in
     front.
   - `node` and `grpc-addr` must reach **one** node. The reporter compares their
     node IDs at startup and refuses to start if they differ.
   - Enable the transaction index (`config.toml`: `[tx_index] indexer = "kv"`).
     Without it the reporter works, but cannot confirm transactions or advance the
     refund queue between passes. See
     [node requirements](reporter.md#akash-node).

2. **Create the home and the configuration.** Run
   `akt-reporter init --home <home>`, then carry your settings over with the table
   [below](#configuration-mapping). Run `akt-reporter check --home <home>` to
   validate them offline. See the [configuration reference](operator-configuration.md#reporter-configuration)
   and the commented [`reporter.example.toml`](../../../_run/akt-reporter/reporter.example.toml).

3. **Move the signing key.**
   - **A key file:** import it into the keyring, then set `from = "reporter"`:

     ```bash
     akt-reporter keys import-hex reporter --keyring-backend test --home <home> < reporter.key
     akt-reporter keys show reporter --address --keyring-backend test --home <home>
     ```

     The address must equal `reporter`. With Docker, mount the home read-write
     and owned by UID 10001 for this command; the example is in
     [key custody](reporter.md#keyring). Delete the transfer copy of `reporter.key`
     afterwards.

   - **An external signer:** the command-based signer is removed. Run
     [Cosmos-KMS](https://github.com/cosmos/kms) with a secp256k1 key for the
     reporter account, and set `signer.remote.*`. If your custody cannot be exposed
     as a Cosmos-KMS service, put a small gRPC service in front of it that
     implements `GetKey`, `GetKeys` and `Sign` of
     [`signerservice.proto`](../../../operator/reporter/signer/kmspb/signerservice.proto):
     `GetKey` returns the 33-byte compressed public key and the scheme
     (`ECDSA_SECP256K1`), and `Sign` returns a 64-byte r‖s signature over the
     payload (the sign bytes for that scheme). Serve it over TLS, and let only the
     reporter reach it. See [key custody](reporter.md#cosmos-kms-remote-signer),
     including the limit on sign bytes.

4. **Check, then run in preview.** `akt-reporter check`, then `akt-reporter start`
   without `broadcast`. Preview never signs, so it can run next to the TypeScript
   reporter. Compare its events with the TypeScript output: the lines are the same;
   only the order across transfers differs. Install it with
   [Docker Compose](operator-releases.md#docker-compose) or the
   [systemd unit](deploy-services.md#supervise-and-activate).

   With Compose, run the Go reporter from its **own directory and its own Compose
   project**, for example `$HOME/akt-reporter-go` with
   `COMPOSE_PROJECT_NAME=akt-reporter-go` in its `.env` (or
   `docker compose -p akt-reporter-go`), and its own home such as
   `/etc/akt-reporter-go`. The TypeScript reporter's directory has the same Compose
   project and service names. Setting the Go files up there would recreate its
   container as a Go preview, stop its voting without the deliberate stop of step 5,
   and overwrite its `.env`. Leave the TypeScript directory and files alone until
   step 5 is complete.

5. **Switch over.** Stop the TypeScript reporter with its own files, from its own
   directory: `docker compose -f docker-compose.yaml -f broadcast.yaml down`, or
   `systemctl stop` for a service. Then follow this rule.

   **Stop the TypeScript reporter. On the Akash node that
   `akt-reporter` uses, wait until 21 blocks have been committed after the stop,
   then wait two more minutes. Only then enable broadcast (`broadcast = true`, or
   `AKT_REPORTER_BROADCAST=true`). Never run the TypeScript reporter and
   `akt-reporter` with the same key.**

   With Compose, apply the change with
   `docker compose up -d --force-recreate reporter`: Compose does not see a change
   to the bind-mounted `reporter.toml`, so a plain `docker compose up -d` keeps the
   preview process running. Then check that the newest `starting reporter` log line
   shows `mode=broadcast` (see [Docker Compose](operator-releases.md#docker-compose)).

   Why: the TypeScript reporter's transactions are ordered and expire 20 blocks
   after it builds them. They sit outside the Go reporter's nonce scheme, so
   nothing stops both from executing the same vote. Twenty-one blocks on the
   reporter's own node means the last of them has been included or has expired. The
   extra two minutes cover every snapshot `akt-reporter` can still accept (at most
   `max-age` old, plus 10 seconds of clock skew), so its first plans already see the
   effect of those transactions.

   After a successful migration, the TypeScript directory (its `docker-compose.yaml`,
   `broadcast.yaml`, `.env` and `config.json`) can be removed once you no longer
   need it for rollback. A fresh install with no TypeScript reporter can use any
   directory.

6. **Keep the existing alert routing.** The event schema is unchanged; only the
   order across transfers differs. The [alerts table](reporter.md#alerts) is the
   same. `akt-reporter` adds Prometheus metrics: set `metrics.listen`.

7. **Adjust fees.** The TypeScript reporter paid `feeAmountUakt` for each
   single-message transaction. With batching, use `gas = "auto"` and `gas-prices`.
   Keep the caps `tx.max-gas` and `tx.max-fee`, and see
   [simulation, gas and fees](reporter.md#simulation-gas-and-fees).

After step 5, confirm that `{id, transactionHash}` events appear when there is work
to do, and that `akt_reporter_tx_total{result="included"}` grows. Once broadcast
is on, run one active signing process. A warm standby is safe
([two processes on one key](reporter.md#two-processes-on-one-key)), but wastes
signatures and node load.

**Rolling back to the TypeScript reporter.** Stop `akt-reporter` and wait for it to
exit, which takes up to a minute. **Then wait until the reporter's own Akash node
shows a block whose time is at least 2 minutes after `akt-reporter` exited, and
then wait the TypeScript reporter's `maxAgeSeconds` plus 10 more seconds. Only then
start the TypeScript reporter in broadcast mode. Never run both on one key.**

Why: when `akt-reporter` exits, the unordered transactions it already sent stay
valid until their timeouts, up to one two-minute window after the snapshot they were
planned from. A TypeScript reporter that broadcast at once would plan from snapshots
that do not yet show them, and send ordered transactions for the same messages. The
wait is step 5 in reverse. You can start the TypeScript reporter in preview at any
time.

## Configuration mapping

| TypeScript field                | `akt-reporter` key                                   |
| ------------------------------- | ---------------------------------------------------- |
| `akashRpc`                      | `node`; new: `grpc-addr`                             |
| `akashChainId`                  | `chain-id`                                           |
| `solanaRpc`                     | `solana.rpc`                                         |
| `solanaGenesisHash`             | `solana.genesis-hash`                                |
| `programId`                     | `solana.program-id`                                  |
| `mint`                          | `solana.mint`                                        |
| `lightClientProgram`            | `solana.light-client-program`                        |
| `maxSlotLead`                   | `solana.max-slot-lead`                               |
| `maxAgeSeconds`                 | `max-age`, a duration such as `"60s"`                |
| `pollSeconds`                   | `scan.interval`, a duration such as `"15s"`          |
| `auditPerScan`                  | `scan.audit-per-scan`                                |
| `reporter`                      | `reporter`                                           |
| `keyFile`                       | `keys import-hex`, then `from`                       |
| `externalSigner`                | `signer.remote.*` (Cosmos-KMS)                       |
| `externalSigner.timeoutSeconds` | `signer.remote.timeout`, a duration                  |
| `feeAmountUakt`                 | `fees`, for example `"12500uakt"`; see step 7        |
| `gasLimit`                      | `gas`, per message; or `auto`. Bound by `tx.max-gas` |

The rest are new: `grpc-addr`, `grpc-insecure`, `request-timeout`, the `keyring-*`
keys, `gas-adjustment`, `gas-prices`, `scan.page-size`, the `scan.*` concurrency
keys, `tx.*` and `metrics.listen`. The defaults suit most deployments.

## Command-line flags

| TypeScript flag         | Replacement                    |
| ----------------------- | ------------------------------ |
| `--watch`               | the `start` command            |
| `--broadcast`           | `broadcast = true`             |
| `--network testnet`     | `network = "testnet"`          |
| `--key-file PATH`       | `keys import-hex` and `from`   |
| `--signer-command PATH` | `signer.remote.*` (Cosmos-KMS) |

The TypeScript `run` command without `--watch` corresponds to `scan`, and
`check` to `check`. `scan` exits 1 if anything failed.

## What else changes

- **Deployment.** The image is multi-arch (amd64 and arm64) and its default
  command is `start`. Compose mounts the home read-only at `/home/akt-reporter`,
  reads an optional `reporter.env` and allows 60 seconds to stop. The separate
  `broadcast.yaml` override is gone; broadcast is a setting.
- **Transactions.** Batched and unordered, with timeouts that come from the
  messages rather than the clock. Expect `timeout_timestamp` values in the last
  seconds of two-minute windows. See [broadcasting](reporter.md#broadcasting).
- **Delay.** A message whose slot is taken in a window waits for the next window,
  about two minutes at most. That is negligible against the challenge period, which
  is at least 72 hours on production networks.
- **Errors.** Some assertions have new texts, and the broadcaster adds
  `sink transaction rejected`, `expired`, `unconfirmed` and `sink message not sent`.
  If you route on error texts, add the new ones.
