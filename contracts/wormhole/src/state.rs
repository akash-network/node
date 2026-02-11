use cosmwasm_schema::cw_serde;
use cosmwasm_std::{Binary, Coin, StdResult, Uint256};
use cw_storage_plus::{Item, Map};

use crate::byte_utils::ByteUtils;
use crate::error::ContractError;

/// Contract configuration
#[cw_serde]
pub struct ConfigInfo {
    /// Governance chain (typically Solana = 1)
    pub gov_chain: u16,
    /// Governance contract address
    pub gov_address: Vec<u8>,
    /// Message sending fee
    pub fee: Coin,
    /// Chain ID for this deployment
    pub chain_id: u16,
    /// Fee denomination
    pub fee_denom: String,
}

/// Parsed VAA (Verified Action Approval)
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
        use sha3::{Digest, Keccak256};

        let data_ref: &[u8] = data;
        let version = data_ref.get_u8(0);
        let guardian_set_index = data_ref.get_u32(Self::GUARDIAN_SET_INDEX_POS);
        let len_signers = data_ref.get_u8(Self::LEN_SIGNER_POS) as usize;
        let body_offset = Self::HEADER_LEN + Self::SIGNATURE_LEN * len_signers;

        if body_offset >= data.len() {
            return ContractError::InvalidVAA.std_err();
        }

        let body = &data[body_offset..];
        let mut hasher = Keccak256::new();
        hasher.update(body);
        let hash = hasher.finalize().to_vec();

        let mut hasher = Keccak256::new();
        hasher.update(&hash);
        let hash = hasher.finalize().to_vec();

        if body_offset + Self::VAA_PAYLOAD_POS > data.len() {
            return ContractError::InvalidVAA.std_err();
        }

        let timestamp = data_ref.get_u32(body_offset);
        let nonce = data_ref.get_u32(body_offset + Self::VAA_NONCE_POS);
        let emitter_chain = data_ref.get_u16(body_offset + Self::VAA_EMITTER_CHAIN_POS);
        let emitter_address = data_ref
            .get_bytes32(body_offset + Self::VAA_EMITTER_ADDRESS_POS)
            .to_vec();
        let sequence = data_ref.get_u64(body_offset + Self::VAA_SEQUENCE_POS);
        let consistency_level = data_ref.get_u8(body_offset + Self::VAA_CONSISTENCY_LEVEL_POS);
        let payload = data[body_offset + Self::VAA_PAYLOAD_POS..].to_vec();

        Ok(ParsedVAA {
            version,
            guardian_set_index,
            timestamp,
            nonce,
            len_signers: len_signers as u8,
            emitter_chain,
            emitter_address,
            sequence,
            consistency_level,
            payload,
            hash,
        })
    }
}

/// Guardian address (20 bytes, Ethereum-style)
#[cw_serde]
pub struct GuardianAddress {
    pub bytes: Binary,
}

#[cfg(test)]
impl GuardianAddress {
    pub fn from(string: &str) -> GuardianAddress {
        GuardianAddress {
            bytes: hex::decode(string).expect("Decoding failed").into(),
        }
    }
}

/// Guardian set information
#[cw_serde]
pub struct GuardianSetInfo {
    pub addresses: Vec<GuardianAddress>,
    pub expiration_time: u64,
}

impl GuardianSetInfo {
    pub fn quorum(&self) -> usize {
        if self.addresses.is_empty() {
            return 0;
        }
        ((self.addresses.len() * 10 / 3) * 2) / 10 + 1
    }
}

/// Governance packet structure
pub struct GovernancePacket {
    pub module: Vec<u8>,
    pub action: u8,
    pub chain: u16,
    pub payload: Vec<u8>,
}

impl GovernancePacket {
    pub fn deserialize(data: &[u8]) -> StdResult<Self> {
        let data_ref: &[u8] = data;
        let module = data_ref.get_bytes32(0).to_vec();
        let action = data_ref.get_u8(32);
        let chain = data_ref.get_u16(33);
        let payload = data[35..].to_vec();

        Ok(GovernancePacket {
            module,
            action,
            chain,
            payload,
        })
    }
}

/// Contract upgrade governance action
pub struct ContractUpgrade {
    pub new_contract: u64,
}

impl ContractUpgrade {
    pub fn deserialize(data: &[u8]) -> StdResult<Self> {
        let data_ref: &[u8] = data;
        let new_contract = data_ref.get_u64(24);
        Ok(ContractUpgrade { new_contract })
    }
}

/// Guardian set upgrade governance action
pub struct GuardianSetUpgrade {
    pub new_guardian_set_index: u32,
    pub new_guardian_set: GuardianSetInfo,
}

impl GuardianSetUpgrade {
    pub fn deserialize(data: &[u8]) -> StdResult<Self> {
        const ADDRESS_LEN: usize = 20;

        let data_ref: &[u8] = data;
        let new_guardian_set_index = data_ref.get_u32(0);
        let n_guardians = data_ref.get_u8(4);

        let mut addresses = vec![];
        for i in 0..n_guardians {
            let pos = 5 + (i as usize) * ADDRESS_LEN;
            if pos + ADDRESS_LEN > data.len() {
                return ContractError::InvalidVAA.std_err();
            }
            addresses.push(GuardianAddress {
                bytes: data[pos..pos + ADDRESS_LEN].to_vec().into(),
            });
        }

        let new_guardian_set = GuardianSetInfo {
            addresses,
            expiration_time: 0,
        };

        Ok(GuardianSetUpgrade {
            new_guardian_set_index,
            new_guardian_set,
        })
    }
}

/// Set fee governance action
pub struct SetFee {
    pub fee: Coin,
}

impl SetFee {
    pub fn deserialize(data: &[u8], fee_denom: String) -> StdResult<Self> {
        let data_ref: &[u8] = data;
        let (_, amount) = data_ref.get_u256(0);
        let fee = Coin {
            denom: fee_denom,
            amount: Uint256::from(amount),
        };
        Ok(SetFee { fee })
    }
}

// Storage items
pub const CONFIG: Item<ConfigInfo> = Item::new("config");
pub const SEQUENCES: Map<&[u8], u64> = Map::new("sequences");
pub const VAA_ARCHIVE: Map<&[u8], bool> = Map::new("vaa_archive");
