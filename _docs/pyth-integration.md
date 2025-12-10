# Pyth Network Integration on Akash

This guide explains how Akash Network integrates with Pyth Network to provide decentralized, trustworthy price feeds (e.g., AKT/USD) for on-chain use.

## Table of Contents

1. [Introduction](#introduction)
2. [Key Concepts](#key-concepts)
3. [Architecture Overview](#architecture-overview)
4. [Smart Contracts](#smart-contracts)
5. [Hermes Client (Price Relayer)](#hermes-client-price-relayer)
6. [Deployment Guide](#deployment-guide)
7. [Monitoring & Verification](#monitoring--verification)
8. [Troubleshooting](#troubleshooting)
9. [Reference](#reference)

---

## Introduction

### What is this integration for?

Akash Network needs reliable price data (AKT/USD) for [BME](https://github.com/akash-network/AEP/tree/main/spec/aep-76). This integration brings prices from Pyth Network a decentralized oracle network — onto Akash in a cryptographically verifiable way.

### Why Pyth Network?

- **Decentralized**: Prices aggregated from multiple first-party publishers
- **Low latency**: Sub-second price updates available
- **Verifiable**: All data is cryptographically signed via Wormhole
- **Wide coverage**: 500+ price feeds across crypto, equities, FX, and commodities

---

## Key Concepts

Before diving into implementation, understand these foundational concepts:

### Pyth Network

[Pyth Network](https://pyth.network/) is a decentralized oracle network that delivers real-time market data. Unlike traditional oracles that push data on-chain, Pyth uses a "pull" model where consumers fetch and verify data on-demand.

**Key components:**
- **Publishers**: First-party data providers (exchanges, market makers, trading firms)
- **Pythnet**: A Solana-based appchain where prices are aggregated
- **Hermes**: Pyth's web service API for fetching price data with VAA proofs

### Wormhole

[Wormhole](https://wormhole.com/) is a cross-chain messaging protocol that enables secure communication between blockchains. For Pyth integration, Wormhole provides:
- Cryptographic attestation of price data
- Guardian network for decentralized verification

### VAA (Verified Action Approval)

A **VAA** is a signed message from Wormhole's Guardian network that proves data is authentic and came from its actual source. It's the cryptographic proof that makes cross-chain price data trustworthy.

**How VAA verification works:**

1. Pyth publishes prices on Pythnet (a Solana-based network)
2. **19 Wormhole Guardians** observe this data
   - Guardians are validators running full nodes on multiple blockchains
   - Current guardian set includes Google Cloud and other major validators
3. Guardians sign the data — a valid VAA requires **13 of 19 signatures** (2/3 supermajority)
4. The VAA contains:
   - Original message/data (price information)
   - Guardian signatures
   - Metadata (source chain, sequence number, timestamp)
5. On Akash, the Wormhole contract **verifies the VAA signatures** before accepting price data

Without VAA verification, anyone could submit fake prices. The guardian network provides decentralized trust.

> **Source:** Guardian set size (19) and quorum (13/19) from [Wormhole Guardians Documentation](https://wormhole.com/docs/protocol/infrastructure/guardians/):
> *"Wormhole relies on a set of 19 distributed nodes that monitor the state on several blockchains."*
> *"With a two-thirds consensus threshold, only 13 signatures must be verified on-chain."*

### TWAP (Time-Weighted Average Price)

**TWAP** is a pricing algorithm that calculates the average price over a specific time period, weighting each price by how long it was valid. This smooths out short-term volatility and manipulation attempts.

Akash's x/oracle module calculates TWAP from submitted price updates.

### CosmWasm

[CosmWasm](https://cosmwasm.com/) is a smart contract platform for Cosmos SDK chains. Akash uses CosmWasm to deploy the Wormhole and Pyth contracts.

**Key terms:**
- **WASM (WebAssembly)**: Binary format for compiled smart contracts
- **Code ID**: Unique identifier for stored contract code on-chain
- **Instantiate**: Create a contract instance from stored code

---

## Architecture Overview

### High-Level Flow

```
┌──────────────────────────────────────────────────────────────┐
│                     Pyth Network (Off-chain)                 │
│              Publishers → Pythnet → Hermes API               │
└──────────────────────────────────────────────────────────────┘
                                │
                         VAA with prices
                                │
┌───────────────────────────────┼──────────────────────────────┐
│          Hermes Client        │        (Off-chain)           │
│    github.com/akash-network/hermes                           │
│    Fetches VAA and submits to Pyth contract          │
└───────────────────────────────┼──────────────────────────────┘
                                │
                    execute: update_price_feed(vaa)
                                ▼
┌──────────────────────────────────────────────────────────────┐
│                Akash Network (On-chain / CosmWasm)           │
│                                                              │
│  ┌────────────────────────────┐                              │
│  │     Wormhole Contract      │◄─── WASM Contract #1         │
│  │  - Verifies VAA signatures │     Verifies guardian        │
│  │  - Returns verified payload│     signatures (13/19)       │
│  └─────────────▲──────────────┘                              │
│                │ query: verify_vaa                           │
│                │                                             │
│  ┌─────────────┴──────────────┐                              │
│  │    Pyth Contract           │◄─── WASM Contract #2         │
│  │  - Receives VAA from client│     Verifies + relays        │
│  │  - Queries Wormhole        │     in single transaction    │
│  │  - Parses Pyth payload     │                              │
│  │  - Relays to x/oracle      │                              │
│  └─────────────┬──────────────┘                              │
│                │                                             │
│       CosmosMsg::Custom(SubmitPrice)                         │
│                ▼                                             │
│  ┌────────────────────────────┐                              │
│  │      x/oracle Module       │◄─── Native Cosmos module     │
│  │  - Stores price            │     Aggregates prices from   │
│  │  - Calculates TWAP         │     authorized sources       │
│  │  - Health checks           │                              │
│  └────────────────────────────┘                              │
└──────────────────────────────────────────────────────────────┘
```

### Data Flow (Step by Step)

1. **Pyth Publishers** aggregate prices (AKT/USD, etc.) on Pythnet
2. **Wormhole Guardians** (19 validators) observe and sign the price attestation as a VAA
3. **Hermes Client** fetches latest price + VAA from Pyth's Hermes API
4. **Hermes Client** submits VAA to Pyth contract on Akash
5. **Pyth Contract** queries Wormhole to verify VAA signatures
6. **Pyth Contract** parses Pyth price attestation from verified VAA payload
7. **Pyth Contract** relays validated price to x/oracle module
8. **x/oracle Module** stores the price, calculates TWAP, performs health checks
9. **Network consumers** query x/oracle for the latest AKT/USD price

### Why Two Contracts?

| Contract     | Responsibility                                    |
|--------------|---------------------------------------------------|
| **Wormhole** | VAA signature verification (reusable)             |
| **Pyth**     | Verify VAA, parse Pyth payload, relay to x/oracle |

This design is streamlined: the Pyth contract handles VAA verification via Wormhole query, parses the Pyth price attestation internally, and relays directly to x/oracle. No intermediate storage is needed.

---

## Smart Contracts

### 1. Wormhole Contract

**Purpose:** Verify VAA signatures from Wormhole's guardian network.

**Key features:**
- Queries guardian set from x/oracle module params (not stored in contract)
- Validates that 13/19 guardians signed a VAA
- Returns verified VAA payload for other contracts to use
- Guardian set updates managed via Akash governance (not Wormhole governance VAAs)

**Source:** `contracts/wormhole/`

**Query Messages:**
```rust
pub enum QueryMsg {
    // Verify VAA and return parsed contents
    VerifyVAA {
        vaa: Binary,        // Base64-encoded VAA
        block_time: u64,    // Current block time for validation
    },
}
```

### 2. Pyth Contract

**Purpose:** Receive VAA, verify via Wormhole, parse Pyth payload, and relay to x/oracle module.

**Key features:**
- Receives raw VAA from Hermes client
- Queries Wormhole contract to verify VAA signatures
- Parses Pyth price attestation from verified payload
- Validates price feed ID and data source
- Relays validated price to x/oracle module (no local storage)
- Admin-controlled for governance

**Source:** `contracts/pyth/`

**Execute Messages:**
```rust
pub enum ExecuteMsg {
    /// Submit price update with VAA proof
    /// Contract will verify VAA via Wormhole, parse Pyth payload, relay to x/oracle
    UpdatePriceFeed {
        vaa: Binary,         // VAA data from Pyth Hermes API (base64 encoded)
    },
    /// Admin: Update the fee
    UpdateFee { new_fee: Uint256 },
    /// Admin: Transfer admin rights
    TransferAdmin { new_admin: String },
    /// Admin: Refresh cached oracle params
    RefreshOracleParams {},
    /// Admin: Update contract configuration
    UpdateConfig {
        wormhole_contract: Option<String>,
        price_feed_id: Option<String>,
        data_sources: Option<Vec<DataSourceMsg>>,
    },
}
```

**Query Messages:**
```rust
pub enum QueryMsg {
    GetConfig {},        // Returns admin, wormhole_contract, fee, feed ID, data_sources
    GetPrice {},         // Returns latest price (cached from last relay)
    GetPriceFeed {},     // Returns price with metadata
    GetOracleParams {},  // Returns cached x/oracle params (uses custom Akash querier)
}
```

**Internal Flow:**
```
1. Receive VAA from Hermes client
2. Query Wormhole: verify_vaa(vaa) → ParsedVAA
3. Validate emitter is trusted Pyth data source
4. Parse Pyth price attestation from VAA payload
5. Validate price feed ID matches expected (AKT/USD)
6. Send CosmosMsg::Custom(SubmitPrice) to x/oracle module
```

---

## Hermes Client (Price Relayer)

The Hermes Client is a TypeScript service that fetches prices from Pyth's Hermes API and submits them to the Pyth contract on Akash.

**Repository:** [github.com/akash-network/hermes](https://github.com/akash-network/hermes)

### Why is it needed?

Pyth uses a "pull" oracle model—prices aren't automatically pushed on-chain. Someone must:
1. Fetch the latest price from Pyth's API
2. Submit it to the on-chain contract
3. Pay the transaction fees

The Hermes Client automates this process.

### Features

- **Daemon mode**: Continuous updates at configurable intervals
- **Smart updates**: Skips transactions when on-chain price is already current
- **Multi-arch Docker**: Supports `linux/amd64` and `linux/arm64`
- **CLI tools**: Manual updates, queries, admin operations

### Quick Start

```bash
# Clone
git clone https://github.com/akash-network/hermes
cd hermes

# Install & build
npm install
npm run build

# Configure
cp .env.example .env
# Edit .env with your settings

# Run daemon (continuous updates)
npm run cli:daemon
```

### Configuration

| Variable             | Required | Default                       | Description                 |
|----------------------|----------|-------------------------------|-----------------------------|
| `RPC_ENDPOINT`       | Yes      | —                             | Akash RPC endpoint          |
| `CONTRACT_ADDRESS`   | Yes      | —                             | Pyth contract address       |
| `MNEMONIC`           | Yes      | —                             | Wallet mnemonic for signing |
| `HERMES_ENDPOINT`    | No       | `https://hermes.pyth.network` | Pyth Hermes API URL         |
| `UPDATE_INTERVAL_MS` | No       | `300000`                      | Update interval (5 min)     |
| `GAS_PRICE`          | No       | `0.025uakt`                   | Gas price for transactions  |
| `DENOM`              | No       | `uakt`                        | Token denomination          |

### CLI Commands

```bash
# One-time price update
npm run cli:update

# Query current price
npm run cli:query

# Query with options
npm run cli:query -- --feed          # Price feed with metadata
npm run cli:query -- --config        # Contract configuration
npm run cli:query -- --oracle-params # Cached oracle parameters

# Admin commands
npm run cli:admin -- refresh-params       # Refresh oracle params
npm run cli:admin -- update-fee <fee>     # Update fee (in uakt)
npm run cli:admin -- transfer <address>   # Transfer admin rights
```

### Production Deployment

#### Using Pre-built Docker Image (Recommended)

Multi-architecture Docker images (`linux/amd64`, `linux/arm64`) are available from GitHub Container Registry:

```bash
# Pull the latest image
docker pull ghcr.io/akash-network/hermes:latest

# Run with environment variables
docker run -d \
  --name hermes-client \
  -e RPC_ENDPOINT=https://rpc.akashnet.net:443 \
  -e CONTRACT_ADDRESS=akash1... \
  -e "MNEMONIC=your twelve word mnemonic here" \
  --restart unless-stopped \
  ghcr.io/akash-network/hermes:latest node dist/cli.js daemon

# Or use an env file
docker run -d \
  --name hermes-client \
  --env-file .env \
  --restart unless-stopped \
  ghcr.io/akash-network/hermes:latest node dist/cli.js daemon

# View logs
docker logs -f hermes-client
```

**Available tags:**
- `latest` — Latest stable release
- `vX.Y.Z` — Specific version (e.g., `v1.0.0`)
- `vX.Y` — Latest patch for major.minor (e.g., `v1.0`)

#### Docker Compose

Create a `docker-compose.yml`:

```yaml
services:
  hermes-client:
    image: ghcr.io/akash-network/hermes:latest
    container_name: hermes-client
    restart: unless-stopped
    env_file:
      - .env
    command: ["node", "dist/cli.js", "daemon"]
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

```bash
# Start
docker-compose up -d

# View logs
docker-compose logs -f hermes-client

# Stop
docker-compose down
```

#### Building Locally (Alternative)

If you need to build from source:

```bash
git clone https://github.com/akash-network/hermes
cd hermes
docker build -t akash-hermes-client .

# Run locally-built image
docker run -d \
  --name hermes-client \
  --env-file .env \
  --restart unless-stopped \
  akash-hermes-client node dist/cli.js daemon
```

#### Systemd (Linux Production)

For running directly on a Linux server without Docker:

```bash
# 1. Clone and build
git clone https://github.com/akash-network/hermes
cd hermes
npm install
npm run build

# 2. Copy to /opt
sudo mkdir -p /opt/hermes-client
sudo cp -r dist package.json .env /opt/hermes-client/
cd /opt/hermes-client
sudo npm ci --production

# 3. Install systemd service
sudo cp hermes-client.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable hermes-client
sudo systemctl start hermes-client

# 4. Check status
sudo systemctl status hermes-client
sudo journalctl -u hermes-client -f
```

### Cost Estimation

The Hermes client submits transactions to update prices. Costs depend on update frequency:

**Per Update:**
```
Gas cost:   ~150,000 gas × 0.025 uakt/gas = 3,750 uakt
Update fee: 1,000,000 uakt (set in contract)
Total:      ~1,003,750 uakt per update (~0.001 AKT)
```

**Monthly Cost by Interval:**

| Interval | Updates/Month | Approx Monthly Cost |
|----------|---------------|---------------------|
| 5 min    | 8,640         | ~9 AKT              |
| 10 min   | 4,320         | ~4.5 AKT            |
| 15 min   | 2,880         | ~3 AKT              |

> **Tip:** Increase `UPDATE_INTERVAL_MS` to reduce costs. The client only submits transactions when the price has actually changed (newer `publish_time`).

### Smart Update Logic

The Hermes client implements intelligent update logic:

1. Fetches latest price from Pyth Hermes API
2. Queries current price from the on-chain contract
3. Compares `publish_time` timestamps
4. **Skips update** if on-chain price is already current
5. Submits transaction only when new data is available

This minimizes transaction costs and blockchain load.

### Wallet Security

**Best Practices:**

- **Use a dedicated wallet** — Create a separate wallet for oracle updates only
- **Limit funding** — Only keep necessary AKT (monthly costs + buffer)
- **Secure mnemonic** — Use environment variables or secrets manager
- **Never commit .env** — Already in `.gitignore`
- **Monitor activity** — Set up alerts for unusual transactions

---

## Local Development Setup

For local development and testing, use the Docker Compose setup that includes both the Akash node and Hermes price relayer.

### Quick Start

```bash
# 1. Build contracts (if not already built)
cd contracts
make build

# 2. Start the local stack
cd _build
docker-compose -f docker-compose.local.yml up -d

# 3. View logs
docker-compose -f docker-compose.local.yml logs -f

# 4. Verify node is running
curl http://localhost:26657/status

# 5. Query oracle price (after Hermes submits prices)
docker exec akash-node akash query oracle prices --chain-id localakash
```

### Services

| Service       | Port  | Description                                       |
|---------------|-------|---------------------------------------------------|
| akash-node    | 26657 | Tendermint RPC                                    |
| akash-node    | 9090  | gRPC                                              |
| akash-node    | 1317  | REST API                                          |
| hermes-client | -     | Price relayer (connects to akash-node internally) |

### What Happens on Startup

1. **validator** initializes a single-node validator with:
   - Permissionless WASM (for direct contract deployment)
   - Pre-funded validator and hermes accounts
   - Guardian addresses configured in x/oracle params

2. **validator** deploys contracts:
   - Stores and instantiates Wormhole contract
   - Stores and instantiates Pyth contract
   - Registers Pyth as authorized oracle source

3. **hermes-client** waits for contracts, then:
   - Reads contract address from shared volume
   - Starts daemon to submit prices every 60 seconds

### Cleanup

```bash
# Stop services
docker-compose -f docker-compose.yml down

# Stop and remove all data (full reset)
docker-compose -f docker-compose.yml down -v
```

---

## Deployment Guide

> **Note:** On Akash mainnet, contract code can only be stored via governance proposals. Direct uploads are restricted.

### Prerequisites

**Tools Required:**
- `akash` CLI (v0.36.0+)
- `cargo` and Rust toolchain (for building contracts)
- Access to governance key (for mainnet deployments)

**Contract Artifacts:**

Pre-built WASM binaries are available in:
```
contracts/wormhole/artifacts/wormhole.wasm
contracts/pyth/artifacts/pyth.wasm
```

**Building from Source:**

```bash
make build-contracts
```

### Step 1: Deploy Wormhole Contract

The Wormhole contract must be deployed first as it has no dependencies.

#### 1.1 Store Code Proposal

```bash
akash tx gov submit-proposal wasm-store \
  contracts/wormhole/artifacts/wormhole.wasm \
  --title "Store Wormhole Contract" \
  --summary "Deploy Wormhole bridge contract for VAA verification. This contract enables cryptographic verification of cross-chain messages from the Pyth Network." \
  --deposit 100000000uakt \
  --instantiate-anyof-addresses "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f" \
  --from <your-key> \
  --chain-id akashnet-2 \
  --gas auto \
  --gas-adjustment 1.5 \
  --gas-prices 0.025uakt
```

#### 1.2 Vote on Proposal

```bash
akash tx gov vote <proposal-id> yes \
  --from <your-key> \
  --chain-id akashnet-2
```

#### 1.3 Instantiate Wormhole Contract

After the proposal passes, instantiate the contract:

```bash
# Instantiate message
# Note: Guardian addresses are loaded from x/oracle params, not stored in the contract
WORMHOLE_INIT='{
  "gov_chain": 1,
  "gov_address": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQ=",
  "chain_id": 29,
  "fee_denom": "uakt"
}'

# Submit instantiate proposal
akash tx gov submit-proposal instantiate-contract <code-id> \
  "$WORMHOLE_INIT" \
  --label "wormhole-v1" \
  --title "Instantiate Wormhole Contract" \
  --summary "Initialize Wormhole contract (guardian set managed via x/oracle params)" \
  --deposit 100000000uakt \
  --admin "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f" \
  --from <your-key> \
  --chain-id akashnet-2
```

**Wormhole Instantiate Parameters:**

| Parameter      | Type   | Description                                    | Example           |
|----------------|--------|------------------------------------------------|-------------------|
| `gov_chain`    | u16    | Wormhole governance chain ID                   | `1` (Solana)      |
| `gov_address`  | Binary | Governance contract address (32 bytes, base64) | See Wormhole docs |
| `chain_id`     | u16    | Wormhole chain ID for Akash                    | `29`              |
| `fee_denom`    | String | Native token denomination                      | `"uakt"`          |

> **Note:** Guardian addresses are managed via x/oracle module params, not stored in the Wormhole contract. This enables guardian set updates via Akash governance rather than Wormhole governance VAAs. See [Guardian Set Management](#guardian-set-management) below.

### Step 2: Deploy Pyth Contract

#### 2.1 Store Code Proposal

```bash
akash tx gov submit-proposal wasm-store \
  contracts/pyth/artifacts/pyth.wasm \
  --title "Store Pyth Contract" \
  --summary "Deploy Pyth contract to verify Pyth VAAs and relay prices to x/oracle module." \
  --deposit 100000000uakt \
  --instantiate-anyof-addresses "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f" \
  --from <your-key> \
  --chain-id akashnet-2
```

#### 2.2 Instantiate Pyth Contract

```bash
# Replace <wormhole-contract-address> with actual address from Step 1
ORACLE_INIT='{
  "admin": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f",
  "wormhole_contract": "<wormhole-contract-address>",
  "update_fee": "1000000",
  "price_feed_id": "0xef0d8b6fda2ceba41da15d4095d1da392a0d2f8ed0c6c7bc0f4cfac8c280b56d",
  "data_sources": [
    {
      "emitter_chain": 26,
      "emitter_address": "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71"
    }
  ]
}'

akash tx gov submit-proposal instantiate-contract <code-id> \
  "$ORACLE_INIT" \
  --label "pyth-v1" \
  --title "Instantiate Pyth Contract" \
  --summary "Initialize pyth with Wormhole contract and Pyth data sources" \
  --deposit 100000000uakt \
  --admin "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f" \
  --from <your-key> \
  --chain-id akashnet-2
```

**Pyth Instantiate Parameters:**

| Parameter                        | Type   | Description                       | Example                  |
|----------------------------------|--------|-----------------------------------|--------------------------|
| `admin`                          | String | Admin address                     | Governance address       |
| `wormhole_contract`              | String | Wormhole contract address         | `akash1...`              |
| `update_fee`                     | String | Fee for price updates (Uint256)   | `"1000000"`              |
| `price_feed_id`                  | String | Pyth price feed ID (64-char hex)  | AKT/USD feed ID          |
| `data_sources[].emitter_chain`   | u16    | Wormhole chain ID                 | `26` (Pythnet)           |
| `data_sources[].emitter_address` | String | Pyth emitter address (32 bytes)   | See Pyth docs            |

### Step 3: Register as Oracle Source

After deploying the Pyth contract, register it as an authorized price source in the x/oracle module.

```bash
# Create param change proposal JSON
cat > oracle-params-proposal.json << 'EOF'
{
  "title": "Register Pyth Contract as Oracle Source",
  "summary": "Add the pyth contract address to authorized sources and configure oracle parameters for Pyth integration.",
  "messages": [
    {
      "@type": "/akash.oracle.v1.MsgUpdateParams",
      "authority": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f",
      "params": {
        "sources": ["<pyth-contract-address>"],
        "min_price_sources": 1,
        "max_price_staleness_blocks": 60,
        "twap_window": 180,
        "max_price_deviation_bps": 150,
        "feed_contracts_params": [
          {
            "@type": "/akash.oracle.v1.PythContractParams",
            "akt_price_feed_id": "0xef0d8b6fda2ceba41da15d4095d1da392a0d2f8ed0c6c7bc0f4cfac8c280b56d"
          },
          {
            "@type": "/akash.oracle.v1.WormholeContractParams",
            "guardian_addresses": [
              "58CC3AE5C097b213cE3c81979e1B9f9570746AA5",
              "fF6CB952589BDE862c25Ef4392132fb9D4A42157",
              "..."
            ]
          }
        ]
      }
    }
  ],
  "deposit": "100000000uakt"
}
EOF

# Submit proposal
akash tx gov submit-proposal oracle-params-proposal.json \
  --from <your-key> \
  --chain-id akashnet-2
```

**Oracle Parameters:**

| Parameter                    | Type     | Description                      | Default         |
|------------------------------|----------|----------------------------------|-----------------|
| `sources`                    | []String | Authorized contract addresses    | `[]`            |
| `min_price_sources`          | u32      | Minimum sources for valid price  | `1`             |
| `max_price_staleness_blocks` | i64      | Max age in blocks (~6s/block)    | `60` (~6 min)   |
| `twap_window`                | i64      | TWAP calculation window (blocks) | `180` (~18 min) |
| `max_price_deviation_bps`    | u64      | Max deviation in basis points    | `150` (1.5%)    |

### Guardian Set Management

Guardian addresses for the Wormhole contract are stored in x/oracle module params, not in the Wormhole contract itself. This architecture enables:

- **Akash governance control**: Guardian set updates via Akash governance proposals
- **Faster incident response**: No need for Wormhole governance VAAs to update guardians
- **Simpler operations**: Single source of truth for guardian configuration

**Updating Guardian Addresses:**

To update the Wormhole guardian set, submit a governance proposal that includes `WormholeContractParams` in the `feed_contracts_params`:

```bash
cat > guardian-update-proposal.json << 'EOF'
{
  "title": "Update Wormhole Guardian Set",
  "summary": "Update guardian addresses to Wormhole Guardian Set 5",
  "messages": [
    {
      "@type": "/akash.oracle.v1.MsgUpdateParams",
      "authority": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f",
      "params": {
        "feed_contracts_params": [
          {
            "@type": "/akash.oracle.v1.PythContractParams",
            "akt_price_feed_id": "0xef0d8b6fda2ceba41da15d4095d1da392a0d2f8ed0c6c7bc0f4cfac8c280b56d"
          },
          {
            "@type": "/akash.oracle.v1.WormholeContractParams",
            "guardian_addresses": [
              "58CC3AE5C097b213cE3c81979e1B9f9570746AA5",
              "fF6CB952589BDE862c25Ef4392132fb9D4A42157",
              "114De8460193bdf3A2fCf81f86a09765F4762fD1",
              "107A0086b32d7A0977926A205131d8731D39cbEB",
              "8C82B2fd82FaeD2711d59AF0F2499D16e726f6b2",
              "11b39756C042441BE6D8650b69b54EbE715E2343",
              "54Ce5B4D348fb74B958e8966e2ec3dBd4958a7cd",
              "15e7cAF07C4e3DC8e7C469f92C8Cd88FB8005a20",
              "74a3bf913953D695260D88BC1aA25A4eeE363ef0",
              "000aC0076727b35FBea2dAc28fEE5cCB0fEA768e",
              "AF45Ced136b9D9e24903464AE889F5C8a723FC14",
              "f93124b7c738843CBB89E864c862c38cddCccF95",
              "D2CC37A4dc036a8D232b48f62cDD4731412f4890",
              "DA798F6896A3331F64b48c12D1D57Fd9cbe70811",
              "71AA1BE1D36CaFE3867910F99C09e347899C19C3",
              "8192b6E7387CCd768277c17DAb1b7a5027c0b3Cf",
              "178e21ad2E77AE06711549CFBB1f9c7a9d8096e8",
              "5E1487F35515d02A92753504a8D75471b9f49EdB",
              "6FbEBc898F403E4773E95feB15E80C9A99c8348d"
            ]
          }
        ]
      }
    }
  ],
  "deposit": "100000000uakt"
}
EOF

akash tx gov submit-proposal guardian-update-proposal.json \
  --from <your-key> \
  --chain-id akashnet-2
```

> **Note:** Guardian addresses are 20-byte Ethereum-style addresses (40 hex characters). Get the current guardian set from [Wormhole documentation](https://wormhole.com/docs/protocol/infrastructure/guardians/).

### Step 4: Run Hermes Client

See the [Hermes Client](#hermes-client-price-relayer) section above for installation and configuration.

---

## Monitoring & Verification

### Query Contract State

```bash
# Wormhole - Get guardian set info
akash query wasm contract-state smart <wormhole-contract> \
  '{"guardian_set_info":{}}'

# Pyth - Get config (includes wormhole_contract, data_sources)
akash query wasm contract-state smart <pyth-contract> \
  '{"get_config":{}}'

# Pyth - Get latest price
akash query wasm contract-state smart <pyth-contract> \
  '{"get_price":{}}'

# Pyth - Get price with metadata
akash query wasm contract-state smart <pyth-contract> \
  '{"get_price_feed":{}}'

# Pyth - Get oracle params (uses custom Akash querier)
akash query wasm contract-state smart <pyth-contract> \
  '{"get_oracle_params":{}}'
```

### Query Oracle Module

```bash
# Get oracle parameters
akash query oracle params

# Get aggregated price (after prices are submitted)
akash query oracle price uakt usd

# Get all prices
akash query oracle prices
```

### Health Checks

```bash
# Check contract code info
akash query wasm code <code-id>

# Check contract info
akash query wasm contract <contract-address>

# List all contracts by code
akash query wasm list-contract-by-code <code-id>
```

### Hermes Client Monitoring

```bash
# Query current price via CLI
npm run cli:query

# Check logs (Docker)
docker-compose logs -f hermes-client

# Check logs (systemd)
journalctl -u hermes-client -f
```

---

## Troubleshooting

### Common Errors

| Issue                            | Cause                             | Solution                                                      |
|----------------------------------|-----------------------------------|---------------------------------------------------------------|
| `Unsupported query type: custom` | Node missing custom Akash querier | Upgrade to node v2.x+ with custom querier support             |
| `unauthorized oracle provider`   | Contract not in `sources` param   | Add contract address via governance proposal                  |
| `price timestamp is too old`     | Stale price data                  | Submit fresher price update or increase `staleness_threshold` |
| `VAA verification failed`        | Invalid guardian signatures       | Verify guardian set matches current Wormhole mainnet          |
| `source not authorized`          | Missing from oracle sources       | Update oracle params via governance                           |
| `price timestamp is from future` | Clock skew                        | Check publisher/relayer clock synchronization                 |
| `price must be positive`         | Zero or negative price            | Check price feed data validity                                |

### Contract Instantiation Errors

```
Error: failed to execute message; message index: 0: invalid request
```
- Check JSON format matches expected schema
- Verify all required fields are present
- Ensure addresses are valid bech32 format

### Hermes Client Errors

| Issue                         | Cause                | Solution                                  |
|-------------------------------|----------------------|-------------------------------------------|
| `Client not initialized`      | Missing initialize() | Ensure `await client.initialize()` called |
| `Insufficient funds`          | Wallet empty         | Fund wallet with AKT                      |
| `Failed to fetch from Hermes` | Network/API issue    | Check Hermes API status                   |
| `Price already up to date`    | Normal behavior      | Client will retry on next interval        |

**Debug Mode:**
```bash
export DEBUG=*
npm run cli:daemon
```

**Test Hermes API:**
```bash
curl "https://hermes.pyth.network/v2/updates/price/latest?ids=<PRICE_FEED_ID>"
```

---

## Reference

### Abbreviations Used

| Abbreviation | Full Term                          | Description                                |
|--------------|------------------------------------|--------------------------------------------|
| VAA          | Verified Action Approval           | Signed message from Wormhole guardians     |
| TWAP         | Time-Weighted Average Price        | Average price weighted by time duration    |
| WASM         | WebAssembly                        | Binary format for smart contracts          |
| API          | Application Programming Interface  | Interface for software communication       |
| RPC          | Remote Procedure Call              | Protocol for executing code on remote systems |
| SDK          | Software Development Kit           | Tools for building applications            |
| CLI          | Command Line Interface             | Text-based interface for running commands  |
| AKT          | Akash Token                        | Native token of Akash Network              |
| USD          | United States Dollar               | Fiat currency reference                    |

### External Links

- [Pyth Network Documentation](https://docs.pyth.network/)
- [Pyth Hermes API](https://hermes.pyth.network/docs/)
- [Pyth Price Feed IDs](https://pyth.network/developers/price-feed-ids)
- [Wormhole Documentation](https://docs.wormhole.com/)
- [Wormhole Guardians](https://wormhole.com/docs/protocol/infrastructure/guardians/)
- [Wormhole Guardian Set Constants](https://docs.wormhole.com/wormhole/reference/constants)
- [CosmWasm Documentation](https://docs.cosmwasm.com/)

### Source Code

| Component      | Location                          |
|----------------|-----------------------------------|
| Wormhole       | `contracts/wormhole/`             |
| Pyth           | `contracts/pyth/`                 |
| x/oracle       | `x/oracle/`                       |
| Custom Querier | `x/wasm/bindings/`                |
| Hermes Client  | `github.com/akash-network/hermes` |
| E2E Tests      | `tests/e2e/pyth_contract_test.go` |

### Key Files

| File                                | Description                |
|-------------------------------------|----------------------------|
| `x/oracle/keeper/keeper.go`         | Oracle module keeper       |
| `x/wasm/bindings/custom_querier.go` | Custom Akash query handler |
| `x/wasm/bindings/akash_query.go`    | Query type definitions     |
| `contracts/pyth/src/msg.rs`         | Contract message schemas   |
| `contracts/pyth/src/pyth.rs`        | Pyth payload parser        |
| `contracts/pyth/src/wormhole.rs`    | Wormhole query interface   |

---

## Appendix: Message Schemas

### Wormhole QueryMsg

```rust
pub enum QueryMsg {
    /// Verify VAA signatures and return parsed contents
    VerifyVAA {
        vaa: Binary,       // Base64-encoded VAA
        block_time: u64,   // Current block time for validation
    },
    /// Get current guardian set info
    GuardianSetInfo {},
}
```

### Wormhole ParsedVAA Response

```rust
pub struct ParsedVAA {
    pub version: u8,
    pub guardian_set_index: u32,
    pub timestamp: u32,
    pub nonce: u32,
    pub len_signers: u8,
    pub emitter_chain: u16,      // Source chain (26 = Pythnet)
    pub emitter_address: Vec<u8>, // 32-byte emitter address
    pub sequence: u64,
    pub consistency_level: u8,
    pub payload: Vec<u8>,        // Pyth price attestation data
    pub hash: Vec<u8>,
}
```

### Pyth ExecuteMsg

```rust
pub enum ExecuteMsg {
    /// Submit price update with VAA proof
    /// Contract verifies VAA via Wormhole, parses Pyth payload, relays to x/oracle
    UpdatePriceFeed {
        vaa: Binary,  // VAA from Pyth Hermes API (base64 encoded)
    },
    /// Admin: Update the fee
    UpdateFee { new_fee: Uint256 },
    /// Admin: Transfer admin rights
    TransferAdmin { new_admin: String },
    /// Admin: Refresh cached oracle params from chain
    RefreshOracleParams {},
    /// Admin: Update contract configuration
    UpdateConfig {
        wormhole_contract: Option<String>,
        price_feed_id: Option<String>,
        data_sources: Option<Vec<DataSourceMsg>>,
    },
}

pub struct DataSourceMsg {
    pub emitter_chain: u16,      // Wormhole chain ID (26 for Pythnet)
    pub emitter_address: String, // 32 bytes, hex encoded
}
```

### Pyth QueryMsg

```rust
pub enum QueryMsg {
    GetConfig {},        // Returns admin, wormhole_contract, fee, feed ID, data_sources
    GetPrice {},         // Returns latest price
    GetPriceFeed {},     // Returns price with metadata
    GetOracleParams {},  // Returns cached x/oracle params (uses custom Akash querier)
}
```

### Pyth Price Attestation Format

The Pyth contract parses Pyth price attestation from the VAA payload:

```rust
/// Parsed Pyth price data from VAA payload
pub struct PythPrice {
    pub id: String,       // Price feed ID (32 bytes, hex encoded)
    pub price: i64,       // Price value (scaled by 10^expo)
    pub conf: u64,        // Confidence interval
    pub expo: i32,        // Price exponent (e.g., -8 means divide by 10^8)
    pub publish_time: i64, // Unix timestamp when price was published
    pub ema_price: i64,   // Exponential moving average price
    pub ema_conf: u64,    // EMA confidence interval
}
```

**P2WH Format (Batch Price Attestation):**
- Magic bytes: `P2WH` (0x50325748)
- Major/minor version: 2 bytes each
- Header size: 2 bytes
- Attestation count: 2 bytes
- Attestation size: 2 bytes
- Each attestation: 150 bytes containing price data
