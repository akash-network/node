use cosmwasm_std::StdResult;
use sha3::{Digest, Keccak256};

use crate::{error::ContractError, msg::ParsedVAA};

pub const HEADER_LEN: usize = 6;
pub const SIGNATURE_LEN: usize = 66;
pub const SIG_DATA_POS: usize = 1;
pub const SIG_DATA_LEN: usize = 64;
pub const SIG_RECOVERY_POS: usize = SIG_DATA_POS + SIG_DATA_LEN;

const GUARDIAN_SET_INDEX_POS: usize = 1;
const LEN_SIGNER_POS: usize = 5;
const VAA_NONCE_POS: usize = 4;
const VAA_EMITTER_CHAIN_POS: usize = 8;
const VAA_EMITTER_ADDRESS_POS: usize = 10;
const VAA_SEQUENCE_POS: usize = 42;
const VAA_CONSISTENCY_LEVEL_POS: usize = 50;
const VAA_PAYLOAD_POS: usize = 51;

pub fn parse_vaa(data: &[u8]) -> StdResult<ParsedVAA> {
    if data.len() < HEADER_LEN {
        return ContractError::InvalidVAA.std_err();
    }

    let version = read_u8(data, 0)?;
    let guardian_set_index = read_u32(data, GUARDIAN_SET_INDEX_POS)?;
    let len_signers = read_u8(data, LEN_SIGNER_POS)?;
    let body_offset = HEADER_LEN
        .checked_add(
            SIGNATURE_LEN
                .checked_mul(len_signers as usize)
                .ok_or_else(|| {
                    cosmwasm_std::StdError::msg(ContractError::InvalidVAA.to_string())
                })?,
        )
        .ok_or_else(|| cosmwasm_std::StdError::msg(ContractError::InvalidVAA.to_string()))?;

    let payload_offset = body_offset
        .checked_add(VAA_PAYLOAD_POS)
        .ok_or_else(|| cosmwasm_std::StdError::msg(ContractError::InvalidVAA.to_string()))?;
    if body_offset >= data.len() || payload_offset > data.len() {
        return ContractError::InvalidVAA.std_err();
    }

    let body = &data[body_offset..];
    let first_hash = Keccak256::digest(body);
    let hash = Keccak256::digest(first_hash).to_vec();

    let emitter_address = read_array::<32>(data, body_offset + VAA_EMITTER_ADDRESS_POS)?.to_vec();

    Ok(ParsedVAA {
        version,
        guardian_set_index,
        timestamp: read_u32(data, body_offset)?,
        nonce: read_u32(data, body_offset + VAA_NONCE_POS)?,
        len_signers,
        emitter_chain: read_u16(data, body_offset + VAA_EMITTER_CHAIN_POS)?,
        emitter_address,
        sequence: read_u64(data, body_offset + VAA_SEQUENCE_POS)?,
        consistency_level: read_u8(data, body_offset + VAA_CONSISTENCY_LEVEL_POS)?,
        payload: data[payload_offset..].to_vec(),
        hash,
    })
}

fn read_array<const N: usize>(data: &[u8], index: usize) -> StdResult<[u8; N]> {
    let end = index
        .checked_add(N)
        .ok_or_else(|| cosmwasm_std::StdError::msg(ContractError::InvalidVAA.to_string()))?;
    let bytes = data
        .get(index..end)
        .ok_or_else(|| cosmwasm_std::StdError::msg(ContractError::InvalidVAA.to_string()))?;
    bytes
        .try_into()
        .map_err(|_| cosmwasm_std::StdError::msg(ContractError::InvalidVAA.to_string()))
}

fn read_u8(data: &[u8], index: usize) -> StdResult<u8> {
    data.get(index)
        .copied()
        .ok_or_else(|| cosmwasm_std::StdError::msg(ContractError::InvalidVAA.to_string()))
}

fn read_u16(data: &[u8], index: usize) -> StdResult<u16> {
    Ok(u16::from_be_bytes(read_array(data, index)?))
}

fn read_u32(data: &[u8], index: usize) -> StdResult<u32> {
    Ok(u32::from_be_bytes(read_array(data, index)?))
}

fn read_u64(data: &[u8], index: usize) -> StdResult<u64> {
    Ok(u64::from_be_bytes(read_array(data, index)?))
}
