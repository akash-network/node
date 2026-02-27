use cosmwasm_schema::{cw_serde, QueryResponses};
use cosmwasm_std::{Binary, Uint128, Uint256};

#[cw_serde]
pub struct InstantiateMsg {
    /// Address of the contract admin
    pub admin: String,
    /// Wormhole contract address for VAA verification
    pub wormhole_contract: String,
    /// Initial update fee in uakt (Uint256 for CosmWasm 3.x)
    pub update_fee: Uint256,
    /// Pyth price feed ID for AKT/USD
    /// If empty, will be fetched from chain oracle params
    pub price_feed_id: String,
    /// Valid Pyth data sources (emitter chain + address pairs)
    pub data_sources: Vec<DataSourceMsg>,
}

/// A data source identifies a valid price feed source (Pyth publisher)
#[cw_serde]
pub struct DataSourceMsg {
    /// Wormhole chain ID of the emitter (26 for Pythnet)
    pub emitter_chain: u16,
    /// Emitter address (32 bytes, hex encoded)
    pub emitter_address: String,
}

#[cw_serde]
pub enum ExecuteMsg {
    /// Update the AKT/USD price feed with VAA proof
    /// VAA is verified via Wormhole contract, then Pyth payload is parsed and relayed to x/oracle
    UpdatePriceFeed {
        /// VAA data from Pyth Hermes API (base64 encoded Binary)
        vaa: Binary,
    },
    /// Update the update fee (admin only)
    UpdateFee { new_fee: Uint256 },
    /// Transfer admin rights (admin only)
    TransferAdmin { new_admin: String },
    /// Refresh cached oracle params from chain (admin only)
    RefreshOracleParams {},
    /// Update contract configuration (admin only)
    UpdateConfig {
        wormhole_contract: Option<String>,
        price_feed_id: Option<String>,
        data_sources: Option<Vec<DataSourceMsg>>,
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

    /// Get cached oracle parameters
    #[returns(OracleParamsResponse)]
    GetOracleParams {},
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
    pub wormhole_contract: String,
    pub update_fee: Uint256,
    pub price_feed_id: String,
    pub default_denom: String,
    pub default_base_denom: String,
    pub data_sources: Vec<DataSourceMsg>,
}

#[cw_serde]
pub struct PriceFeedIdResponse {
    pub price_feed_id: String,
}

/// Response for GetOracleParams query
#[cw_serde]
pub struct OracleParamsResponse {
    pub max_price_deviation_bps: u64,
    pub min_price_sources: u32,
    pub max_price_staleness_blocks: i64,
    pub twap_window: i64,
    pub last_updated_height: u64,
}

#[cw_serde]
pub struct MigrateMsg {}
