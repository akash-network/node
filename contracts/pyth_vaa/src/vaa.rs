use cosmwasm_schema::cw_serde;
use cosmwasm_std::{StdError, StdResult};
use sha3::{Digest, Keccak256};

use crate::error::ContractError;

/// Parsed router-signed VAA payload from upgraded Pyth updates.
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

impl ParsedVAA {
    pub const HEADER_LEN: usize = 6;
    pub const SIGNATURE_LEN: usize = 66;

    pub const GUARDIAN_SET_INDEX_POS: usize = 1;
    pub const LEN_SIGNER_POS: usize = 5;

    pub const VAA_NONCE_POS: usize = 4;
    pub const VAA_EMITTER_CHAIN_POS: usize = 8;
    pub const VAA_EMITTER_ADDRESS_POS: usize = 10;
    pub const VAA_SEQUENCE_POS: usize = 42;
    pub const VAA_CONSISTENCY_LEVEL_POS: usize = 50;
    pub const VAA_PAYLOAD_POS: usize = 51;

    pub const SIG_DATA_POS: usize = 1;
    pub const SIG_DATA_LEN: usize = 64;
    pub const SIG_RECOVERY_POS: usize = Self::SIG_DATA_POS + Self::SIG_DATA_LEN;

    pub fn deserialize(data: &[u8]) -> StdResult<Self> {
        if data.len() < Self::HEADER_LEN {
            return Err(invalid_vaa());
        }

        let version = data[0];
        let guardian_set_index = read_u32(data, Self::GUARDIAN_SET_INDEX_POS)?;
        let len_signers = data[Self::LEN_SIGNER_POS] as usize;
        let body_offset = Self::HEADER_LEN + Self::SIGNATURE_LEN * len_signers;

        if body_offset >= data.len() || body_offset + Self::VAA_PAYLOAD_POS > data.len() {
            return Err(invalid_vaa());
        }

        let body = &data[body_offset..];
        let hash = Keccak256::digest(Keccak256::digest(body)).to_vec();

        Ok(ParsedVAA {
            version,
            guardian_set_index,
            timestamp: read_u32(data, body_offset)?,
            nonce: read_u32(data, body_offset + Self::VAA_NONCE_POS)?,
            len_signers: len_signers as u8,
            emitter_chain: read_u16(data, body_offset + Self::VAA_EMITTER_CHAIN_POS)?,
            emitter_address: read_bytes(data, body_offset + Self::VAA_EMITTER_ADDRESS_POS, 32)?
                .to_vec(),
            sequence: read_u64(data, body_offset + Self::VAA_SEQUENCE_POS)?,
            consistency_level: data[body_offset + Self::VAA_CONSISTENCY_LEVEL_POS],
            payload: data[body_offset + Self::VAA_PAYLOAD_POS..].to_vec(),
            hash,
        })
    }
}

fn read_u16(data: &[u8], pos: usize) -> StdResult<u16> {
    let bytes: [u8; 2] = read_bytes(data, pos, 2)?
        .try_into()
        .map_err(|_| invalid_vaa())?;
    Ok(u16::from_be_bytes(bytes))
}

fn read_u32(data: &[u8], pos: usize) -> StdResult<u32> {
    let bytes: [u8; 4] = read_bytes(data, pos, 4)?
        .try_into()
        .map_err(|_| invalid_vaa())?;
    Ok(u32::from_be_bytes(bytes))
}

fn read_u64(data: &[u8], pos: usize) -> StdResult<u64> {
    let bytes: [u8; 8] = read_bytes(data, pos, 8)?
        .try_into()
        .map_err(|_| invalid_vaa())?;
    Ok(u64::from_be_bytes(bytes))
}

fn read_bytes(data: &[u8], pos: usize, len: usize) -> StdResult<&[u8]> {
    data.get(pos..pos + len).ok_or_else(invalid_vaa)
}

fn invalid_vaa() -> StdError {
    StdError::msg(ContractError::InvalidVAA.to_string())
}
