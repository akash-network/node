# Operator configuration

Use the config example shipped with the selected operator version. Unknown JSON
fields fail validation. Addresses select a compatible deployment, not a different
contract interface. Changing the protocol or supported interface requires a new
software release; changing RPC URLs, keys or deployment addresses does not.

## Compose environment

`.env` supplies Compose substitutions. These are not private-key inputs and are
not automatically forwarded into the service. The JSON file owns chain settings.

| Variable           | Required/default     | Meaning                                                                                             |
| ------------------ | -------------------- | --------------------------------------------------------------------------------------------------- |
| `OPERATOR_IMAGE`   | Required             | Published role-specific version or digest; no `latest` default                                      |
| `OPERATOR_NETWORK` | `mainnet`            | `mainnet` requires Akash mainnet and Solana mainnet-beta; `testnet` rejects both mainnet identities |
| `OPERATOR_UID`     | `10001`              | Container user; must own the key and relayer state                                                  |
| `OPERATOR_GID`     | `10001`              | Container group                                                                                     |
| `OPERATOR_CONFIG`  | Required             | Absolute host path to the existing JSON config; mounted read-only                                   |
| `OPERATOR_KEY`     | Required             | Absolute host path to the existing `0600` key; mounted read-only at `/run/keys/key`                 |
| `OPERATOR_STATE`   | Required for relayer | Existing writable host directory mounted at `/data`; retain across upgrades                         |

Compose fails for missing mount sources. RPC authentication should use a local
authenticated proxy; do not embed passwords in URLs. The images expose no ports.
Logs go to stdout/stderr, with Compose rotation at three 10 MB files.

## Shared JSON fields

| Field                | Requirement                                                                             |
| -------------------- | --------------------------------------------------------------------------------------- |
| `akashRpc`           | HTTP(S) CometBFT endpoint with proofs and historical state                              |
| `akashChainId`       | Exact expected chain identity                                                           |
| `solanaRpc`          | HTTP(S) endpoint with finalized history; reporters use independent replaying nodes      |
| `solanaGenesisHash`  | Exact expected cluster genesis hash                                                     |
| `programId`          | Deployed AKT program ID                                                                 |
| `lightClientProgram` | Optional pin; omitted means follow the AKT bridge's current client                      |
| `maxAgeSeconds`      | Maximum age of Akash state; relayer default 60; reporter requires a value (example: 60) |
| `pollSeconds`        | Poll interval; relayer default 3; reporter requires a value (example: 15)               |

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

## Reporter JSON fields

| Field            | Requirement/default                                                                       |
| ---------------- | ----------------------------------------------------------------------------------------- |
| `mint`           | Verified mint PDA belonging to `programId`                                                |
| `reporter`       | Akash address admitted by governance; must match the signing key                          |
| `keyFile`        | Owner-only raw 32-byte lowercase hex key; `/run/keys/key` in Compose                      |
| `externalSigner` | Optional alternative to `keyFile`; existing external signer integrations remain supported |
| `maxSlotLead`    | Optional freshness bound in slots; derived from `maxAgeSeconds` when omitted              |
| `auditPerScan`   | Settled records audited per scan; default 100                                             |
| `feeAmountUakt`  | Required fee amount as a decimal string; example `12500`                                  |
| `gasLimit`       | Required gas limit as a decimal string; example `500000`                                  |

The reporter needs no persistent state directory: it queries canonical chain
state on each scan. Preview never loads a signing key. Broadcast requires exactly
one signer. `externalSigner` accepts an absolute `command`, optional string-array
`args`, optional `timeoutSeconds` (default 10), and optional `publicKeyBase64`.
See [reporter policy and custody](reporter.md) for the signer protocol and alerts.
