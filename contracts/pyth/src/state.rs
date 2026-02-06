use cosmwasm_schema::cw_serde;
use cosmwasm_std::{Addr, Uint128, Uint256};
use cw_storage_plus::Item;

/// DataID uniquely identifies a price pair by asset and base denomination
/// Used in Config to store the default price pair for this contract
#[cw_serde]
pub struct DataID {
    /// Asset denomination (e.g., "uakt")
    pub denom: String,
    /// Base denomination for the price pair (e.g., "usd")
    pub base_denom: String,
}

impl DataID {
    pub fn new(denom: String, base_denom: String) -> Self {
        Self { denom, base_denom }
    }

    /// Default for AKT/USD pair
    /// Note: Oracle module expects "akt" (not "uakt") and "usd" as denom/base_denom
    pub fn akt_usd() -> Self {
        Self {
            denom: "akt".to_string(),
            base_denom: "usd".to_string(),
        }
    }
}

impl Default for DataID {
    fn default() -> Self {
        Self::akt_usd()
    }
}

/// A data source identifies a valid price feed source (Pyth publisher)
#[cw_serde]
pub struct DataSource {
    /// Wormhole chain ID of the emitter (26 for Pythnet)
    pub emitter_chain: u16,
    /// Emitter address (32 bytes, hex encoded)
    pub emitter_address: String,
}

impl DataSource {
    /// Check if this data source matches the given emitter chain and address
    pub fn matches(&self, chain: u16, address: &[u8]) -> bool {
        if self.emitter_chain != chain {
            return false;
        }
        // Compare hex-encoded address with raw bytes
        match hex::decode(&self.emitter_address) {
            Ok(decoded) => decoded == address,
            Err(_) => false,
        }
    }
}

#[cw_serde]
pub struct Config {
    /// Admin address that can update contract settings
    pub admin: Addr,
    /// Wormhole contract address for VAA verification
    pub wormhole_contract: Addr,
    /// Fee required to update the price feed (in Uint256 for CosmWasm 3.x)
    pub update_fee: Uint256,
    /// Pyth price feed ID for AKT/USD
    pub price_feed_id: String,
    /// Default data ID for price submissions (denom + base_denom)
    pub default_data_id: DataID,
    /// Valid Pyth data sources (emitter chain + address pairs)
    pub data_sources: Vec<DataSource>,
}

/// Cached oracle module parameters from the chain
/// These are fetched from chain and cached for validation
#[cw_serde]
pub struct CachedOracleParams {
    /// Maximum price deviation in basis points (e.g., 150 = 1.5%)
    pub max_price_deviation_bps: u64,
    /// Minimum number of price sources required
    pub min_price_sources: u32,
    /// Maximum price staleness in blocks
    pub max_price_staleness_blocks: i64,
    /// TWAP window in blocks
    pub twap_window: i64,
    /// Last block height when params were fetched
    pub last_updated_height: u64,
}

impl Default for CachedOracleParams {
    fn default() -> Self {
        Self {
            max_price_deviation_bps: 150,
            min_price_sources: 2,
            max_price_staleness_blocks: 50,
            twap_window: 50,
            last_updated_height: 0,
        }
    }
}

#[cw_serde]
pub struct PriceFeed {
    /// Symbol for the price feed (always "AKT/USD")
    pub symbol: String,
    /// Current price with decimals based on expo
    pub price: Uint128,
    /// Confidence interval
    pub conf: Uint128,
    /// Price exponent (typically -8 for 8 decimal places)
    pub expo: i32,
    /// Unix timestamp of current price publication
    pub publish_time: i64,
    /// Unix timestamp of previous price publication
    pub prev_publish_time: i64,
}

impl PriceFeed {
    pub fn new() -> Self {
        Self {
            symbol: "AKT/USD".to_string(),
            price: Uint128::zero(),
            conf: Uint128::zero(),
            expo: -8,
            publish_time: 0,
            prev_publish_time: 0,
        }
    }
}

impl Default for PriceFeed {
    fn default() -> Self {
        Self::new()
    }
}

/// Contract configuration storage
pub const CONFIG: Item<Config> = Item::new("config");

/// AKT/USD price feed storage
pub const PRICE_FEED: Item<PriceFeed> = Item::new("price_feed");

/// Cached oracle params from chain
pub const CACHED_ORACLE_PARAMS: Item<CachedOracleParams> = Item::new("cached_oracle_params");
