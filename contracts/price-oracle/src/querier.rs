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

/// Oracle module parameters
#[cw_serde]
pub struct OracleParams {
    /// Pyth price feed ID for AKT/USD
    pub akt_price_feed_id: String,
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
