use cosmwasm_schema::{cw_serde, QueryResponses};
use cosmwasm_std::{Binary, Coin};

#[allow(unused_imports)]
use crate::state::{GuardianAddress, GuardianSetInfo, ParsedVAA};

#[cw_serde]
pub struct InstantiateMsg {
    /// Governance chain ID (typically Solana = 1)
    pub gov_chain: u16,
    /// Governance contract address
    pub gov_address: Binary,
    /// Chain ID for this deployment
    pub chain_id: u16,
    /// Fee denomination
    pub fee_denom: String,
}

#[cw_serde]
pub enum ExecuteMsg {
    /// Submit a VAA for verification and execution
    SubmitVAA { vaa: Binary },
    /// Post a message (only in full mode)
    #[cfg(feature = "full")]
    PostMessage { message: Binary, nonce: u32 },
}

#[cw_serde]
#[derive(QueryResponses)]
pub enum QueryMsg {
    /// Get current guardian set info
    #[returns(GuardianSetInfoResponse)]
    GuardianSetInfo {},

    /// Verify a VAA without executing it
    #[returns(ParsedVAA)]
    VerifyVAA { vaa: Binary, block_time: u64 },

    /// Get contract state
    #[returns(GetStateResponse)]
    GetState {},

    /// Get address in hex format
    #[returns(GetAddressHexResponse)]
    QueryAddressHex { address: String },
}

#[cw_serde]
pub struct MigrateMsg {}

#[cw_serde]
pub struct GuardianSetInfoResponse {
    pub guardian_set_index: u32,
    pub addresses: Vec<GuardianAddress>,
}

#[cw_serde]
pub struct GetStateResponse {
    pub fee: Coin,
}

#[cw_serde]
pub struct GetAddressHexResponse {
    pub hex: String,
}
