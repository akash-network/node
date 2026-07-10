use cosmwasm_schema::{cw_serde, QueryResponses};
use cosmwasm_std::{Binary, Uint128, Uint256};

#[cw_serde]
pub struct InstantiateMsg {
    /// Address of the contract admin
    pub admin: String,
    /// Pyth VAA verifier contract address
    pub pyth_vaa_contract: String,
    /// Initial update fee in uakt (Uint256 for CosmWasm 3.x)
    pub update_fee: Uint256,
    /// Pyth price feed ID for AKT/USD (required)
    pub price_feed_id: String,
}

#[cw_serde]
pub enum ExecuteMsg {
    /// Update the AKT/USD price feed with upgraded Pyth PNAU data.
    /// pyth-vaa validates the embedded router-signed VAA before the price is relayed to x/oracle.
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
        pyth_vaa_contract: Option<String>,
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
    pub pyth_vaa_contract: String,
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
pub enum VaaQueryMsg {
    VerifyVAA { vaa: Binary, block_time: u64 },
}

#[cw_serde]
pub struct ParsedVAA {
    pub version: u8,
    pub guardian_set_index: u32,
    pub timestamp: u32,
    pub nonce: u32,
    pub len_signers: u8,
    pub emitter_chain: u16,
    pub emitter_address: Vec<u8>,
    pub sequence: u64,
    pub consistency_level: u8,
    pub payload: Vec<u8>,
    pub hash: Vec<u8>,
}
