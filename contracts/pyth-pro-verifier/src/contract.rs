use cosmwasm_std::{to_json_binary, Binary, Deps, DepsMut, Env, MessageInfo, Response, StdResult};
use k256::ecdsa::{RecoveryId, Signature, VerifyingKey};
use sha3::{Digest, Keccak256};

#[cfg(not(feature = "library"))]
use cosmwasm_std::entry_point;

use crate::{
    error::ContractError,
    msg::{InstantiateMsg, MigrateMsg, QueryMsg},
    state::{Config, CONFIG},
};
use wormhole::state::ParsedVAA;

const ROUTER_COUNT: usize = 5;
const ROUTER_QUORUM: usize = 3;

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn instantiate(
    deps: DepsMut,
    _env: Env,
    _info: MessageInfo,
    msg: InstantiateMsg,
) -> StdResult<Response> {
    if msg.routers.len() != ROUTER_COUNT {
        return ContractError::InvalidConfig.std_err();
    }

    let mut routers: Vec<Vec<u8>> = Vec::with_capacity(ROUTER_COUNT);
    for router in msg.routers {
        let bytes = router.bytes.as_slice();
        if bytes.len() != 20 {
            return ContractError::InvalidAddressLength.std_err();
        }
        if routers.iter().any(|existing| existing.as_slice() == bytes) {
            return ContractError::InvalidConfig.std_err();
        }
        routers.push(bytes.to_vec());
    }

    if msg.expected_emitter_address.len() != 32 {
        return ContractError::InvalidAddressLength.std_err();
    }
    let expected_emitter_address = msg.expected_emitter_address.to_vec();

    CONFIG.save(
        deps.storage,
        &Config {
            router_set_index: msg.router_set_index,
            routers,
            expected_emitter_chain: msg.expected_emitter_chain,
            expected_emitter_address,
        },
    )?;

    Ok(Response::default())
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn migrate(_deps: DepsMut, _env: Env, _msg: MigrateMsg) -> StdResult<Response> {
    Ok(Response::default())
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn query(deps: Deps, _env: Env, msg: QueryMsg) -> StdResult<Binary> {
    match msg {
        QueryMsg::VerifyVAA { vaa, block_time } => {
            to_json_binary(&query_verify_vaa(deps, vaa.as_slice(), block_time)?)
        }
    }
}

pub fn query_verify_vaa(deps: Deps, data: &[u8], _block_time: u64) -> StdResult<ParsedVAA> {
    let config = CONFIG.load(deps.storage)?;
    let vaa = ParsedVAA::deserialize(data)?;

    if vaa.version != 1 {
        return ContractError::InvalidVersion.std_err();
    }
    if vaa.guardian_set_index != config.router_set_index {
        return ContractError::InvalidRouterSetIndex.std_err();
    }
    if vaa.emitter_chain != config.expected_emitter_chain
        || vaa.emitter_address != config.expected_emitter_address
    {
        return ContractError::InvalidEmitter.std_err();
    }

    verify_router_signatures(&config, &vaa, data)?;
    Ok(vaa)
}

fn verify_router_signatures(config: &Config, vaa: &ParsedVAA, data: &[u8]) -> StdResult<()> {
    let signer_count = vaa.len_signers as usize;
    if signer_count < ROUTER_QUORUM {
        return ContractError::NoQuorum.std_err();
    }
    if signer_count > config.routers.len() {
        return ContractError::TooManySignatures.std_err();
    }

    let mut last_index: i16 = -1;
    let mut pos = ParsedVAA::HEADER_LEN;
    for _ in 0..signer_count {
        if pos + ParsedVAA::SIGNATURE_LEN > data.len() {
            return ContractError::InvalidVAA.std_err();
        }

        let router_index = data[pos] as i16;
        if router_index <= last_index {
            return ContractError::WrongRouterIndexOrder.std_err();
        }
        last_index = router_index;

        let router_index = router_index as usize;
        if router_index >= config.routers.len() {
            return ContractError::TooManySignatures.std_err();
        }

        let signature = Signature::try_from(
            &data[pos + ParsedVAA::SIG_DATA_POS
                ..pos + ParsedVAA::SIG_DATA_POS + ParsedVAA::SIG_DATA_LEN],
        )
        .map_err(|_| {
            cosmwasm_std::StdError::msg(ContractError::CannotDecodeSignature.to_string())
        })?;
        let recovery_id =
            RecoveryId::try_from(data[pos + ParsedVAA::SIG_RECOVERY_POS]).map_err(|_| {
                cosmwasm_std::StdError::msg(ContractError::CannotDecodeSignature.to_string())
            })?;
        let verify_key =
            VerifyingKey::recover_from_prehash(vaa.hash.as_slice(), &signature, recovery_id)
                .map_err(|_| {
                    cosmwasm_std::StdError::msg(ContractError::CannotRecoverKey.to_string())
                })?;

        if router_address(&verify_key) != config.routers[router_index] {
            return ContractError::RouterSignatureError.std_err();
        }

        pos += ParsedVAA::SIGNATURE_LEN;
    }

    Ok(())
}

fn router_address(key: &VerifyingKey) -> Vec<u8> {
    let point = key.to_encoded_point(false);
    let hash = Keccak256::digest(&point.as_bytes()[1..]);
    hash[12..].to_vec()
}
