# Reporter

`reporter/` implements finalized Solana observation, Akash settlement decisions and an opt-in Cosmos signer. It previews actions by default. The [complete local stack](local-stack.md) creates development identities and runs three signing processes. This runner supports single-key secp256k1 reporter accounts, whose native addresses are 20 bytes. Governance can select independent operators with their own nodes and keys. This signing restriction does not limit deposit owners, which may have native addresses of 1–255 bytes.

## Run an independent reporter

Each reporter needs its own Akash node, its own Solana node and its own key. The code cannot establish who operates an RPC endpoint, and finalized commitment does not authenticate a dishonest RPC response. Reporters must not share nodes.

**Akash node.** Run the paired Akash build with the matching `x/sink` interface. The reporter queries only the latest committed height, so a pruned node works. The node must report the configured chain ID, must not be catching up, and its latest block must be within `maxAgeSeconds`. The reporter queries canonical transfer-pending, void-requested and refund-pending buckets, every dispute and a rolling chronological audit. A pruned requested height fails the affected scan. An audit failure retains its cursor and allows open-transfer reviews to continue.

**Solana node.** Run a replaying node with `--enable-rpc-transaction-history`, not a public RPC provider. The reporter reads account state at finalized commitment and, for a first observation, the finalized block at the resolution slot. Keep blocks for at least the longest reporter outage you plan to survive plus the Akash challenge period. Once this reporter's own vote is stored on Akash, it reuses that evidence and needs no older blocks for that transfer. The terminal audit reads only account state unless it finds a disagreement, which then needs the resolution block. Increasing retention cannot recover deleted history.

**Configuration.** Copy the example outside the repository and fill in your values:

```bash
mkdir -p ~/.config/akt-reporter
cp reporter/config.example.json ~/.config/akt-reporter/config.json
```

