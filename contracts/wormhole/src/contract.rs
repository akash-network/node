use std::ops::Deref;

use cosmwasm_std::{
    entry_point, to_json_binary, Binary, Coin, CosmosMsg, Deps, DepsMut, Env,
    MessageInfo, QuerierWrapper, Response, StdError, StdResult, Storage, Uint256, WasmMsg,
};

use crate::{
    byte_utils::{extend_address_to_32, ByteUtils},
    error::ContractError,
    msg::{
        ExecuteMsg, GetAddressHexResponse, GetStateResponse, GuardianSetInfoResponse,
        InstantiateMsg, MigrateMsg, QueryMsg,
    },
    querier::{AkashQuerier, AkashQuery},
    state::{
        ConfigInfo, ContractUpgrade, GovernancePacket, GuardianAddress, GuardianSetInfo,
        ParsedVAA, SetFee, CONFIG, SEQUENCES, VAA_ARCHIVE,
    },
};

use k256::ecdsa::{RecoveryId, Signature, VerifyingKey};
use sha3::{Digest, Keccak256};

// Lock assets fee amount
const FEE_AMOUNT: u128 = 0;

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn instantiate(
    deps: DepsMut,
    _env: Env,
    _info: MessageInfo,
    msg: InstantiateMsg,
) -> StdResult<Response> {
    let state = ConfigInfo {
        gov_chain: msg.gov_chain,
        gov_address: msg.gov_address.to_vec(),
        fee: Coin::new(Uint256::from(FEE_AMOUNT), &msg.fee_denom),
        chain_id: msg.chain_id,
        fee_denom: msg.fee_denom.clone(),
    };
    CONFIG.save(deps.storage, &state)?;

    Ok(Response::new()
        .add_attribute("action", "instantiate"))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn execute(deps: DepsMut, env: Env, info: MessageInfo, msg: ExecuteMsg) -> StdResult<Response> {
    match msg {
        #[cfg(feature = "full")]
        ExecuteMsg::PostMessage { message, nonce } => {
            handle_post_message(deps, env, info, message.as_slice(), nonce)
        }
        ExecuteMsg::SubmitVAA { vaa } => handle_submit_vaa(deps, env, info, vaa.as_slice()),
        #[cfg(not(feature = "full"))]
        _ => Err(StdError::msg("Invalid during shutdown mode")),
    }
}

fn handle_submit_vaa(
    deps: DepsMut,
    env: Env,
    _info: MessageInfo,
    data: &[u8],
) -> StdResult<Response> {
    let state = CONFIG.load(deps.storage)?;

    // Always use oracle-based guardian set from x/oracle params
    let querier: QuerierWrapper<AkashQuery> = QuerierWrapper::new(deps.querier.deref());
    let vaa = parse_and_verify_vaa(
        deps.storage,
        &querier,
        data,
        env.block.time.seconds(),
    )?;

    VAA_ARCHIVE.save(deps.storage, vaa.hash.as_slice(), &true)?;

    if state.gov_chain == vaa.emitter_chain && state.gov_address == vaa.emitter_address {
        return handle_governance_payload(deps, env, &vaa.payload);
    }

    ContractError::InvalidVAAAction.std_err()
}

fn handle_governance_payload(deps: DepsMut, env: Env, data: &[u8]) -> StdResult<Response> {
    let gov_packet = GovernancePacket::deserialize(data)?;
    let state = CONFIG.load(deps.storage)?;

    let module = String::from_utf8(gov_packet.module).unwrap();
    let module: String = module.chars().filter(|c| c != &'\0').collect();

    if module != "Core" {
        return Err(StdError::msg("this is not a valid module"));
    }

    if gov_packet.chain != 0 && gov_packet.chain != state.chain_id {
        return Err(StdError::msg(
            "the governance VAA is for another chain",
        ));
    }

    match gov_packet.action {
        1u8 => vaa_update_contract(deps, env, &gov_packet.payload),
        // Guardian set updates (action 2) are handled via Akash governance, not Wormhole governance
        #[cfg(feature = "full")]
        3u8 => handle_set_fee(deps, env, &gov_packet.payload),
        _ => ContractError::InvalidVAAAction.std_err(),
    }
}

/// Parse and verify VAA using guardian set from x/oracle params.
fn parse_and_verify_vaa(
    storage: &dyn Storage,
    querier: &QuerierWrapper<AkashQuery>,
    data: &[u8],
    _block_time: u64,
) -> StdResult<ParsedVAA> {
    let vaa = ParsedVAA::deserialize(data)?;

    if vaa.version != 1 {
        return ContractError::InvalidVersion.std_err();
    }

    if VAA_ARCHIVE.may_load(storage, vaa.hash.as_slice())?.unwrap_or(false) {
        return ContractError::VaaAlreadyExecuted.std_err();
    }

    // Get guardian set from x/oracle params (only source)
    let guardian_set = querier.query_guardian_set()
        .map_err(|e| StdError::msg(format!("failed to query guardian set from oracle: {}", e)))?
        .to_guardian_set_info();

    if guardian_set.addresses.is_empty() {
        return Err(StdError::msg("no guardian addresses configured in oracle params"));
    }

    // Oracle-provided guardian sets don't expire (managed by Akash governance)
    verify_vaa_signatures(&vaa, data, &guardian_set)?;

    Ok(vaa)
}

/// Verify VAA signatures against the provided guardian set.
/// Extracted to share logic between stored and oracle-based verification.
fn verify_vaa_signatures(
    vaa: &ParsedVAA,
    data: &[u8],
    guardian_set: &GuardianSetInfo,
) -> StdResult<()> {
    if (vaa.len_signers as usize) < guardian_set.quorum() {
        return ContractError::NoQuorum.std_err();
    }

    // Verify guardian signatures
    let mut last_index: i32 = -1;
    let mut pos = ParsedVAA::HEADER_LEN;
    let data_ref: &[u8] = data;

    for _ in 0..vaa.len_signers {
        if pos + ParsedVAA::SIGNATURE_LEN > data.len() {
            return ContractError::InvalidVAA.std_err();
        }

        let index = data_ref.get_u8(pos) as i32;
        if index <= last_index {
            return ContractError::WrongGuardianIndexOrder.std_err();
        }
        last_index = index;

        let sig_bytes = &data[pos + ParsedVAA::SIG_DATA_POS
            ..pos + ParsedVAA::SIG_DATA_POS + ParsedVAA::SIG_DATA_LEN];
        let recovery_id = data_ref.get_u8(pos + ParsedVAA::SIG_RECOVERY_POS);

        let signature = Signature::try_from(sig_bytes)
            .map_err(|_| StdError::msg("cannot decode signature"))?;

        let recovery_id = RecoveryId::try_from(recovery_id)
            .map_err(|_| StdError::msg("cannot decode recovery id"))?;

        let verify_key = VerifyingKey::recover_from_prehash(
            vaa.hash.as_slice(),
            &signature,
            recovery_id,
        )
        .map_err(|_| StdError::msg("cannot recover key"))?;

        let index = index as usize;
        if index >= guardian_set.addresses.len() {
            return ContractError::TooManySignatures.std_err();
        }

        if !keys_equal(&verify_key, &guardian_set.addresses[index]) {
            return ContractError::GuardianSignatureError.std_err();
        }

        pos += ParsedVAA::SIGNATURE_LEN;
    }

    Ok(())
}

fn vaa_update_contract(_deps: DepsMut, env: Env, data: &[u8]) -> StdResult<Response> {
    let ContractUpgrade { new_contract } = ContractUpgrade::deserialize(data)?;

    Ok(Response::new()
        .add_message(CosmosMsg::Wasm(WasmMsg::Migrate {
            contract_addr: env.contract.address.to_string(),
            new_code_id: new_contract,
            msg: to_json_binary(&MigrateMsg {})?,
        }))
        .add_attribute("action", "contract_upgrade"))
}

#[cfg(feature = "full")]
pub fn handle_set_fee(deps: DepsMut, _env: Env, data: &[u8]) -> StdResult<Response> {
    let mut state = CONFIG.load(deps.storage)?;
    let set_fee_msg = SetFee::deserialize(data, state.fee_denom.clone())?;

    state.fee = set_fee_msg.fee;
    CONFIG.save(deps.storage, &state)?;

    Ok(Response::new()
        .add_attribute("action", "fee_change")
        .add_attribute("new_fee.amount", state.fee.amount.to_string())
        .add_attribute("new_fee.denom", state.fee.denom))
}

#[cfg(feature = "full")]
fn handle_post_message(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    message: &[u8],
    nonce: u32,
) -> StdResult<Response> {
    let state = CONFIG.load(deps.storage)?;
    let fee = &state.fee;

    // Check fee - compare Uint256 values directly
    if !fee.amount.is_zero() {
        let sent = info.funds.iter()
            .find(|c| c.denom == fee.denom)
            .map(|c| c.amount)
            .unwrap_or(Uint256::zero());
        if sent < fee.amount {
            return ContractError::FeeTooLow.std_err();
        }
    }

    let emitter = extend_address_to_32(&deps.api.addr_canonicalize(info.sender.as_str())?);
    let sequence = SEQUENCES.may_load(deps.storage, emitter.as_slice())?.unwrap_or(0);
    SEQUENCES.save(deps.storage, emitter.as_slice(), &(sequence + 1))?;

    Ok(Response::new()
        .add_attribute("message.message", hex::encode(message))
        .add_attribute("message.sender", hex::encode(&emitter))
        .add_attribute("message.chain_id", state.chain_id.to_string())
        .add_attribute("message.nonce", nonce.to_string())
        .add_attribute("message.sequence", sequence.to_string())
        .add_attribute("message.block_time", env.block.time.seconds().to_string()))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn query(deps: Deps, _env: Env, msg: QueryMsg) -> StdResult<Binary> {
    match msg {
        QueryMsg::GuardianSetInfo {} => to_json_binary(&query_guardian_set_info(deps)?),
        QueryMsg::VerifyVAA { vaa, block_time } => {
            to_json_binary(&query_parse_and_verify_vaa(deps, vaa.as_slice(), block_time)?)
        }
        QueryMsg::GetState {} => to_json_binary(&query_state(deps)?),
        QueryMsg::QueryAddressHex { address } => to_json_binary(&query_address_hex(deps, &address)?),
    }
}

pub fn query_guardian_set_info(deps: Deps) -> StdResult<GuardianSetInfoResponse> {
    // Always get guardian set from x/oracle params
    let querier: QuerierWrapper<AkashQuery> = QuerierWrapper::new(deps.querier.deref());
    let response = querier.query_guardian_set()
        .map_err(|e| StdError::msg(format!("failed to query guardian set: {}", e)))?;

    let guardian_set = response.to_guardian_set_info();
    Ok(GuardianSetInfoResponse {
        // Index 0 indicates oracle-sourced guardian set
        guardian_set_index: 0,
        addresses: guardian_set.addresses,
    })
}

pub fn query_parse_and_verify_vaa(deps: Deps, data: &[u8], block_time: u64) -> StdResult<ParsedVAA> {
    // Always use oracle-based guardian set
    let querier: QuerierWrapper<AkashQuery> = QuerierWrapper::new(deps.querier.deref());
    parse_and_verify_vaa(deps.storage, &querier, data, block_time)
}

pub fn query_address_hex(deps: Deps, address: &str) -> StdResult<GetAddressHexResponse> {
    Ok(GetAddressHexResponse {
        hex: hex::encode(extend_address_to_32(&deps.api.addr_canonicalize(address)?)),
    })
}

pub fn query_state(deps: Deps) -> StdResult<GetStateResponse> {
    let state = CONFIG.load(deps.storage)?;
    Ok(GetStateResponse { fee: state.fee })
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn migrate(_deps: DepsMut, _env: Env, _msg: MigrateMsg) -> StdResult<Response> {
    Ok(Response::default())
}

#[allow(unused_imports)]
fn keys_equal(a: &VerifyingKey, b: &GuardianAddress) -> bool {
    use k256::elliptic_curve::sec1::ToEncodedPoint;

    let mut hasher = Keccak256::new();
    let point = a.to_encoded_point(false);
    hasher.update(&point.as_bytes()[1..]);
    let a_hash = &hasher.finalize()[12..];

    let b_bytes = b.bytes.as_slice();
    if a_hash.len() != b_bytes.len() {
        return false;
    }

    a_hash.iter().zip(b_bytes.iter()).all(|(ai, bi)| ai == bi)
}
