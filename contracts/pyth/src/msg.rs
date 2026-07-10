use cosmwasm_schema::{cw_serde, QueryResponses};
use cosmwasm_std::{Binary, Uint128, Uint256};

#[cw_serde]
pub struct InstantiateMsg {
    /// Address of the contract admin
    pub admin: String,
    /// Upgraded Pyth router verifier configuration
    pub router_verifier: RouterVerifierConfigMsg,
    /// Initial update fee in uakt (Uint256 for CosmWasm 3.x)
    pub update_fee: Uint256,
    /// Pyth price feed ID for AKT/USD (required)
    pub price_feed_id: String,
}

#[cw_serde]
pub struct RouterVerifierConfigMsg {
    pub router_set_index: u32,
    pub routers: Vec<RouterAddress>,
    pub expected_emitter_chain: u16,
    pub expected_emitter_address: Binary,
}

#[cw_serde]
pub struct RouterAddress {
    pub bytes: Binary,
}

#[cw_serde]
pub enum ExecuteMsg {
    /// Update the AKT/USD price feed with upgraded Pyth PNAU data.
    /// The router verifier validates the embedded VAA before the price is relayed to x/oracle.
    UpdatePriceFeed {
        /// PNAU update data from the upgraded Pyth Hermes API.
        vaa: Binary,
    },
    /// Update the update fee (admin only)
    UpdateFee { new_fee: Uint256 },
    /// Transfer admin rights (admin only)
    TransferAdmin { new_admin: String },
    /// Update contract configuration (admin only)
    UpdateConfig {
        router_verifier: Option<RouterVerifierConfigMsg>,
        price_feed_id: Option<String>,
    },
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
    pub router_verifier: RouterVerifierConfigMsg,
    pub update_fee: Uint256,
    pub price_feed_id: String,
    pub default_denom: String,
    pub default_base_denom: String,
}

#[cw_serde]
pub struct PriceFeedIdResponse {
    pub price_feed_id: String,
}

#[cw_serde]
pub struct MigrateMsg {
    pub router_verifier: Option<RouterVerifierConfigMsg>,
}
