# Production hosting guide

For DevOps. It lists what the Akash to Solana AKT transfer system needs hosted
on mainnet, what each piece talks to, what to back up and what to watch.
Versions, binary links and some owners are not final; they are marked **TBD**.
Launch steps and governance are in [production deployment](production-deployment.md);
this guide covers the infrastructure under them.

## What the system is

Users lock AKT on Akash (`x/sink` module). AKT is then minted on Solana by our
program. Off-chain services carry proofs between the chains and check the
results:

```text
 User (Keplr) ──deposit──▶ Akash (x/sink, akashnet-2)
                               │  headers + proofs
                               ▼
                           Relayer ──mint/void──▶ Solana (AKT program, light client)
                                                      │  finalized results
                               ┌──────────────────────┘
                               ▼
                 Reporters (7+, independent) ──votes──▶ Akash (settles: minted, or refund)
```

- **On-chain parts** (not hosted by DevOps): the `x/sink` module in the Akash
  node binary, and three Solana programs (AKT program, ICS07 light client,
  access manager).
- **Off-chain parts** (hosted): Akash nodes, Solana RPC nodes, the relayer,
  reporters, monitoring and reconciliation, the portal, and the token metadata
  files.

Trust model, in one line each:

- The relayer cannot forge anything. The light client and the AKT program
  verify every header and proof. A broken relayer only stalls transfers.
- Reporters decide whether a transfer settles as minted or refunds on Akash.
  A quorum (at least 5 of 7) is needed, so reporters must be independent:
  different operators, nodes and keys.
- Program upgrade keys can mint without limit. They live in a Squads multisig
  with a time lock and hardware wallets, never on a server.

## Components

| Component                     | What it runs                           | Count                                       | Who operates                                   | Public?     |
| ----------------------------- | -------------------------------------- | ------------------------------------------- | ---------------------------------------------- | ----------- |
| Akash full node               | `akash` with x/sink                    | 1 per relayer, reporter, monitor and portal | Each operator for their own                    | No          |
| Solana RPC node               | Agave (`agave-validator` in RPC mode)  | 1 per reporter; 2 sources for the relayer   | Each operator for their own                    | No          |
| Relayer                       | `local/relay.ts --config`              | 1 required, 2 or more recommended           | DevOps (and anyone else)                       | No          |
| Reporter                      | `reporter/run.ts --watch --broadcast`  | 7 or more                                   | 7 independent operators; DevOps runs only ours | No          |
| Monitoring and reconciliation | Alert rules plus the supply reconciler | 1                                           | DevOps                                         | No          |
| Portal                        | Static web app plus a small API        | 1                                           | DevOps                                         | Yes (HTTPS) |
| Token metadata                | `akt.json` and `akt.png`               | 1                                           | DevOps                                         | Yes (HTTPS) |

Keep each component on its own host or VM. Never run the relayer, a reporter
and the portal on one machine, and keep no keys on the portal host.

## Versions and artifacts

| Item                        | Version                                                                                       | Source                                                                              |
| --------------------------- | --------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| Akash node with x/sink      | **TBD**: release `v2.2.0` from public `akash-network/node` `feat/sink` (tested at `f7d52ce3`) | Akash release binary, **TBD** link                                                  |
| Akash chain SDK             | **TBD**: release from public `akash-network/chain-sdk` `feat/sink` (tested at `b6a12d9e`)     | Built into the node                                                                 |
| Agave (Solana)              | Match mainnet's majority version at launch; 4.3.2 on 2026-10-23                               | `agave-validator` is built from the public tag; Anza no longer ships it in releases |
| Relayer, reporter and tools | **TBD**: release of `akash-network/node` `contracts/solana`                                   | Built in CI                                                                         |
| Node.js                     | 24.14.1                                                                                       | nodejs.org                                                                          |
| Rust                        | 1.89.0 (`contracts/solana/rust-toolchain.toml`)                                               | rustup                                                                              |
| Anchor CLI                  | 0.32.1                                                                                        | build host only                                                                     |
| Go                          | 1.26.2 (node `go.mod`)                                                                        | build host only                                                                     |
| Program hashes              | Published at launch from `npm run build:verifiable`                                           | Release notes, **TBD**                                                              |

