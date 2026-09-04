use cosmwasm_schema::cw_serde;
use cosmwasm_std::Addr;
use cw_storage_plus::{Item, Map};

#[cw_serde]
pub struct Config {
    pub admin: Addr,
    pub router_set_index: u32,
    pub expected_emitter_chain: u16,
    pub expected_emitter_address: Vec<u8>,
}

#[cw_serde]
pub struct LegacyConfig {
    pub admin: Addr,
    pub router_verifier: RouterVerifierConfig,
}

#[cw_serde]
pub struct RouterSet {
    pub routers: Vec<Vec<u8>>,
}

#[cw_serde]
pub struct RouterVerifierConfig {
    pub router_set_index: u32,
    pub routers: Vec<Vec<u8>>,
    pub expected_emitter_chain: u16,
    pub expected_emitter_address: Vec<u8>,
}

pub const CONFIG: Item<Config> = Item::new("config");
pub const LEGACY_CONFIG: Item<LegacyConfig> = Item::new("config");
pub const ROUTER_SETS: Map<u32, RouterSet> = Map::new("router_set");
