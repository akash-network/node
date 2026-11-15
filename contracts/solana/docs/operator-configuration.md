# Operator configuration

The relayer reads a JSON file. The reporter, `akt-reporter`, reads a TOML file,
environment variables and flags. Use the configuration example shipped with the
selected version. Unknown keys fail validation. Addresses select a compatible
deployment, not a different contract interface. Changing the protocol or
supported interface requires a new software release; changing RPC URLs, keys or
deployment addresses does not.

## Compose environment

`.env` supplies Compose substitutions. These are not private-key inputs and are
not automatically forwarded into the service.

| Variable           | Required/default   | Meaning                                                                                             |
| ------------------ | ------------------ | --------------------------------------------------------------------------------------------------- |
| `OPERATOR_IMAGE`   | Required           | Published role-specific version or digest; no `latest` default                                      |
| `OPERATOR_UID`     | `10001`            | Container user; must own the key and relayer state, and be able to read the reporter home           |
| `OPERATOR_GID`     | `10001`            | Container group                                                                                     |
| `OPERATOR_HOME`    | Required, reporter | Existing host directory mounted read-only at `/home/akt-reporter`                                   |
| `OPERATOR_NETWORK` | `mainnet`, relayer | `mainnet` requires Akash mainnet and Solana mainnet-beta; `testnet` rejects both mainnet identities |
| `OPERATOR_CONFIG`  | Required, relayer  | Absolute host path to the existing JSON config; mounted read-only                                   |
| `OPERATOR_KEY`     | Required, relayer  | Absolute host path to the existing `0600` key; mounted read-only at `/run/keys/key`                 |
| `OPERATOR_STATE`   | Required, relayer  | Existing writable host directory mounted at `/data`; retain across upgrades                         |

The reporter's own settings are not Compose variables. They live in
`config/reporter.toml` in `OPERATOR_HOME`, and in an optional `reporter.env`
next to the Compose file, whose `AKT_REPORTER_*` variables reach the container.
Compose fails for missing mount sources. RPC authentication should use a local
authenticated proxy; do not embed passwords in URLs. The images expose no ports.
Logs go to stdout/stderr, with Compose rotation at three 10 MB files.

## Reporter configuration

The reporter needs no persistent state directory: it queries canonical chain
state on each pass. See [reporter policy and custody](reporter.md) for key
custody, broadcasting and alerts, and the [migration guide](reporter-migration.md)
for the mapping from the TypeScript reporter's JSON fields.

### Files, flags and environment

- **Home.** `--home`, also `AKT_REPORTER_HOME`, defaults to `$HOME/.akt-reporter`
  (`/home/akt-reporter` in the image). `<home>/config/reporter.toml` is read when
  present. `akt-reporter init` writes it, rendered from the effective
  configuration; [`reporter.example.toml`](../../../_run/akt-reporter/reporter.example.toml)
  is its output with the defaults.
- **Flags.** Every key below is a flag of the same name, for example
  `--solana.rpc`. The logging flags `--log_level`, `--log_format plain|json`,
  `--log_color` and `--log_timestamp` match `akash`. Logs go to stderr; stdout
  carries only events.
- **Environment.** Add the prefix `AKT_REPORTER_` and replace `.` and `-` with
  `_`: `solana.rpc` is `AKT_REPORTER_SOLANA_RPC`. An empty value is ignored.
- **Precedence:** flag, then environment, then file, then default.
- **Unknown keys** in the file are errors. Unknown `AKT_REPORTER_*` variables are
  logged as warnings and ignored, because platforms add their own. Kubernetes, for
  example, injects `AKT_REPORTER_SERVICE_HOST` and similar variables when a
  Service is named `akt-reporter`.
- Only `init` and `keys` write files.

### Keys

