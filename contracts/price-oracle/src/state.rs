use cosmwasm_schema::cw_serde;
use cosmwasm_std::{Addr, Uint128, Uint256};
use cw_storage_plus::Item;

#[cw_serde]
pub struct Config {
    /// Admin address that can update contract settings
    pub admin: Addr,
    /// Fee required to update the price feed (in Uint256 for CosmWasm 3.x)
    pub update_fee: Uint256,
    /// Pyth price feed ID for AKT/USD
    pub price_feed_id: String,
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
