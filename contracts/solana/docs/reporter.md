# Reporter

`akt-reporter` implements finalized Solana observation, Akash settlement decisions and an opt-in Cosmos signer. It is a Go binary built from the node module without CGO, and it previews actions by default. The [complete local stack](local-stack.md) creates development identities and runs three signing processes. This runner supports single-key secp256k1 reporter accounts, whose native addresses are 20 bytes. Governance can select independent operators with their own nodes and keys. This signing restriction does not limit deposit owners, which may have native addresses of 1–255 bytes.

Moving from the TypeScript reporter? Follow the [migration guide](reporter-migration.md) before enabling broadcast. Native configuration migration is optional; existing JSON uses [legacy JSON compatibility](reporter-migration.md#legacy-json-compatibility).

- [Run an independent reporter](#run-an-independent-reporter)
- [Key custody](#key-custody)
- [Light client](#light-client)
- [Scanning](#scanning)
- [Broadcasting](#broadcasting)
- [Output](#output)
- [Alerts](#alerts)
- [Operating limits](#operating-limits)
- [Tests and schema](#tests-and-schema)
- [Development](#development)

## Run an independent reporter

Each reporter needs its own Akash node, its own Solana node and its own key. The code cannot establish who operates an RPC endpoint, and finalized commitment does not authenticate a dishonest RPC response. Reporters must not share nodes.

### Akash node

Run the paired Akash build with the matching `x/sink` interface. The node must report the configured chain ID, must not be catching up, and its latest block must be within `max-age`. The reporter reads recent committed heights, so a pruned node works. A pruned requested height fails the affected scan. The reporter queries canonical transfer-pending, void-requested and refund-pending buckets, every dispute and a rolling chronological audit. An audit failure retains its cursor and allows open-transfer reviews to continue.

For native TOML configuration, the node must provide:

- **gRPC.** Enable it in `app.toml` with `[grpc] enable = true`, and set `grpc-addr`. Every Akash state read goes through gRPC, pinned to one height. The node serves gRPC without TLS. Loopback addresses always use plaintext. Other addresses use TLS unless `grpc-insecure = true`, which suits a private network; otherwise put TLS in front.
- **Both endpoints on one node.** `node` (CometBFT RPC) and `grpc-addr` must reach the same node, because height H must exist on both. At startup the reporter compares the node ID of each and refuses to start if they differ. Do not point them at a load balancer that spreads requests across nodes.
- **A transaction index** for confirmations. In `config.toml`, set `[tx_index] indexer = "kv"`. The reporter reads `node_info.other.tx_index` from `/status`.

Without a transaction index, broadcast mode still works, and safety does not depend on the index (see [Broadcasting](#broadcasting)). What changes:

- The reporter logs one warning at startup, because transaction outcomes cannot be observed.
- A transaction the node accepts leaves the in-flight count at once, and keeps its slot until chain time passes its timeout. It gets no outcome event and does not count as a failure. `scan` does not wait for it.
- Refund progression stops. Without an index no lookup confirms a refund, so refund-head progression does not run, and only passes advance the refund head.

CheckTx rejections, including a taken slot, are still reported or deferred as usual.

### Solana node

Run a replaying node with `--enable-rpc-transaction-history`, not a public RPC provider. The reporter reads account state at finalized commitment and, for a first observation, the finalized block at the resolution slot. Keep blocks for at least the longest reporter outage you plan to survive plus the Akash challenge period. Once this reporter's own vote is stored on Akash, it reuses that evidence and needs no older blocks for that transfer. The terminal audit reads only account state unless it finds a disagreement, which then needs the resolution block. Increasing retention cannot recover deleted history.

### Configuration

For native TOML configuration, create the home and its configuration file with `init`. The file is `<home>/config/reporter.toml`, and the home defaults to `$HOME/.akt-reporter` (`--home`, or `AKT_REPORTER_HOME`):

```bash
akt-reporter init --home ~/.akt-reporter \
  --chain-id akashnet-2 --reporter akash1... \
  --solana.genesis-hash 5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d \
  --solana.program-id <program> --solana.mint <mint>
```

`init` renders the effective configuration: defaults plus any flags and environment variables. It refuses to overwrite a file without `--overwrite`. With `--overwrite`, it renames a synced temporary file over the target, so a failed write keeps the old file. It keeps the existing file's mode, owner and group where it may, and a new file is 0600; without the privilege to keep the owner, the file belongs to the user who runs `init`. Through a symlink, it replaces the link's target, and it refuses a target that is not a regular file. Where no temporary file can be created or renamed into place (a single-file bind mount), it writes in place, and there a failed write can leave a partial file. Through a dangling symlink, it creates the link's target in place. It writes the file even when it breaks a rule, because a fresh file has its required keys empty, and it prints every rule the file does not yet satisfy. [`reporter.example.toml`](../../../_run/akt-reporter/reporter.example.toml) is the output of `init` on an empty home, and every release archive contains it.

Every key is also a flag, such as `--solana.rpc`, and an environment variable: add the prefix `AKT_REPORTER_` and replace `.` and `-` with `_`, as in `AKT_REPORTER_SOLANA_RPC`. Precedence is flag, environment, file, default. Unknown keys in the file are errors. Unknown `AKT_REPORTER_*` variables are logged as warnings and ignored. The [configuration reference](operator-configuration.md#reporter-configuration) lists every key, its default and its rules. The ones you must set:

| Key                   | Meaning                                                                                                                                                            |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `reporter`            | Your reporter account: a lowercase 20-byte `akash` address                                                                                                         |
| `chain-id`            | Akash chain ID; also checked against the bridge account                                                                                                            |
| `node`, `grpc-addr`   | Your Akash node's CometBFT RPC and gRPC addresses, on one node                                                                                                     |
| `solana.rpc`          | Your Solana node. Keep credentials out of the URL and use a local authenticated proxy: `user:password@` is refused, and a key in the path or query is not detected |
| `solana.genesis-hash` | From your own node: `solana --url <node> genesis-hash`. A validator reset changes it                                                                               |
| `solana.program-id`   | The compatible AKT program                                                                                                                                         |
| `solana.mint`         | The program's canonical mint PDA. Set it in the configuration; no IDL rebuild is needed                                                                            |

The chain compares `reporter` with the listed members as exact strings, so governance must list it in that lowercase form; x/sink also rejects a module account as a reporter. `solana.light-client-program` is an optional pin; leave it empty to follow the bridge account (see [light client](#light-client)). `max-age` is the freshness bound for both nodes, at most `10m`. `solana.max-slot-lead` is optional, and defaults to `(max-age + 60 s) × 5` slots, with the sum rounded up to whole seconds (see [scanning](#scanning)).

For production installation, supervision and rollback use [service deployment](deploy-services.md). [Independent downloads and Docker Compose](operator-releases.md) include the binary and the image. The commands below also support explicitly configured development networks.

| Command   | Behavior                                                                                                                                      |
| --------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `init`    | Writes the configuration file                                                                                                                 |
| `check`   | Validates the configuration offline, including the network guard and signer selection. It makes no RPC request and opens no keyring or signer |
| `scan`    | Runs one pass, then waits for its transactions and follow-up reviews. Exits 1 if anything failed                                              |
| `start`   | Runs a pass, waits `scan.interval`, and repeats. This is the image's default command                                                          |
| `keys`    | The cosmos-sdk keyring commands: `add`, `import`, `import-hex`, `export`, `list`, `show`, `delete` and the rest                               |
| `version` | Prints the service name, version, commit, platform and Go version                                                                             |

`start` and `scan` first run the `check` validation. They then connect to both Akash endpoints and run the node-ID check, plus the transaction-index check in broadcast mode. In broadcast mode they load the signer, check its address and read the account number. Finally they bind the metrics listener, if configured; a bind failure ends the command before any pass.

Preview one pass without loading a key, then run continuously once the output looks right:

```bash
akt-reporter check --home ~/.akt-reporter
akt-reporter scan --home ~/.akt-reporter
akt-reporter start --home ~/.akt-reporter
```

Broadcast needs `broadcast = true`, or `AKT_REPORTER_BROADCAST=true`, and exactly one signer. Enable it only as the [migration guide](reporter-migration.md) describes.

Until governance lists `reporter`, every pass stops with `not a current reporter` after reading both chains, so preview confirms only configuration and connectivity.

The reporter pays each Akash transaction's fee and needs no Solana key. Its Akash account must exist, which it does once it has received funds; otherwise broadcast mode refuses to start with `reporter account not found`. The chain rechecks authorization and eligibility on execution; a rejected or expired transaction is reported and the next pass rereads chain state.

## Key custody

Native TOML broadcast requires exactly one signer: the keyring (`from`) or the remote signer (`signer.remote.address`). Preview never opens the keyring or connects to the signer, even when either is configured. Never use a validator consensus key or commit a key.

### Keyring

The key must be secp256k1 and must derive `reporter`; otherwise startup fails with `signer does not match reporter`. Set `from` to the key name or address. `keyring-backend` defaults to `test`, and `keyring-dir` to the home directory. `keys` reads these settings from the same configuration as `start` (flags, environment, file), so both use one keyring. The cosmos-sdk default of `os` does not apply.

| Backend | Use                                                                                                                                                                                            |
| ------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `test`  | Files in the home, mode 0600, owned by the service user (UID 10001 in the image). The key is plaintext at rest, like the TypeScript key file. This is the container default                    |
| `file`  | Encrypted at rest. The passphrase is read from stdin once at startup, so this backend suits interactive runs or systemd with `StandardInput=file:`. Containers use `test` or the remote signer |
| `os`    | The Linux secret service. It needs a D-Bus session, which containers and system services usually lack, and it is unavailable in darwin builds, which have no CGO                               |

`akt-reporter keys` exposes the cosmos-sdk key commands. `import-hex` without its hex argument reads the key from stdin, which keeps it out of shell history. In the image, the home is read-only and not writable by the container user unless you mount it. Mount it read-write, owned by UID 10001, for commands that write:

```bash
sudo install -d -o 10001 -g 10001 -m 0700 /etc/akt-reporter
docker run --rm -i -v /etc/akt-reporter:/home/akt-reporter \
  ghcr.io/akash-network/akt-reporter:0.2.0 keys import-hex reporter < reporter.key
docker run --rm -v /etc/akt-reporter:/home/akt-reporter \
  ghcr.io/akash-network/akt-reporter:0.2.0 keys show reporter --address
```

Compare the printed address with `reporter`. The file for `import-hex` holds exactly 64 hex characters, as the TypeScript key file did. See [existing wallets](operator-releases.md#existing-wallets) to export a key from an Akash CLI keyring.

### Cosmos-KMS remote signer

[Cosmos-KMS](https://github.com/cosmos/kms) can serve a gRPC `SignerService` for custody in a KMS, HSM or other service. The reporter uses its `GetKey` and `Sign` calls:

```toml
[signer.remote]
address = "kms.internal:9443"
key-id = "reporter"
ca-file = "/etc/akt-reporter/kms-ca.pem"
```

- **Startup.** `GetKey(key-id)` must return scheme `ECDSA_SECP256K1` or `ECDSA_SECP256K1ETH`, and a 33-byte compressed key on the curve that derives `reporter`. Otherwise the reporter refuses to start.
- **Each transaction.** The payload is the sign bytes for `ECDSA_SECP256K1`, or their SHA-256 for `ECDSA_SECP256K1ETH`. The response may be 64 bytes (r‖s) or 65 bytes (r‖s‖v), and `v` is dropped. The reporter converts the signature to low S and verifies it over SHA-256 of the sign bytes with the key from startup. On any failure nothing is broadcast, and each message gets `{id, error}`.
- **Transport.** TLS only; there is no plaintext option. `signer.remote.ca-file` sets the trusted CA, and system roots are used when it is empty. `signer.remote.server-name` overrides the verified server name. `signer.remote.cert-file` and `signer.remote.key-file` supply a client certificate for an mTLS proxy in front of the KMS. `signer.remote.timeout` applies per call.
- **Access.** The `SignerService` authenticates no caller. Restrict network access so that only the reporter can reach it.

**The 4096-byte limit.** With `ECDSA_SECP256K1` keys, Cosmos-KMS signs through AWS KMS raw messages, which are limited to 4096 bytes. The reporter therefore sizes batches to fit: a batch whose sign bytes exceed the limit is halved and its second half deferred, and a single message over the limit gets `{id, error}`. Operators on such keys can lower `tx.max-msgs` to about 16 to avoid halving. That is guidance, not a guaranteed number, because the size of the sign bytes depends on the transfers in a batch. `ECDSA_SECP256K1ETH` sends a 32-byte digest and has no limit.

The reporter signs only `MsgReportOutcome`, `MsgVetoVoid` and `MsgFinalizeRefund` from the reporter account. A signer service can decode the sign bytes and refuse anything else, another chain ID or another account.

The external-command signer remains available through [legacy JSON compatibility](reporter-migration.md#legacy-json-compatibility). To migrate that custody to native TOML configuration, run a small service in front of it that implements `GetKey`, `GetKeys` and `Sign` of [`signerservice.proto`](../../../operator/reporter/signer/kmspb/signerservice.proto), or use the keyring.

## Light client

Governance can re-pin the bridge to another light client with `repin_light_client`. The reporter reads the light-client program from the finalized bridge account on every observation.

- **Empty `solana.light-client-program` (follow).** The reporter observes under whichever client the bridge names. When it changes, it prints `{"alert":"light-client-changed","previous":...,"current":...,"slot":...}` before any decision made under the new client, and keeps running. Each pass summary includes the current `lightClientProgram`. The follow only moves to higher Solana slots, so a re-pin and a re-pin back while one chunk is in flight (A→B→A) can leave a chunk decided under B without its own alert: a later snapshot already showed A again, and the older one naming B is ignored.
- **Set `solana.light-client-program` (pin).** A different program in the bridge account stops the pass before any decision. The reporter prints `{"alert":"light-client-mismatch","pinned":...,"current":...}` and an error every pass until the configuration or the bridge changes; that alert is the pass's only mismatch alert. Messages already planned are still sent. Follow-up reviews are skipped until the next pass, so a message deferred to one of them gets `sink message not sent: deferred (<reason>) until <time>` and counts as a failure.

Follow is the default for independent operators. Pin only if a re-pin should stop your reporter until you have reviewed the new client.

## Scanning

Each pass reads the light client, then Akash state at one committed height.

1. Query every dispute, then refund-pending, void-requested and transfer-pending transfer at the committed height. There is no population limit. Listing errors stop the pass.
2. Reread each complete transfer before planning a message. `State.refund_head` identifies the canonical refund queue head.
3. Audit the next chronological page, skipping entries already reviewed. An audit query failure raises `audit-unavailable` and retains the cursor; open-transfer reviews still run. A terminal transfer with no hold or contradictory report can use finalized account state. Otherwise fetch complete transaction evidence.

The audit cursor is intentionally round-robin and restarts after the last page. Durable relayer discovery has a separate checkpoint that never resets at an empty `next_key`.

Reviews run in chunks of up to 96 transfers, `scan.workers` chunks at a time. Each chunk shares one Akash height and one finalized Solana snapshot. Each Akash snapshot is cached per height, and Solana block times and block evidence per slot.

Each observation checks the Solana genesis hash and reads program, launch configuration, bridge configuration, mint and transfer status together at finalized commitment. It verifies account owners, the canonical mint PDA, mint authority, decimals, absent freeze authority, the Akash chain ID and the light client as above. Status must match all three components of the immutable Akash transfer ID. A mint must also match the amount and recipient, and its proof height must follow the deposit height.

For a first observation, the observer reads the finalized block at the slot recorded in the permanent status and searches all of its transactions for the successful resolution. Any transaction can reference the status account, so its signature history is not used. It requires a writable reference to that status account and the matching `TransferResolved` event, including proof height, resolved slot, and typed outcome emitted while the AKT program is executing, at any invocation depth. Anchor's event parser ignores events inside a CPI, so the observer attributes each `Program data:` line to the frame on the invoke stack it rebuilds from the logs. Unbalanced logs supply no evidence. If a later instruction exhausts the log limit, only a top-level AKT invocation that closed with `success` before `Log truncated` can supply it; the transaction must still succeed. Failed retries, unrelated transactions and other programs' events cannot supply evidence. More than one match in the block stops a new report. A skipped, missing or pruned block also stops it. Blocks and transactions are decoded from JSON, which includes Solana v1 transactions.

After restart, the reporter may reuse evidence from its own authenticated vote stored on Akash. It still rereads finalized Solana state and checks the full status, outcome and resolution slot. A different outcome or slot requires transaction discovery again. Other reporters' votes never substitute for its own observation.

- A finalized mint or void produces `MsgReportOutcome` with the current reporter policy version and transfer round. An identical stored vote suppresses another submission. A frozen quorum or matching governance observation needs no same-outcome vote, including from a reporter outside its certificate.
- The first contradictory finalized outcome produces a report to trigger the native conflict hold, including after a false terminal confirmation. An existing conflict produces alerts without repeated paid reports. A fabricated future slot in another report cannot hide an already finalized permanent outcome.
- A missing status never produces a void report or a refund. A governance order without outcome evidence is expected to have no status and causes no false-absence alert. It can produce `MsgVetoVoid` during an active void challenge only when the finalized snapshot is at least as recent as all plausible reported evidence. A lagging node cannot establish absence for evidence it has not reached.
- A reported slot above this node's fresh finalized slot plus `solana.max-slot-lead` cannot have been finalized. It is excluded from that bound and raises an alert, so reporters at threshold cannot block an honest veto with an invented future slot. A fabricated slot just inside the lead delays a veto until the node reaches it, about `max-slot-lead / 2.5` seconds; keep that well inside the challenge window.
- When the status is missing, the reporter also looks up the signature of each vote or governance observation at or below the snapshot slot, at finalized commitment. A missing transaction, another slot, or one that does not resolve that ID with that outcome is false evidence. A transaction that does show it contradicts the missing status, so the observation stops without a veto.
- A directly observed void can produce `MsgFinalizeRefund` only with current-policy quorum or retained governance authorization, an elapsed challenge, no hold or contradictory evidence, refunds enabled, the native refund queue head and remaining epoch capacity. A needed fresh vote is submitted before considering finalization on a later pass.

Akash queries for one decision use the same committed height. The client reads both CometBFT status and the application's last committed height because consensus can advertise a height before application queries can serve it. Freshness uses the selected block's timestamp. Membership, rounds, challenges and accounting remain enforced by `x/sink`. Restart recovery comes from native votes and terminal states, with no local delivery cursor that could lose a veto or repeat a refund.

## Broadcasting

In broadcast mode the reporter sends the messages its reviews plan as batched, unordered Akash transactions and confirms them asynchronously. Reviews never wait for broadcasting, so a slow or unavailable signer delays submissions, not passes. Nothing needs to be persisted: after a restart, a new process plans the same messages again.

### Unordered transactions and nonce slots

Transactions are unordered: sequence 0, with a `timeout_timestamp` instead of a sequence. The chain refuses a second transaction with the same signer and `timeout_timestamp`; the refused one has no effect and pays no fee. The reporter derives every timeout from the messages a transaction carries, so all transactions that could carry the same message in the same window share one replay key, and the chain executes at most one of them. This needs no stored state and never reads the host clock.

- **Windows.** A window is 2 minutes of chain time. A message belongs to the window of the Akash snapshot it was planned from.
- **Slots.** Within a window, each message maps to one of 1,152 slots by a hash of its key: 128 slots for votes, and 1,024 for vetoes and refunds. A slot's timeout falls in the last 1.152 seconds of its window, so timeouts in an explorer look unrelated to wall-clock time. A transaction carries messages of one window and one slot.
- **Message key.** The transfer ID and message kind, plus the outcome, round and policy version for a vote. A veto's key has no round, because `MsgVetoVoid` carries none.
- **Constants.** The window length and the slot counts are protocol values, not configuration. A release that changes them ships an upgrade note: stop every old process and wait four minutes before starting the new one.

**What "executes at most once" means.** Across restarts, lost responses, separate `scan` runs, lagging or load-balanced nodes, a missing transaction index and several processes sharing a key:

- a vote executes at most once per transfer, outcome, round and policy version;
- a veto executes at most once per transfer and settlement round;
- a refund executes at most once per transfer.

A governance resolution starts a new round, so a veto after it is a new decision, which the TypeScript reporter also makes. A repeated vote or refund that reached execution would still pay a fee even where it changes nothing; the slots stop it before that.

**The one exception.** The guarantee rests on the SDK's replay-key check, which has one boundary case in the Akash cosmos-sdk fork. Upstream cosmos-sdk fixed it in cosmos/cosmos-sdk#26672, and the Akash fork has not adopted that change yet. Until it does, this boundary is the one exception to the guarantee. See [spec section 14](../../../docs/superpowers/specs/2026-11-12-akt-reporter-go-design.md#14-risks-and-follow-ups).

### Batching, priority and deferral

- `MsgReportOutcome` messages of one window and slot are batched, up to `tx.max-msgs` per transaction. A vote batch is sent when it is full, when the review that filled it ends, or 10 seconds before its send cutoff.
- `MsgVetoVoid` and `MsgFinalizeRefund` get one transaction each, in their own slots, and are sent at once. One failed message fails its whole transaction, and these can fail on the challenge window, refund order or caps.
- At most `tx.max-in-flight` transactions are unconfirmed at once. When one resolves, the next send is chosen by priority: `MsgVetoVoid` first, then `MsgFinalizeRefund`, then vote batches in plan order. At the defaults, at most 200 votes are unconfirmed at once.
- A window carries at most 128 vote transactions. A 5,000-vote backlog drains in three to four windows on a fresh node.
- **Send cutoff.** A slot's transaction is signed only while its timeout is at least 20 seconds plus the signer timeout away from the latest block the reporter has seen. This keeps transactions from expiring unsent. When broadcasting, validation requires `max-age` plus the signer timeout to be at most 80 seconds, so that a message planned from the oldest accepted snapshot can still be sent.
- **Deferral.** A message is deferred to the next window when its slot already holds another transaction in this window, when its batch is full, or when its send cutoff has passed. A plan from a window earlier than the latest block seen counts as past its cutoff. A deferral is not an outcome and writes no event, and its counter is `akt_reporter_deferred_messages_total` with reasons `slot_busy`, `slot_full`, `cutoff`, `simulation` and `cap`.
- **Deferred vetoes** wait up to one window. That is negligible against the 72 hour minimum challenge period. On networks built with a shorter period (the `sinkdev` tag allows 1 second), a deferred veto can miss its challenge window.
- Before signing, the latest snapshot must still list `reporter` (otherwise `reporter was removed`), the signer's address must equal `reporter`, and the send cutoff must not have passed.

### Simulation, gas and fees

- Every batch is simulated before signing. If simulation fails at `message index: N`, message N is dropped with `{id, error}` and the rest is simulated again, for up to three rounds. A failure without a message index fails the whole batch.
- With `gas = "auto"`, the gas limit is the simulated gas times `gas-adjustment`. A number is gas per message, so the limit is that times the message count.
- With `gas = "auto"`, the simulated transaction declares a gas limit of 0, because the final limit is not known yet. Simulation meters no limit, and a declared 0 always passes the chain's block max gas check, which runs in simulation too.
- Simulation runs against the latest committed block, so the limit must also cover other reporters' votes landing first in the same block, including the vote that completes the quorum. The default `gas-adjustment` of 2.0 covers that with a 10% margin. The 2.0 default is measured to cover policies of up to 5 members voting in one block; for larger policies, re-measure with `TestReporterGasMeasure` (`SINK_GAS_MEASURE=1 go test -run TestReporterGasMeasure ./x/sink/`) and raise `gas-adjustment`.
- The fee is `fees` when set, otherwise the gas limit times `gas-prices`. The TypeScript reporter paid `feeAmountUakt` for every single-message transaction; batching makes `gas = "auto"` with `gas-prices` the better choice.
- **Caps.** The gas limit must be at most the gas cap and the fee at most `tx.max-fee`, which must be above 0. The gas cap is `tx.max-gas`, or the chain's consensus block max gas when that is positive and lower, because no block includes a transaction declaring more. Broadcast mode reads the block max gas at startup and logs `tx.max-gas`, `block-max-gas` and the effective `gas-cap`. A batch over a cap is halved and its second half deferred. A single message over a cap gets `{id, error}` and is not sent. A signer's sign-bytes limit is a cap of the same kind (see [the 4096-byte limit](#cosmos-kms-remote-signer)).

### Confirmation

The reporter broadcasts with `broadcast_tx_sync`, and re-sends the same signed bytes after a transport error until the timeout passes. While a transaction is unconfirmed it refreshes its view of the chain once a second, including between passes, and looks the transaction up on every new height and at the next refresh after a failed lookup. Outcomes are events on stdout:

| Outcome                                   | Event                                                                    |
| ----------------------------------------- | ------------------------------------------------------------------------ |
| Included with code 0                      | `{id, transactionHash}` for each message, with the hash in uppercase hex |
| Slot already taken by another transaction | Its messages are deferred; no event                                      |
| Refused at first send, any other reason   | `{id, error: "sink transaction rejected: <HASH> code <n>: <log>"}`       |
| Failed in a block                         | `{id, error: "sink transaction failed: <HASH> code <n>"}`                |
| Timeout passed, never included            | `{id, error: "sink transaction expired: <HASH>"}`                        |

Expiry is decided by chain time only. `scan` waits until no transaction is unresolved, nothing is deferred and follow-up reviews are idle, or until 3 minutes of wall-clock time pass without progress. A transaction still unresolved then, for example because the chain halted, gives `sink transaction unconfirmed: <HASH>`. A message still deferred or queued gives `sink message not sent: deferred (<reason>) until <time>` or `sink message not sent: queued until <time>`. These count as failures, so `scan` exits 1. `start` keeps them: a sent transaction stays until inclusion or chain-time expiry, and a deferred message is reviewed again in the next window.

Without a transaction index, accepted transactions get none of these events (see [Akash node](#akash-node)).

### Follow-up reviews

Between passes the broadcaster asks the scanner to review specific transfers:

- **Refund-head progression.** `x/sink` pays refunds strictly in queue order, and `MsgFinalizeRefund` succeeds only for the current refund head. When a refund finalization is included with code 0, the broadcaster takes a fresh snapshot at once and reviews the new head. If that review plans another finalization, it is sent the same way, and the steps repeat. Progression stops when the head plans no finalization (not yet eligible, held, or the epoch capacity is used) and on shutdown. Refunds are paid at about one per block; a refund whose slot is taken in the current window waits for the next one. **Without a transaction index no lookup confirms a refund, so refund-head progression does not run and only passes advance the head.**
- **Deferred transfers.** Once a block from a later window is seen, the deferred transfers are reviewed again in chunks of up to 96, each with a fresh snapshot. If a deferred message executed in the meantime, through another process's transaction or an earlier attempt, the review plans nothing.

Each follow-up review needs a snapshot at least as high as the block that triggered it. A lower snapshot from a load-balanced node is retried until the waiting limit or shutdown. Follow-up reviews run one at a time and write the normal review events. `scan` waits for them before its summary.

### Two processes on one key

Two `akt-reporter` processes with one key are safe. For every message they plan in the same window they share one replay key, so the chain executes at most one transaction and charges nothing for the refused one. Byte-identical copies, which two processes can build because signing is deterministic, are recognized by their hash. A warm standby is therefore allowed, but run one active signing process, because a second one wastes signatures and node load.

This does not extend to the TypeScript reporter, whose ordered transactions are outside the nonce scheme. Never run it and `akt-reporter` with the same key; see the [migration guide](reporter-migration.md).

### A lagging node

A node that lags makes the reporter plan from old snapshots, and the send cutoff is measured against what that node has seen. The chain may then refuse the resulting transactions as expired, which shows up as expiry events and `cutoff` deferrals. The remedy is a healthy node or a lower `max-age`.

### Shutdown

On the first SIGINT or SIGTERM no new chunk starts and no new transaction is signed. Queued and deferred messages are released without an event and planned again on the next start. In-flight transactions get up to 30 seconds to confirm, then the summary of the partial pass is written. `start` exits 0 and `scan` exits 1 if the pass had failures. A second signal exits immediately. The Compose file allows 60 seconds before it kills the container.

## Output

Events go to stdout as JSON lines: per-transfer observations, proposed messages, alerts, transaction hashes, errors and one summary per pass. Amounts and slots are decimal strings; signature bytes are base64; transaction hashes are uppercase hex. The schema is the TypeScript reporter's, so existing alert routing keeps working. Only the order across transfers differs: a transfer's review comes before its transaction events, a `light-client-changed` alert comes before every event decided under the new client, and the summary is the last event of a pass, except that in `start` transaction outcomes and follow-up reviews may follow it.

Logs go to stderr as akash-style zerolog: `--log_level`, `--log_format plain|json`, `--log_color` and `--log_timestamp`. They cover a startup summary of the effective configuration without key material, retries, per-pass cache and RPC statistics, and signer and node identity. Startup logs show RPC endpoints as `scheme://host`. Events and logs never contain key material.

When `metrics.listen` is set, `/metrics` serves Prometheus metrics. Bind it to a private interface.

| Metric                                 | Type      | Labels            |
| -------------------------------------- | --------- | ----------------- |
| `akt_reporter_pass_duration_seconds`   | histogram |                   |
| `akt_reporter_pass_transfers`          | gauge     |                   |
| `akt_reporter_reviews_total`           | counter   | `result`          |
| `akt_reporter_alerts_total`            | counter   | `alert`           |
| `akt_reporter_rpc_duration_seconds`    | histogram | `chain`, `method` |
| `akt_reporter_rpc_errors_total`        | counter   | `chain`, `method` |
| `akt_reporter_cache_requests_total`    | counter   | `cache`, `result` |
| `akt_reporter_tx_total`                | counter   | `result`          |
| `akt_reporter_tx_messages_total`       | counter   | `type`            |
| `akt_reporter_tx_fees_uakt_total`      | counter   |                   |
| `akt_reporter_tx_in_flight`            | gauge     |                   |
| `akt_reporter_deferred_messages_total` | counter   | `reason`          |

`result` is `ok` or `error` for reviews, `hit` or `miss` for caches, and `included`, `failed`, `rejected`, `slot_taken`, `expired` or `unconfirmed` for transactions.

## Alerts

Route these to paging:

| Output                                                            | Meaning                                                                |
| ----------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `light-client-mismatch`                                           | Governance re-pinned away from your pin; the reporter is stopped       |
| `light-client-changed`                                            | Governance re-pinned; review the new client                            |
| `Akash settlement contradicts finalized Solana outcome`           | Possible false report or false terminal confirmation                   |
| `Akash reports an outcome without a finalized Solana status`      | Possible false void or mint quorum                                     |
| `Akash reports a Solana slot beyond any plausible finalized slot` | A report carries a fabricated slot                                     |
| `Transfer requires governance review`                             | A veto or conflict hold is waiting for governance                      |
| `audit-unavailable`                                               | History audit failed; its cursor is retained and open reviews continue |
| Repeated `error` lines                                            | Stale or misconfigured nodes, missing history or signer failures       |

Stale nodes, wrong deployment configuration, unavailable history, signer failures and RPC errors stop reporting; they never veto or refund on their own. When evidence cannot be checked before a challenge expires, use governance or the refund pause. The runner alone does not guarantee an honest veto is delivered in time.

## Operating limits

A pass over 100,000 open transfers takes at most 60 seconds on a 4-vCPU runner in the project's benchmark, with bounded memory, because a pass keeps only compact transfer IDs. It loads the node with up to about 2,000 height-pinned gRPC queries per second while it runs. The benchmark's latencies are synthetic, so watch `akt_reporter_pass_duration_seconds` and `akt_reporter_rpc_duration_seconds` in production. The program ID check does not pin deployed bytecode against upgrades, and the admin upgrade trust assumption is unchanged. Earlier local integration exercised an authenticated header, a mint, three signed reporter votes and backing reconciliation. Those results do not validate the current schema and program pairing. Complete live refund recovery, wallet signing and independent multi-operator deployments remain live integration checks.

## Tests and schema

```bash
make test-reporter
GOWORK=off go test ./operator/... ./cmd/akt-reporter/...
REPORTER_PERF=1 GOWORK=off go test ./operator/reporter/scan/ ./operator/reporter/tx/ ./x/sink/ -run 'TestSteady100k|TestReporterNodeLoad|TestNoVoteExecutesTwice'
npm run test:reporter
npm run typecheck
npm run test:sink
npm test
```

Run the Go commands from the node repository root and the `npm` commands from `contracts/solana`. The performance tests also run nightly in CI. The TypeScript reporter source stays in the repository, because the relayer, the local stack and the tests import it.

The unit tests cover replay evidence, CPI and truncated logs, block spam, wrong owners/amounts/recipients, stale and lagging nodes, fabricated future slots, missing reported signatures, missing history, reuse of matching own-vote evidence, duplicate votes, rotation, contradictions, vetoes and refund eligibility. They also cover light-client pinning and following, open-transfer paging and its consistency checks, dispute/refund priority, populations beyond the former scan limits, audit failures, and the round-robin audit. The Go reporter runs the same scenarios as the TypeScript reporter against a simulated node and must produce structurally identical events (the parity goldens in `operator/reporter/testdata/goldens`). Its tests also cover nonce slots, batching, deferral, confirmation, the no-index fallback, refund progression, the remote signer against a fake `SignerService` with throwaway keys, and `TestNoVoteExecutesTwice`, which randomizes lower snapshots, restarts, lost responses and two processes sharing a key. A test on the real application shows the chain refusing a reused replay key without charging a fee. `npm test` reads actual finalized automatic and governance voids on a local Solana validator. The validator suite creates its own ledger and stops only its own process. Choose free `AKT_TEST_RPC_PORT` and `AKT_TEST_PORT_BASE` values if its defaults are occupied; preserve existing runtimes.

`npm run test:sink` executes an Akash deposit and exports its real committed proof and protobuf query responses. It mints through the upstream light client, observes the finalized Solana transaction, and signs three `SIGN_MODE_DIRECT` reports with deterministic test keys. The native Akash BaseApp then verifies and executes those exact transaction bytes, reaches quorum, marks the transfer minted and preserves escrow. It uses separate test processes and a trusted test checkpoint; it does not test a continuously running CometBFT network, live RPC broadcasting or authenticated header relaying.

The Akash client types of the TypeScript services come from the local `@akashnetwork/chain-sdk` package. Build and pack the selected committed SDK source using its existing pipeline, then supply that archive explicitly as described in [SDK integration](sdk-integration.md). Setup only installs the locked input. Update the package lock in `contracts/solana` after an SDK change. SDK int64 and uint64 fields use bigint; JSON boundaries use exact decimal strings.

See [canonical Sink launch and recovery](sink-migration-v2.md) for native protobuf proofs, policy history, governance messages and refund authorization.

## Development

### Regenerate the parity goldens

`make reporter-goldens` runs the TypeScript reporter in preview against the simulated node, once per scenario, and rewrites `operator/reporter/testdata/goldens/*.jsonl` and `pda_vectors.json`. CI only reads the goldens. Regenerate them when a scenario or the TypeScript reporter changes, then run `GOWORK=off go test ./operator/reporter/scan/ ./operator/reporter/solana/ -run 'Goldens|PDAVectors'`. It needs the `contracts/solana` development setup:

1. **Build `chain-sdk.tgz`** as [SDK integration](sdk-integration.md) describes, then run `npm ci --prefix contracts/solana`.
2. **Install an isolated toolchain** under `.cache/solana/toolchain`, which git ignores. The goldens need anchor 0.32.1 and Agave 2.3.13. Keep them off your global PATH and out of AVM, so that anchor never switches the global Solana version. Anchor is checked against the SHA-256 that [`scripts/install-anchor.sh`](../scripts/install-anchor.sh) pins for your platform:

   ```bash
   # from the node repository root, in bash
   R=$PWD/.cache/solana/toolchain
   target=aarch64-apple-darwin # or x86_64-apple-darwin, x86_64-unknown-linux-gnu
   digest=<the digest install-anchor.sh pins for $target>
   mkdir -p "$R/bin"
   curl -fL "https://github.com/otter-sec/anchor/releases/download/v0.32.1/anchor-0.32.1-$target" -o "$R/bin/anchor"
   echo "$digest  $R/bin/anchor" | shasum -a 256 -c
   chmod 755 "$R/bin/anchor"
   curl -fL "https://github.com/anza-xyz/agave/releases/download/v2.3.13/solana-release-$target.tar.bz2" | tar -xj -C "$R"
   export PATH="$R/bin:$R/solana-release/bin:$PATH"
   anchor --version # anchor-cli 0.32.1
   solana --version # solana-cli 2.3.13
   ```

   The Agave tarball goes first on `PATH`, so anchor finds Solana 2.3.13 there and never installs or switches a version. Check `command -v anchor` points into `.cache`.

3. **Build the IDLs** in the same shell. Source `scripts/workspace.sh` from **bash only**: it derives its paths from `BASH_SOURCE`, and zsh resolves them outside the repository. Build the AKT IDL, then the `ics07_tendermint` and `access_manager` IDLs that `npm run typecheck` needs. Only IDLs are built, not the SBF programs:

   ```bash
   cd contracts/solana
   source scripts/workspace.sh
   mkdir -p "$CARGO_TARGET_DIR/idl" "$CARGO_TARGET_DIR/types"
   anchor idl build --program-name akt --out "$CARGO_TARGET_DIR/idl/akt.json" \
     --out-ts "$CARGO_TARGET_DIR/types/akt.ts" -- --locked
   python3 scripts/fetch-light-client.py
   cd "$SOLANA_DEPS/ibc-contracts/ibc-solana"
   mkdir -p target/idl target/types
   for p in ics07_tendermint access_manager; do
     RUSTUP_TOOLCHAIN=1.89.0 anchor idl build --program-name "$p" \
       --out "target/idl/$p.json" --out-ts "target/types/$p.ts" -- --locked
   done
   ```

4. **Check and regenerate** from the repository root, in the same shell:

   ```bash
   (cd contracts/solana && npm run typecheck)
   make reporter-goldens
   git status operator/reporter/testdata
   ```

   Review the diff: a golden changes only when a scenario or the TypeScript reporter's behavior changed.

### Regenerate the Cosmos-KMS client code

`operator/reporter/signer/kmspb` holds `signerservice.proto`, copied from `github.com/cosmos/kms` at the commit named in its header, and its generated Go code. The generators are pinned and installed into an isolated `GOBIN` under the git-ignored `.cache`. `protoc` comes from your system (the committed code was generated with libprotoc 36.2). From the repository root:

```bash
GOBIN=$PWD/.cache/tools/bin GOWORK=off go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
GOBIN=$PWD/.cache/tools/bin GOWORK=off go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
PATH=$PWD/.cache/tools/bin:$PATH GOWORK=off go generate ./operator/reporter/signer/kmspb/
```

`go generate` runs the `protoc` directive in `generate.go`, which sets the Go package with `M` options, so the proto's `go_package` stays unedited. To move to a newer upstream proto, replace the file, keep the header naming the new commit, and regenerate. The remote-signer release gate then runs against a real `kms` binary.