Build once in CI and ship artifacts; do not build on production hosts. A
`contracts/solana` build needs `npm ci`, `npm run build` (program IDL),
`npm run setup:light-client` (patched verifier IDL) and
`npm run build:header-encoder` (Rust helper the relayer calls). Ship the
resulting workspace (or a container image of it) with the commit hash.

## Hosts

### Akash full node

| Need                                        | Value                                                                                                |
| ------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Binary                                      | `akash` with x/sink, run under cosmovisor                                                            |
| Chain                                       | `akashnet-2`                                                                                         |
| Ports used by our services                  | CometBFT RPC 26657, REST 1317, gRPC 9090                                                             |
| Pruning, relayer's node                     | Keep at least one light-client trusting period of state (14 days). `pruning = "nothing"` is simplest |
| Pruning, reporter, monitor and portal nodes | Default pruning is fine; they read the latest height                                                 |
| Health                                      | Not catching up; latest block younger than 60 s                                                      |

Upgrade notes (the chain upgrade that installs x/sink):

- Each node must stop at the upgrade height and restart on the new binary.
  cosmovisor 1.7.1 confirms the height by calling `akash status`; set
  `AKASH_NODE` (or `client.toml`) so that call reaches the local node.
- Cosmovisor switches by itself only if the node is running when it reaches
  the height. It does not switch if the node is restarted after halting at the
  height, or if it reaches the height while catching up (block sync from
  behind, or a pre-upgrade snapshot). Installing the binary and restarting is
  not enough. Fix it by hand: copy `data/upgrade-info.json` into
  `cosmovisor/upgrades/<name>/`, point `cosmovisor/current` at that directory
  and restart. Put this in the upgrade runbook. After the upgrade, start new
  nodes from post-upgrade snapshots on the new binary.
- Have the new binary in place on every node before the height. Our nodes
  must not miss the height; transfers cannot start until they follow the chain.
- Rehearsed on 20-validator copies of mainnet state at mainnet's voting
  shares: nodes that downloaded the binary through cosmovisor
  (`DAEMON_ALLOW_DOWNLOAD_BINARIES=true`) and nodes with it installed by hand
  all switched on their own; the chain stopped for about 10 seconds. With
  over a third of voting power missing the binary the chain halted until
  enough nodes had it, then continued. A node whose new binary failed in the
  upgrade handler crashed alone and recovered once the correct binary was
  installed.
- A full state export (`akash export`) of mainnet-sized state needs about
  80 GB of free memory. Do not run it on a host that is also running nodes.

### Solana RPC node

| Need           | Value                                                                                                                                                                            |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Software       | Agave at mainnet's version, RPC mode, not voting                                                                                                                                 |
| Required flags | `--full-rpc-api --enable-rpc-transaction-history` (plus `--enable-extended-tx-metadata-storage`)                                                                                 |
| History        | Must cover the 72-hour challenge period plus reporter outages. Plan for 7 days or more. At 200 ms slots, 72 hours is about 1.3 million slots                                     |
| Hardware       | Follow Anza's current RPC node requirements: hundreds of GB of RAM, fast NVMe, high clock CPUs with SHA and AVX2 (virtual CPUs must expose them; on Proxmox use CPU type `host`) |
| Older history  | Keep an archive source (BigTable or a provider) for evidence older than local retention                                                                                          |
| Ports          | RPC 8899 (HTTP), 8900 (WebSocket); private only                                                                                                                                  |

Reporters must each use their own node. Finalized commitment does not
authenticate a dishonest node, so reporters must not share one or rely on a
public provider. The relayer can use a trusted provider as its second source.

### Relayer