| Key                                                 | Default                     | Rules                                                                                                   |
| --------------------------------------------------- | --------------------------- | ------------------------------------------------------------------------------------------------------- |
| `network`                                           | `mainnet`                   | `mainnet` or `testnet` (see [validation](#validation))                                                  |
| `broadcast`                                         | `false`                     | `false` means preview                                                                                   |
| `reporter`                                          | required                    | Canonical lowercase `akash1…` address of 20 bytes                                                       |
| `chain-id`                                          | required                    |                                                                                                         |
| `node`                                              | `tcp://localhost:26657`     | `tcp`, `http` or `https`; no credentials in the URL                                                     |
| `grpc-addr`                                         | `localhost:9090`            | `host:port`                                                                                             |
| `grpc-insecure`                                     | `false`                     | Plaintext gRPC for a non-loopback address; loopback addresses always use plaintext                      |
| `max-age`                                           | `60s`                       | Above 0, at most `10m`; when broadcasting, `max-age` plus the signer timeout at most 80 s               |
| `request-timeout`                                   | `15s`                       | `1s` to `60s`                                                                                           |
| `from`                                              | empty                       | Keyring key name or address                                                                             |
| `keyring-backend`                                   | `test`                      | `os`, `file` or `test`; also used by `keys`                                                             |
| `keyring-dir`                                       | the home directory          |                                                                                                         |
| `gas`                                               | `auto`                      | `auto`, or gas per message; the transaction limit is that times the message count, at most `tx.max-gas` |
| `gas-adjustment`                                    | `2.0`                       | `1.0` to `10.0`                                                                                         |
| `gas-prices`                                        | `0.025uakt`                 | `uakt` only                                                                                             |
| `fees`                                              | empty                       | `uakt` only, at most `tx.max-fee`; overrides `gas-prices`                                               |
| `solana.rpc`                                        | `http://localhost:8899`     | `http` or `https`; no credentials in the URL                                                            |
| `solana.genesis-hash`                               | required                    | Canonical base58 of 32 bytes                                                                            |
| `solana.program-id`                                 | required                    | Canonical base58                                                                                        |
| `solana.mint`                                       | required                    | Must equal `PDA("mint")` of the program                                                                 |
| `solana.light-client-program`                       | empty, meaning follow       | Canonical base58                                                                                        |
| `solana.max-slot-lead`                              | `0`, meaning derive         | At most 100,000. Derived as `(max-age + 60 s) × 5` slots                                                |
| `scan.interval`                                     | `15s`                       | `1s` to `60s`                                                                                           |
| `scan.audit-per-scan`                               | `100`                       | 1 to 10,000                                                                                             |
| `scan.page-size`                                    | `500`                       | 1 to 5,000                                                                                              |
| `scan.workers`                                      | `16`                        | 1 to 256                                                                                                |
| `scan.akash-concurrency`                            | `32`                        | 1 to 256                                                                                                |
| `scan.solana-concurrency`                           | `8`                         | 1 to 64                                                                                                 |
| `scan.block-concurrency`                            | `2`                         | 1 to 16                                                                                                 |
| `tx.max-gas`                                        | `20000000`                  | 1 to 20,000,000; a lower positive block max gas of the chain caps it further                            |
| `tx.max-fee`                                        | `1000000uakt`               | `uakt`, above 0, at most `1000000uakt`                                                                  |
| `tx.max-msgs`                                       | `25`                        | 1 to 100                                                                                                |
| `tx.max-in-flight`                                  | `8`                         | 1 to 64                                                                                                 |
| `signer.remote.address`                             | empty                       | `host:port`                                                                                             |
| `signer.remote.key-id`                              | empty                       | Required with `address`                                                                                 |
| `signer.remote.ca-file`                             | empty, meaning system roots | Path                                                                                                    |
| `signer.remote.server-name`                         | empty                       |                                                                                                         |
| `signer.remote.cert-file`, `signer.remote.key-file` | empty                       | Both or neither                                                                                         |
| `signer.remote.timeout`                             | `10s`                       | `1s` to `120s`                                                                                          |
| `metrics.listen`                                    | empty, meaning disabled     | `host:port`                                                                                             |

Durations use Go syntax, such as `60s` or `1m`. `max-age` is the freshness bound
for both nodes. `scan.interval` is the wait between passes of `start`.
`tx.max-msgs` also sizes batches for a remote signer that limits its sign bytes
(see [the 4096-byte limit](reporter.md#cosmos-kms-remote-signer)).

### Validation

`check`, `start` and `scan` apply every rule above, plus:

- `solana.mint` must equal `PDA("mint")` derived from `solana.program-id`.
- In broadcast mode, exactly one of `from` and `signer.remote.address` must be set.
- The network guard. **`mainnet`** requires `chain-id` `akashnet-2` and the Solana
  genesis hash `5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d`, and refuses the
  development program ID `FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5`.
  **`testnet`** refuses both mainnet identities.

`check` validates offline and prints
`Configuration valid. No RPC request or transaction sent.` `start` and `scan` then
connect to both Akash endpoints and verify that they reach one node.
`init` writes the file even when it breaks a rule, because a fresh file has its
required keys empty, and prints every rule the file does not yet satisfy.

## Shared relayer JSON fields

| Field                | Requirement                                                        |
| -------------------- | ------------------------------------------------------------------ |
| `akashRpc`           | HTTP(S) CometBFT endpoint with proofs and historical state         |
| `akashChainId`       | Exact expected chain identity                                      |
| `solanaRpc`          | HTTP(S) endpoint with finalized history                            |
| `solanaGenesisHash`  | Exact expected cluster genesis hash                                |
| `programId`          | Deployed AKT program ID                                            |
| `lightClientProgram` | Optional pin; omitted means follow the AKT bridge's current client |
| `maxAgeSeconds`      | Maximum age of Akash state; relayer default 60                     |
| `pollSeconds`        | Poll interval; relayer default 3                                   |

## Relayer JSON fields

| Field                    | Requirement/default                                                         |
| ------------------------ | --------------------------------------------------------------------------- |
| `payerKeyFile`           | Absolute path to owner-only Solana JSON keypair; `/run/keys/key` in Compose |
| `stateDir`               | Absolute durable directory outside the release/cache; `/data` in Compose    |
| `headerEncoder`          | Absolute bundled encoder path; `/opt/akt/bin/akt-header-encoder` in Compose |
| `maxLamportsPerTransfer` | Spend budget including retries; default 5,000,000                           |
| `maxHeaderSignatures`    | Maximum signatures submitted per header; default 100                        |
| `refreshFraction`        | Refresh point within the trusting period; default 0.2                       |
| `retryMaxSeconds`        | Maximum retry backoff; default 900                                          |
| `pruneIntervalSeconds`   | Consensus pruning interval; default 600                                     |

The relayer discovers access-manager and light-client accounts through the
configured bridge. Keep the spend ledger and header journals on durable storage.
See [relayer operations](operators.md) for budget recovery and alert handling.
