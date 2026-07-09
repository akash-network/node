use cosmwasm_schema::{cw_serde, QueryResponses};
use cosmwasm_std::Binary;

#[cw_serde]
pub struct InstantiateMsg {
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
pub struct MigrateMsg {}

#[cw_serde]
#[derive(QueryResponses)]
pub enum QueryMsg {
    #[returns(ParsedVAA)]
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
