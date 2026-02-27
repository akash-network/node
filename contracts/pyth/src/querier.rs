use cosmwasm_schema::cw_serde;
use cosmwasm_std::{CustomQuery, QuerierWrapper, StdResult};

/// Custom query type for Akash chain queries
#[cw_serde]
pub enum AkashQuery {
    /// Query oracle module parameters
    OracleParams {},
}

impl CustomQuery for AkashQuery {}

/// Response for oracle params query
#[cw_serde]
pub struct OracleParamsResponse {
    pub params: OracleParams,
}

/// PythContractParams contains configuration for Pyth price feeds
/// Matches proto: akash.oracle.v1.PythContractParams
#[cw_serde]
pub struct PythContractParams {
    /// Pyth price feed ID for AKT/USD
    pub akt_price_feed_id: String,
}

/// Oracle module parameters
/// Matches proto: akash.oracle.v1.Params
#[cw_serde]
pub struct OracleParams {
    /// Source addresses allowed to write prices (contract addresses)
    #[serde(default)]
    pub sources: Vec<String>,
    /// Minimum number of price sources required (default: 2)
    #[serde(default = "default_min_price_sources")]
    pub min_price_sources: u32,
    /// Maximum price staleness in blocks (default: 50)
    #[serde(default = "default_max_price_staleness_blocks")]
    pub max_price_staleness_blocks: i64,
    /// TWAP window in blocks (default: 50)
    #[serde(default = "default_twap_window")]
    pub twap_window: i64,
    /// Maximum price deviation in basis points (default: 150 = 1.5%)
    #[serde(default = "default_max_price_deviation_bps")]
    pub max_price_deviation_bps: u64,
    /// Pyth-specific configuration (extracted from feed_contracts_params Any)
    #[serde(default)]
    pub pyth_params: Option<PythContractParams>,
}

fn default_min_price_sources() -> u32 {
    2
}

fn default_max_price_staleness_blocks() -> i64 {
    50
}

fn default_twap_window() -> i64 {
    50
}

fn default_max_price_deviation_bps() -> u64 {
    150
}

impl Default for OracleParams {
    fn default() -> Self {
        Self {
            sources: vec![],
            min_price_sources: default_min_price_sources(),
            max_price_staleness_blocks: default_max_price_staleness_blocks(),
            twap_window: default_twap_window(),
            max_price_deviation_bps: default_max_price_deviation_bps(),
            pyth_params: None,
        }
    }
}

impl OracleParams {
    /// Get AKT price feed ID from pyth_params
    pub fn get_akt_price_feed_id(&self) -> Option<&str> {
        self.pyth_params
            .as_ref()
            .map(|p| p.akt_price_feed_id.as_str())
            .filter(|id| !id.is_empty())
    }
}

/// Extension trait for querying Akash-specific data
pub trait AkashQuerier {
    fn query_oracle_params(&self) -> StdResult<OracleParamsResponse>;
}

impl<'a> AkashQuerier for QuerierWrapper<'a, AkashQuery> {
    fn query_oracle_params(&self) -> StdResult<OracleParamsResponse> {
        self.query(&AkashQuery::OracleParams {}.into())
    }
}
