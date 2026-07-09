use cosmwasm_schema::cw_serde;
use cw_storage_plus::Item;

#[cw_serde]
pub struct Config {
    pub router_set_index: u32,
    pub routers: Vec<Vec<u8>>,
    pub expected_emitter_chain: u16,
    pub expected_emitter_address: Vec<u8>,
}

pub const CONFIG: Item<Config> = Item::new("config");