| Need            | Value                                                                                                   |
| --------------- | ------------------------------------------------------------------------------------------------------- |
| Runtime         | Node 24 plus the `contracts/solana` artifact; no validator                                              |
| Config          | JSON file, see [operators](operators.md#configure) (`relayer/config.example.json`)                      |
| Inputs          | Its Akash node (proof queries), Solana RPC                                                              |
| Key             | Solana fee payer keypair file, mode 0600. Hot key holding a few days of spend                           |
| State directory | Header-upload journals, upload lock, spend ledger. One running relayer per directory                    |
| Process         | `npm run relayer -- --config /etc/akt-relayer/config.json` under systemd or a container, restart always |
| Output          | JSON lines on stdout; alerts on stderr                                                                  |

Funding: a mint to a new wallet costs about 0.0025 to 0.0035 SOL; header
updates about 0.0015 SOL each. Keep a few days of spend plus 0.05 SOL. The
relayer does not alert on its own balance; monitoring must.

Throughput: one relayer resolves transfers one at a time, about 11 a minute
(a burst of 90 cleared in 8 minutes on the fork). Run a second relayer with
its own payer and state directory for redundancy and bursts.

### Reporter

Each reporter operator runs one. DevOps runs ours and gives other operators
this section.

| Need    | Value                                                                                                                                                                 |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Runtime | Node 24 plus the `contracts/solana` artifact                                                                                                                          |
| Config  | JSON file, see [reporter](reporter.md#run-an-independent-reporter) (`reporter/config.example.json`)                                                                   |
| Inputs  | Its own Akash node and its own Solana RPC node                                                                                                                        |
| Key     | Akash account key in a KMS or HSM through the external signer interface ([key custody](reporter.md#key-custody)). A key file is for testing only                      |
| Funds   | AKT for transaction fees in the reporter account                                                                                                                      |
| Process | `npm run reporter -- --config PATH --watch --broadcast` with the signer, restart always. Run first without `--broadcast` (preview) until governance adds the reporter |
| Rule    | One signing process per reporter account                                                                                                                              |

### Monitoring and reconciliation

What exists today: the relayer and reporters print alerts as JSON lines, and
`local/reconcile.ts` and `local/monitor.ts` check supply and health on test
networks. **TBD**: a configured production version of the reconciler and
monitor. Until then, alert rules must read the service logs and chain state
directly.

Every alert goes to a person on call. The full list is in
[production deployment](production-deployment.md#monitoring). The ones DevOps
owns:

| Signal                                                                                                               | Alert                               |
| -------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| Light-client age (now minus latest verified Akash time)                                                              | Warn at 25% of 14 days, page at 50% |
| Transfer pending on Akash too long                                                                                   | Page well past a few minutes        |
| Supply mismatch (Solana supply vs AKT locked on Akash)                                                               | Page on any mismatch                |
| Program or configuration change on Solana, any x/sink proposal                                                       | Page                                |
| Relayer alerts (`relay-unbacked-settlement`, `relay-budget-exhausted`, `header-unsupported`, `light-client-changed`) | Page                                |
| Reporter alerts (see [reporter alerts](reporter.md#alerts))                                                          | Page                                |
| Relayer payer and reporter fee balances                                                                              | Below a few days of spend           |
| Node height stalled, node catching up                                                                                | Page                                |
| Solana history no longer covers the challenge period                                                                 | Page                                |
| Disk above 70% on any host                                                                                           | Page                                |
| Clock drift (NTP)                                                                                                    | Page                                |

### Portal

| Need     | Value                                                                                                                                               |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| What     | Static web app plus an API that reads Akash and Solana and proxies Akash RPC and REST for Keplr                                                     |
| Inputs   | Its own Akash node (RPC, REST) and a Solana RPC                                                                                                     |
| Keys     | None. Production has no faucet                                                                                                                      |
| Exposure | Public HTTPS behind a CDN or load balancer with rate limits. Only the portal is public, never the nodes behind it                                   |
| Status   | **TBD**: the portal lives in its own repository. The testnet server includes a faucet and testnet wording; a production server is still to be built |

### Token metadata

| Need    | Value                                                                          |
| ------- | ------------------------------------------------------------------------------ |
| Files   | `akt.json` (name, symbol, description, image URL) and `akt.png`                |
| Hosting | Durable public HTTPS, correct content types, no authentication                 |
| URLs    | Versioned (for example `/token/v1/akt.json`); never change a published version |
| Record  | Store the files' hashes with the launch record                                 |

See the [metadata launch gate](token-metadata.md#mainnet-launch-gate).

## Network and access

- Only the portal and the metadata files are public. Nodes, relayer and
  reporters accept no inbound connections except from their own operators.
- Akash and Solana P2P ports must be reachable by peers as each network
  requires; RPC ports must not be.
- Run NTP on every host.
- Separate cloud accounts or credentials for the relayer and our reporter, so
  one compromise does not reach both.

## Keys

| Key                                               | Where it lives                                             | On a server?            |
| ------------------------------------------------- | ---------------------------------------------------------- | ----------------------- |
| Program upgrade authorities, access-manager admin | Squads multisig vault (members on hardware wallets)        | No                      |
| Ceremony key                                      | Offline machine; deleted after launch                      | No                      |
| Solana pause guardian                             | On-call holder chosen by governance                        | No (on-call device)     |
| x/sink pause guardian                             | On-call key, multisig or group policy chosen by governance | No                      |
| Relayer fee payer                                 | Relayer host, mode 0600, small balance                     | Yes                     |
| Reporter account keys                             | Each operator's KMS or HSM                                 | Through the signer only |

Guardians can only pause. Practice a pause with them before launch.

## Backups

| Data                                                                                 | Back up?                 | Why                                                                                |
| ------------------------------------------------------------------------------------ | ------------------------ | ---------------------------------------------------------------------------------- |
| Relayer config and state directory                                                   | Yes, frequently          | Journals resume interrupted uploads; the spend ledger keeps limits across restarts |
| Relayer payer key                                                                    | Yes, in a secret manager | Losing it only costs its balance, but replacing it needs a multisig role grant     |
| Reporter configs                                                                     | Yes                      | Keys stay in the KMS                                                               |
| Launch records (program hashes, checkpoint, transaction signatures, metadata hashes) | Yes, permanently         | Audit trail                                                                        |
| Akash and Solana node data                                                           | No                       | Restore from snapshots or resync; keep tested snapshot sources                     |
| Logs                                                                                 | 30 days or more          | Incident review                                                                    |

Never copy chain data into backup or evidence folders on the same disk. A full
disk halted the test network for 29 hours and expired its light client.

## Failure effects

| Failure                              | Effect                                                | Action                                                                                                                                                                  |
| ------------------------------------ | ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Relayer down                         | Transfers wait; nothing is lost                       | Restart; it resumes an interrupted upload from its state directory; a second relayer covers it                                                                          |
| Relayer down near 14 days            | Light client expires; mints stop                      | Page early (client age alert). Recovery: deploy a new ICS07 instance, `init-client` from a new checkpoint and `repin` through the multisig; waiting transfers then mint |
| More than 2 of 7 reporters down      | Transfers mint on Solana but stay pending on Akash    | Restore reporters; governance can settle a stalled transfer with `MsgResolveTransfer`                                                                                   |
| Daily mint cap reached               | Transfers wait with their AKT locked on Akash         | None needed; they mint once the cap allows                                                                                                                              |
| Solana RPC history gap               | Reporters cannot confirm older transfers              | Archive source; never shrink retention                                                                                                                                  |
| Akash node misses upgrade height     | That service stops following the chain                | Manual cosmovisor switch (above)                                                                                                                                        |
| Portal down                          | Users cannot use the web app; CLI deposits still work | Restart; no keys at risk                                                                                                                                                |
| Key compromise or suspected bad mint |                                                       | Guardians pause; follow the [runbooks](production-deployment.md#incident-runbooks)                                                                                      |

## Deployment order

1. Before launch: Akash nodes and Solana RPC nodes synced; relayer and reporter
   hosts built; monitoring and paging live; metadata files hosted.
2. Solana programs, a day or more before the Akash upgrade (multisig; see
   [deploying the programs](deploy-program.md)): deploy, initialize (the mint
   starts at supply 0), authorities to the multisig, metadata and guardian.
3. Akash upgrade height: every Akash node switches to the x/sink binary.
   x/sink installs with deposits off.
4. Checkpoint ceremony after the upgrade: light client and bridge
   configuration through the multisig. Start the relayer at once; the client's
   trusting period starts at the checkpoint.
5. Reporters run in preview, then broadcast after governance adds them.
6. A governance proposal sets reporters, limits and the guardian and turns
   deposits on; the multisig enables minting.
7. Portal goes live at activation.

## Open items

- Release versions and binary links (node `v2.2.0`, chain SDK, `contracts/solana`).
- Production portal server.
- Configured production monitor and reconciler.
- Reporter operators and their custody setup.
- Who holds the guardians and multisig seats (decisions L1, L2, L3).
- Production upgrade name and height (decision A1).
