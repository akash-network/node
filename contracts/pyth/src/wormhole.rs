use cosmwasm_schema::cw_serde;
use cosmwasm_std::Binary;

/// Wormhole contract query messages
#[cw_serde]
pub enum WormholeQueryMsg {
    /// Verify a VAA without executing it
    VerifyVAA {
        vaa: Binary,
        block_time: u64,
    },
}

/// Parsed VAA (Verified Action Approval) returned by Wormhole contract
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
