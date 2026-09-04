use cosmwasm_schema::{cw_serde, QueryResponses};
use cosmwasm_std::Binary;

#[cw_serde]
pub struct InstantiateMsg {
    pub admin: String,
    pub router_verifier: RouterVerifierConfigMsg,
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
    TransferAdmin { new_admin: String },
    SubmitVAA { vaa: Binary },
}

#[cw_serde]
pub struct MigrateMsg {}

#[cw_serde]
#[derive(QueryResponses)]
pub enum QueryMsg {
    #[returns(crate::vaa::ParsedVAA)]
    VerifyVAA { vaa: Binary, block_time: u64 },

    #[returns(ConfigResponse)]
    GetConfig {},
}

#[cw_serde]
pub struct ConfigResponse {
    pub admin: String,
    pub router_verifier: RouterVerifierConfigMsg,
}
