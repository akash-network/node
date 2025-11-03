use cosmwasm_schema::cw_serde;
use cosmwasm_std::{Binary, CustomQuery, QuerierWrapper, StdResult};

use crate::state::{GuardianAddress, GuardianSetInfo};

/// Custom query type for Akash chain queries
#[cw_serde]
pub enum AkashQuery {
    /// Query the Wormhole guardian set from x/oracle params
    GuardianSet {},
}

impl CustomQuery for AkashQuery {}

/// Response for guardian set query from x/oracle params.
/// Matches the Go type in x/wasm/bindings/akash_query.go
#[cw_serde]
pub struct GuardianSetResponse {
    /// List of guardian addresses (20 bytes each, base64 encoded)
    pub addresses: Vec<GuardianAddressResponse>,
    /// When this guardian set expires (0 = never)
    pub expiration_time: u64,
}

/// Guardian address in the response (base64 encoded Binary)
#[cw_serde]
pub struct GuardianAddressResponse {
    /// 20-byte guardian address, base64 encoded
    pub bytes: Binary,
}

impl GuardianSetResponse {
    /// Convert to GuardianSetInfo for use in VAA verification
    pub fn to_guardian_set_info(&self) -> GuardianSetInfo {
        GuardianSetInfo {
            addresses: self
                .addresses
                .iter()
                .map(|addr| GuardianAddress {
                    bytes: addr.bytes.clone(),
                })
                .collect(),
            expiration_time: self.expiration_time,
        }
    }
}

/// Extension trait for querying Akash-specific data
pub trait AkashQuerier {
    fn query_guardian_set(&self) -> StdResult<GuardianSetResponse>;
}

impl<'a> AkashQuerier for QuerierWrapper<'a, AkashQuery> {
    fn query_guardian_set(&self) -> StdResult<GuardianSetResponse> {
        self.query(&AkashQuery::GuardianSet {}.into())
    }
}

/// Query the guardian set from x/oracle params.
/// This allows the Wormhole contract to use guardian keys managed by Akash governance.
pub fn query_guardian_set_from_oracle(
    querier: &QuerierWrapper<AkashQuery>,
) -> StdResult<GuardianSetInfo> {
    let response = querier.query_guardian_set()?;
    Ok(response.to_guardian_set_info())
}
