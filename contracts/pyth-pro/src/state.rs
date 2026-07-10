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

#[cw_serde]
pub struct Config {
    /// Admin address that can update contract settings
    pub admin: Addr,
    /// Pyth VAA verifier contract address
    pub pyth_vaa_contract: Addr,
    /// Fee required to update the price feed (in Uint256 for CosmWasm 3.x)
    pub update_fee: Uint256,
    /// Pyth price feed ID for AKT/USD
    pub price_feed_id: String,
    /// Default data ID for price submissions (denom + base_denom)
    pub default_data_id: DataID,
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
