use cosmwasm_std::testing::{message_info, mock_dependencies, mock_env};
use cosmwasm_std::{from_json, Binary, Coin, Uint256};

use crate::contract::{instantiate, query};
use crate::msg::{
    GetAddressHexResponse, GetStateResponse, GuardianSetInfoResponse, InstantiateMsg, QueryMsg,
};
use crate::state::{GuardianAddress, GuardianSetInfo};

const GOV_ADDRESS: &str = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE";
const GUARDIAN_ADDR: &str = "54dbb737eac5007103e729e9ab7ce64a6850a310";

fn mock_guardian_set() -> GuardianSetInfo {
    GuardianSetInfo {
        addresses: vec![GuardianAddress {
            bytes: Binary::from(hex::decode(GUARDIAN_ADDR).unwrap()),
        }],
        expiration_time: 0,
    }
}

fn mock_instantiate_msg(guardian_set_index: u32) -> InstantiateMsg {
    InstantiateMsg {
        gov_chain: 1,
        gov_address: Binary::from(hex::decode(GOV_ADDRESS).unwrap()),
        initial_guardian_set: mock_guardian_set(),
        guardian_set_index,
        guardian_set_expirity: 86400,
        chain_id: 32,
        fee_denom: "uakt".to_string(),
    }
}

#[test]
fn test_instantiate() {
    let mut deps = mock_dependencies();
    let env = mock_env();
    let info = message_info(&deps.api.addr_make("creator"), &[]);

    let msg = mock_instantiate_msg(4);
    let res = instantiate(deps.as_mut(), env, info, msg).unwrap();
    assert_eq!(res.messages.len(), 0);
}

#[test]
fn test_query_guardian_set_info() {
    let mut deps = mock_dependencies();
    let env = mock_env();
    let info = message_info(&deps.api.addr_make("creator"), &[]);

    let msg = mock_instantiate_msg(4);
    instantiate(deps.as_mut(), env.clone(), info, msg).unwrap();

    let res = query(deps.as_ref(), env, QueryMsg::GuardianSetInfo {}).unwrap();
    let guardian_info: GuardianSetInfoResponse = from_json(res).unwrap();

    assert_eq!(guardian_info.guardian_set_index, 4);
    assert_eq!(guardian_info.addresses.len(), 1);
    assert_eq!(
        guardian_info.addresses[0].bytes,
        Binary::from(hex::decode(GUARDIAN_ADDR).unwrap())
    );
}

#[test]
fn test_query_guardian_set_info_index_zero() {
    let mut deps = mock_dependencies();
    let env = mock_env();
    let info = message_info(&deps.api.addr_make("creator"), &[]);

    let msg = mock_instantiate_msg(0);
    instantiate(deps.as_mut(), env.clone(), info, msg).unwrap();

    let res = query(deps.as_ref(), env, QueryMsg::GuardianSetInfo {}).unwrap();
    let guardian_info: GuardianSetInfoResponse = from_json(res).unwrap();

    assert_eq!(guardian_info.guardian_set_index, 0);
    assert_eq!(guardian_info.addresses.len(), 1);
}

#[test]
fn test_query_get_state() {
    let mut deps = mock_dependencies();
    let env = mock_env();
    let info = message_info(&deps.api.addr_make("creator"), &[]);

    let msg = mock_instantiate_msg(4);
    instantiate(deps.as_mut(), env.clone(), info, msg).unwrap();

    let res = query(deps.as_ref(), env, QueryMsg::GetState {}).unwrap();
    let state: GetStateResponse = from_json(res).unwrap();

    assert_eq!(
        state.fee,
        Coin::new(Uint256::zero(), "uakt")
    );
}

#[test]
fn test_query_address_hex() {
    let mut deps = mock_dependencies();
    let env = mock_env();
    let info = message_info(&deps.api.addr_make("creator"), &[]);

    let msg = mock_instantiate_msg(0);
    instantiate(deps.as_mut(), env.clone(), info, msg).unwrap();

    let addr = deps.api.addr_make("sender");
    let res = query(
        deps.as_ref(),
        env,
        QueryMsg::QueryAddressHex {
            address: addr.to_string(),
        },
    )
    .unwrap();
    let hex_resp: GetAddressHexResponse = from_json(res).unwrap();

    // The hex response should be a 64-char hex string (32 bytes zero-padded)
    assert_eq!(hex_resp.hex.len(), 64);
}

#[test]
fn test_instantiate_multiple_guardians() {
    let mut deps = mock_dependencies();
    let env = mock_env();
    let info = message_info(&deps.api.addr_make("creator"), &[]);

    let guardian_set = GuardianSetInfo {
        addresses: vec![
            GuardianAddress {
                bytes: Binary::from(hex::decode(GUARDIAN_ADDR).unwrap()),
            },
            GuardianAddress {
                bytes: Binary::from(hex::decode("beFA429d57cD18b7F8A4d91A2da9AB4AF05d0FBe").unwrap()),
            },
        ],
        expiration_time: 0,
    };

    let msg = InstantiateMsg {
        gov_chain: 1,
        gov_address: Binary::from(hex::decode(GOV_ADDRESS).unwrap()),
        initial_guardian_set: guardian_set,
        guardian_set_index: 4,
        guardian_set_expirity: 86400,
        chain_id: 32,
        fee_denom: "uakt".to_string(),
    };

    instantiate(deps.as_mut(), env.clone(), info, msg).unwrap();

    let res = query(deps.as_ref(), env, QueryMsg::GuardianSetInfo {}).unwrap();
    let guardian_info: GuardianSetInfoResponse = from_json(res).unwrap();

    assert_eq!(guardian_info.addresses.len(), 2);
    assert_eq!(guardian_info.guardian_set_index, 4);
}

#[test]
fn test_quorum_calculation() {
    // Single guardian: quorum = 1
    let single = GuardianSetInfo {
        addresses: vec![GuardianAddress {
            bytes: Binary::from(vec![0u8; 20]),
        }],
        expiration_time: 0,
    };
    assert_eq!(single.quorum(), 1);

    // 3 guardians: quorum = 2+1 = 3
    let three = GuardianSetInfo {
        addresses: vec![
            GuardianAddress { bytes: Binary::from(vec![0u8; 20]) },
            GuardianAddress { bytes: Binary::from(vec![1u8; 20]) },
            GuardianAddress { bytes: Binary::from(vec![2u8; 20]) },
        ],
        expiration_time: 0,
    };
    assert_eq!(three.quorum(), 3);

    // Empty set: quorum = 0
    let empty = GuardianSetInfo {
        addresses: vec![],
        expiration_time: 0,
    };
    assert_eq!(empty.quorum(), 0);

    // 19 guardians (mainnet-like): quorum = 13
    let nineteen = GuardianSetInfo {
        addresses: (0..19u8)
            .map(|i| GuardianAddress { bytes: Binary::from(vec![i; 20]) })
            .collect(),
        expiration_time: 0,
    };
    assert_eq!(nineteen.quorum(), 13);
}