| Field                | Meaning                                                                                          |
| -------------------- | ------------------------------------------------------------------------------------------------ |
| `akashRpc`           | Your Akash node's CometBFT RPC. Credentials in URLs are refused; use a local authenticated proxy |
| `akashChainId`       | Akash chain ID; also checked against the bridge account                                          |
| `solanaRpc`          | Your Solana node                                                                                 |
| `solanaGenesisHash`  | From your own node: `solana --url <node> genesis-hash`. A validator reset changes it             |
| `programId`, `mint`  | AKT program and canonical mint PDA. Another program ID requires rebuilding the IDL               |
| `reporter`           | Your reporter account                                                                            |
| `lightClientProgram` | Optional pin. Leave unset to follow the bridge account; see [light client](#light-client)        |
| `maxAgeSeconds`      | Freshness bound for both nodes, at most 600                                                      |
| `maxSlotLead`        | Optional; default `(maxAgeSeconds + 60) * 5`. See [scanning](#scanning)                          |
| `auditPerScan`       | Optional; default 100. Transfer IDs the terminal audit rechecks per pass                         |
| `pollSeconds`        | Delay between passes in watch mode                                                               |
| `feeAmountUakt`      | Fee per Akash transaction, at most 1 AKT. The example pays 0.025 uakt per gas                    |
| `gasLimit`           | Gas per Akash transaction, at most 20,000,000                                                    |
| `externalSigner`     | Optional signer command; see [key custody](#key-custody)                                         |

Unknown configuration fields are rejected. `reporter` must be a lowercase 20-byte `akash` address. The chain compares it with the listed members as exact strings, so governance must list it in that lowercase form; x/sink also rejects a module account as a reporter.

For production installation, supervision and rollback use [service deployment](deploy-services.md). Its packaged `akt-reporter.service` starts in preview, and its broadcast entry point checks the production configuration and requires `externalSigner` in the config file. The generic commands below also support explicitly configured development networks.

Preview one pass without loading a key, then run continuously once the output looks right:

```bash
npm run reporter -- --config ~/.config/akt-reporter/config.json
npm run reporter -- --config ~/.config/akt-reporter/config.json --watch --broadcast --signer-command /usr/local/bin/akt-sign
```

Until governance lists `reporter`, every pass stops with `not a current reporter` after reading both chains, so preview confirms only configuration and connectivity.

Output is JSON lines: per-transfer observations, proposed messages, alerts, transaction hashes, errors and one summary per pass. Amounts and slots are decimal strings; signature bytes are base64. A one-shot run exits nonzero if anything failed. Watch mode logs the failure and retries on the next pass. Ctrl-C stops after the current bounded RPC or transaction operation.

The reporter pays each Akash transaction's fee and needs no Solana key. The chain rechecks authorization and eligibility on execution; a rejected or timed-out transaction is logged and the next pass rereads chain state. Run one signing process per reporter account to avoid sequence contention.

## Key custody

Broadcast requires exactly one signer. Preview never loads a key or runs a signer.

**Key file** (`--key-file PATH`), for development. The file is owner-only, regular, and contains exactly 64 lowercase hex characters. The derived address must match `reporter`. Never use a validator consensus key or commit the file.

**External signer**, for operator custody in a KMS, HSM or other service. Configure it in the config file, or pass `--signer-command PATH`, which replaces the configured command and its arguments:

```json
"externalSigner": {
  "command": "/usr/local/bin/akt-sign",
  "args": ["--key", "reporter-1"],
  "timeoutSeconds": 10,
  "publicKeyBase64": "A..."
}
```

The command must be an absolute path. It runs without a shell, inherits the reporter's environment, and its stderr passes through. `timeoutSeconds` defaults to 10, at most 120. For each transaction the reporter writes one JSON object to stdin and closes it:

```json
{
  "chainId": "akashnet-2",
  "accountNumber": "42",
  "sequence": "7",
  "address": "akash1...",
  "signDocBase64": "..."
}
```

`signDocBase64` holds the `SIGN_MODE_DIRECT` `SignDoc` bytes; numbers are decimal strings. The signer signs the SHA-256 of those bytes with secp256k1, prints one JSON object and exits 0:

```json
{
  "publicKeyBase64": "<33-byte compressed key>",
  "signatureBase64": "<64-byte r||s>"
}
```

Before broadcasting, the reporter checks that the key is a valid compressed point, derives `reporter` and equals the expected key, and that the signature verifies over the exact sign doc. A high-S signature is converted to low S, which the Cosmos SDK requires. A timeout, nonzero exit, more than 64 KiB of output or any failed check stops that transaction; nothing is broadcast. The key is part of the signed transaction, so the reporter needs it first: from `publicKeyBase64` or, when that is unset, from the account on the Akash node. An account that has never signed must set `publicKeyBase64`.

The reporter signs only `MsgReportOutcome`, `MsgVetoVoid` and `MsgFinalizeRefund` from the reporter account. A signer can decode the sign doc and refuse anything else, another chain ID or another account.

## Light client

Governance can re-pin the bridge to another light client with `repin_light_client`. The reporter reads the light-client program from the finalized bridge account on every observation.

- **Unset `lightClientProgram` (follow).** The reporter observes under whichever client the bridge names. When it changes, it prints `{"alert":"light-client-changed","previous":...,"current":...,"slot":...}` before any decision made under the new client, and keeps running. Each pass summary includes the current `lightClientProgram`.
- **Set `lightClientProgram` (pin).** A different program in the bridge account stops the pass before any decision. The reporter prints `{"alert":"light-client-mismatch","pinned":...,"current":...}` and an error every pass until the configuration or the bridge changes.

Follow is the default for independent operators. Pin only if a re-pin should stop your reporter until you have reviewed the new client.

## Scanning

Each pass reads the light client, then Akash state at one committed height.

1. Query every dispute, then refund-pending, void-requested and transfer-pending transfer at the committed height. There is no population limit. Listing errors stop the pass.
2. Reread each complete transfer before planning a message. `State.refund_head` identifies the canonical refund queue head.
3. Audit the next chronological page, skipping entries already reviewed. An audit query failure raises `audit-unavailable` and retains the cursor; open-transfer reviews still run. A terminal transfer with no hold or contradictory report can use finalized account state. Otherwise fetch complete transaction evidence.

The audit cursor is intentionally round-robin and restarts after the last page. Durable relayer discovery has a separate checkpoint that never resets at an empty `next_key`.

Each observation checks the Solana genesis hash and reads program, launch configuration, bridge configuration, mint and transfer status together at finalized commitment. It verifies account owners, the canonical mint PDA, mint authority, decimals, absent freeze authority, the Akash chain ID and the light client as above. Status must match all three components of the immutable Akash transfer ID. A mint must also match the amount and recipient, and its proof height must follow the deposit height.

For a first observation, the observer reads the finalized block at the slot recorded in the permanent status and searches all of its transactions for the successful resolution. Any transaction can reference the status account, so its signature history is not used. It requires a writable reference to that status account and the matching `TransferResolved` event, including proof height, resolved slot, and typed outcome emitted while the AKT program is executing, at any invocation depth. Anchor's event parser ignores events inside a CPI, so the observer attributes each `Program data:` line to the frame on the invoke stack it rebuilds from the logs. Unbalanced logs supply no evidence. If a later instruction exhausts the log limit, only a top-level AKT invocation that closed with `success` before `Log truncated` can supply it; the transaction must still succeed. Failed retries, unrelated transactions and other programs' events cannot supply evidence. More than one match in the block stops a new report. A skipped, missing or pruned block also stops it.

After restart, the reporter may reuse evidence from its own authenticated vote stored on Akash. It still rereads finalized Solana state and checks the full status, outcome and resolution slot. A different outcome or slot requires transaction discovery again. Other reporters' votes never substitute for its own observation.

- A finalized mint or void produces `MsgReportOutcome` with the current reporter policy version and transfer round. An identical stored vote suppresses another submission. A frozen quorum or matching governance observation needs no same-outcome vote, including from a reporter outside its certificate.
- The first contradictory finalized outcome produces a report to trigger the native conflict hold, including after a false terminal confirmation. An existing conflict produces alerts without repeated paid reports. A fabricated future slot in another report cannot hide an already finalized permanent outcome.
- A missing status never produces a void report or a refund. A governance order without outcome evidence is expected to have no status and causes no false-absence alert. It can produce `MsgVetoVoid` during an active void challenge only when the finalized snapshot is at least as recent as all plausible reported evidence. A lagging node cannot establish absence for evidence it has not reached.
- A reported slot above this node's fresh finalized slot plus `maxSlotLead` cannot have been finalized. It is excluded from that bound and raises an alert, so reporters at threshold cannot block an honest veto with an invented future slot. A fabricated slot just inside the lead delays a veto until the node reaches it, about `maxSlotLead / 2.5` seconds; keep that well inside the challenge window.
- When the status is missing, the reporter also looks up the signature of each vote or governance observation at or below the snapshot slot, at finalized commitment. A missing transaction, another slot, or one that does not resolve that ID with that outcome is false evidence. A transaction that does show it contradicts the missing status, so the observation stops without a veto.
- A directly observed void can produce `MsgFinalizeRefund` only with current-policy quorum or retained governance authorization, an elapsed challenge, no hold or contradictory evidence, refunds enabled, the native refund queue head and remaining epoch capacity. A needed fresh vote is submitted before considering finalization on a later pass.

Akash queries for one decision use the same committed height. The client reads both CometBFT status and the application's last committed height because consensus can advertise a height before application queries can serve it. Freshness uses the selected block's timestamp. Membership, rounds, challenges and accounting remain enforced by `x/sink`. Restart recovery comes from native votes and terminal states, with no local delivery cursor that could lose a veto or repeat a refund.

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

There are no durable metrics or scan-latency SLA. Each pass loads the complete open set, so memory and pass time grow with the backlog. The program ID check does not pin deployed bytecode against upgrades, and the admin upgrade trust assumption is unchanged. Earlier local integration exercised an authenticated header, a mint, three signed reporter votes and backing reconciliation. Those results do not validate the current schema and program pairing. Complete live refund recovery, wallet signing and independent multi-operator deployments remain live integration checks.

## Tests and schema

```bash
npm run test:reporter
npm run typecheck
npm run test:sink
npm test
```

The unit tests cover replay evidence, CPI and truncated logs, block spam, wrong owners/amounts/recipients, stale and lagging nodes, fabricated future slots, missing reported signatures, missing history, reuse of matching own-vote evidence, duplicate votes, rotation, contradictions, vetoes and refund eligibility. They also cover light-client pinning and following, open-transfer paging and its consistency checks, dispute/refund priority, populations beyond the former scan limits, audit failures, the round-robin audit, and the external signer protocol against a fake signer in `reporter/testdata` with throwaway keys. `npm test` reads actual finalized automatic and governance voids on a local Solana validator. The validator suite creates its own ledger and stops only its own process. Choose free `AKT_TEST_RPC_PORT` and `AKT_TEST_PORT_BASE` values if its defaults are occupied; preserve existing runtimes.

`npm run test:sink` executes an Akash deposit and exports its real committed proof and protobuf query responses. It mints through the upstream light client, observes the finalized Solana transaction, and signs three `SIGN_MODE_DIRECT` reports with deterministic test keys. The native Akash BaseApp then verifies and executes those exact transaction bytes, reaches quorum, marks the transfer minted and preserves escrow. It uses separate test processes and a trusted test checkpoint; it does not test a continuously running CometBFT network, live RPC broadcasting or authenticated header relaying.

The Akash client types come from the local `@akashnetwork/chain-sdk` package. Build and pack the selected committed SDK source using its existing pipeline, then supply that archive explicitly as described in [SDK integration](sdk-integration.md). Setup only installs the locked input. Update the package lock in `contracts/solana` after an SDK change. SDK int64 and uint64 fields use bigint; JSON boundaries use exact decimal strings.

See [canonical Sink launch and recovery](sink-migration-v2.md) for native protobuf proofs, policy history, governance messages and refund authorization.
