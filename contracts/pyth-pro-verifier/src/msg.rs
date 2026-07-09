use cosmwasm_schema::{cw_serde, QueryResponses};
use cosmwasm_std::Binary;
pub use wormhole::state::ParsedVAA;

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
