use cosmwasm_schema::{cw_serde, QueryResponses};
use cosmwasm_std::{Uint128, Uint256};

#[cw_serde]
pub struct InstantiateMsg {
    /// Address of the contract admin
    pub admin: String,
    /// Initial update fee in uakt (Uint256 for CosmWasm 3.x)
    pub update_fee: Uint256,
    /// Pyth price feed ID for AKT/USD
    /// If empty, will be fetched from chain oracle params
    pub price_feed_id: String,
}

#[cw_serde]
pub enum ExecuteMsg {
    /// Update the AKT/USD price feed
    UpdatePriceFeed {
        price: Uint128,
        conf: Uint128,
        expo: i32,
        publish_time: i64,
    },
    /// Update the update fee (admin only)
    UpdateFee { new_fee: Uint256 },
    /// Transfer admin rights (admin only)
    TransferAdmin { new_admin: String },
}

#[cw_serde]
#[derive(QueryResponses)]
pub enum QueryMsg {
    /// Get the current AKT/USD price
    #[returns(PriceResponse)]
    GetPrice {},

    /// Get the current AKT/USD price with metadata
    #[returns(PriceFeedResponse)]
    GetPriceFeed {},

    /// Get contract configuration
    #[returns(ConfigResponse)]
    GetConfig {},

    /// Get the Pyth price feed ID
    #[returns(PriceFeedIdResponse)]
    GetPriceFeedId {},
}

#[cw_serde]
pub struct PriceResponse {
    pub price: Uint128,
    pub conf: Uint128,
    pub expo: i32,
    pub publish_time: i64,
}

#[cw_serde]
pub struct PriceFeedResponse {
    pub symbol: String,
    pub price: Uint128,
    pub conf: Uint128,
    pub expo: i32,
    pub publish_time: i64,
    pub prev_publish_time: i64,
}

#[cw_serde]
pub struct ConfigResponse {
    pub admin: String,
    pub update_fee: Uint256,
    pub price_feed_id: String,
}

#[cw_serde]
pub struct PriceFeedIdResponse {
    pub price_feed_id: String,
}

#[cw_serde]
pub struct MigrateMsg {}
