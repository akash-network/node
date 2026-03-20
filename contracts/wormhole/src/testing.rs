use cosmwasm_std::testing::{message_info, mock_dependencies, mock_env};
use cosmwasm_std::{from_json, Binary, Coin, Uint256};

use crate::contract::{instantiate, execute, query};
use crate::msg::{
    ExecuteMsg, GetAddressHexResponse, GetStateResponse, GuardianSetInfoResponse, InstantiateMsg,
    QueryMsg,
};
use crate::state::{GovernancePacket, GuardianAddress, GuardianSetInfo, ParsedVAA};

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

    // Empty set: quorum = 1 (never allow zero quorum)
    let empty = GuardianSetInfo {
        addresses: vec![],
        expiration_time: 0,
    };
    assert_eq!(empty.quorum(), 1);

    // 19 guardians (mainnet-like): quorum = 13
    let nineteen = GuardianSetInfo {
        addresses: (0..19u8)
            .map(|i| GuardianAddress { bytes: Binary::from(vec![i; 20]) })
            .collect(),
        expiration_time: 0,
    };
    assert_eq!(nineteen.quorum(), 13);
}

// ---------------------------------------------------------------------------
// H-1: ParsedVAA::deserialize rejects short input instead of panicking
// ---------------------------------------------------------------------------

#[test]
fn test_deserialize_vaa_empty_input() {
    let result = ParsedVAA::deserialize(&[]);
    assert!(result.is_err());
    assert!(result.unwrap_err().to_string().contains("InvalidVAA"));
}

#[test]
fn test_deserialize_vaa_short_input() {
    // 3 bytes — shorter than HEADER_LEN (6)
    let result = ParsedVAA::deserialize(&[0x01, 0x02, 0x03]);
    assert!(result.is_err());
    assert!(result.unwrap_err().to_string().contains("InvalidVAA"));
}

#[test]
fn test_deserialize_vaa_exactly_header_len() {
    // 6 bytes = HEADER_LEN, but body_offset will exceed data.len() with 0 signers
    // version=1, guardian_set_index=0, len_signers=0 → body_offset=6, data.len()=6
    // body_offset >= data.len() → InvalidVAA (existing check)
    let data = [0x01, 0x00, 0x00, 0x00, 0x00, 0x00];
    let result = ParsedVAA::deserialize(&data);
    assert!(result.is_err());
}

#[test]
fn test_short_vaa_via_execute() {
    let mut deps = mock_dependencies();
    let env = mock_env();
    let info = message_info(&deps.api.addr_make("creator"), &[]);
    let msg = mock_instantiate_msg(0);
    instantiate(deps.as_mut(), env.clone(), info.clone(), msg).unwrap();

    let short_vaa = Binary::from(vec![0x01, 0x02, 0x03]);
    let res = execute(
        deps.as_mut(),
        env,
        info,
        ExecuteMsg::SubmitVAA { vaa: short_vaa },
    );
    assert!(res.is_err());
    assert!(res.unwrap_err().to_string().contains("InvalidVAA"));
}

#[test]
fn test_short_vaa_via_query() {
    let mut deps = mock_dependencies();
    let env = mock_env();
    let info = message_info(&deps.api.addr_make("creator"), &[]);
    let msg = mock_instantiate_msg(0);
    instantiate(deps.as_mut(), env.clone(), info, msg).unwrap();

    let short_vaa = Binary::from(vec![0x01, 0x02, 0x03]);
    let res = query(
        deps.as_ref(),
        env,
        QueryMsg::VerifyVAA {
            vaa: short_vaa,
            block_time: 0,
        },
    );
    assert!(res.is_err());
    assert!(res.unwrap_err().to_string().contains("InvalidVAA"));
}

// ---------------------------------------------------------------------------
// M-3: Empty guardian set now requires quorum of 1 (blocks unsigned VAAs)
// ---------------------------------------------------------------------------

#[test]
fn test_empty_guardian_set_rejects_unsigned_vaa() {
    let mut deps = mock_dependencies();
    let env = mock_env();
    let info = message_info(&deps.api.addr_make("creator"), &[]);

    // Instantiate with an empty guardian set
    let msg = InstantiateMsg {
        gov_chain: 1,
        gov_address: Binary::from(hex::decode(GOV_ADDRESS).unwrap()),
        initial_guardian_set: GuardianSetInfo {
            addresses: vec![],
            expiration_time: 0,
        },
        guardian_set_index: 0,
        guardian_set_expirity: 86400,
        chain_id: 32,
        fee_denom: "uakt".to_string(),
    };
    instantiate(deps.as_mut(), env.clone(), info.clone(), msg).unwrap();

    // Build a minimal valid-structure VAA with 0 signatures:
    // version(1) + guardian_set_index(4) + len_signers(1) = 6 bytes header
    // body needs at least VAA_PAYLOAD_POS(51) bytes
    let mut vaa = Vec::new();
    vaa.push(0x01);                            // version
    vaa.extend_from_slice(&0u32.to_be_bytes()); // guardian_set_index = 0
    vaa.push(0x00);                            // len_signers = 0
    // body: 51 bytes minimum (timestamp + nonce + emitter_chain + emitter_address + sequence + consistency_level)
    vaa.extend_from_slice(&[0u8; 51]);

    let res = execute(
        deps.as_mut(),
        env,
        info,
        ExecuteMsg::SubmitVAA { vaa: Binary::from(vaa) },
    );
    assert!(res.is_err());
    assert!(res.unwrap_err().to_string().contains("NoQuorum"));
}

// ---------------------------------------------------------------------------
// M-4: GovernancePacket returns error on invalid UTF-8 instead of panicking
// ---------------------------------------------------------------------------

#[test]
fn test_governance_packet_invalid_utf8_module() {
    // 32-byte module with invalid UTF-8, followed by action + chain
    let mut data = vec![0xFF; 32]; // invalid UTF-8 bytes
    data.push(0x01);               // action
    data.extend_from_slice(&0u16.to_be_bytes()); // chain

    let packet = GovernancePacket::deserialize(&data);
    // Deserialization succeeds — it just stores raw bytes
    assert!(packet.is_ok());

    // The panic was in contract.rs where String::from_utf8().unwrap() was called.
    // Verify that from_utf8 on these bytes would fail:
    let packet = packet.unwrap();
    assert!(String::from_utf8(packet.module).is_err());
}

// ---------------------------------------------------------------------------
// M-6: GovernancePacket::deserialize rejects short input instead of panicking
// ---------------------------------------------------------------------------

#[test]
fn test_governance_packet_empty_input() {
    let result = GovernancePacket::deserialize(&[]);
    assert!(result.is_err());
    assert!(result.unwrap_err().to_string().contains("InvalidVAA"));
}

#[test]
fn test_governance_packet_short_input() {
    // 10 bytes — shorter than MIN_LEN (35)
    let result = GovernancePacket::deserialize(&[0u8; 10]);
    assert!(result.is_err());
    assert!(result.unwrap_err().to_string().contains("InvalidVAA"));
}

#[test]
fn test_governance_packet_exactly_min_len() {
    // Exactly 35 bytes — should succeed with an empty payload
    let mut data = vec![0u8; 32]; // module (all zeros = null-padded)
    data.push(0x02);               // action
    data.extend_from_slice(&0u16.to_be_bytes()); // chain = 0

    assert_eq!(data.len(), 35);
    let result = GovernancePacket::deserialize(&data);
    assert!(result.is_ok());
    let packet = result.unwrap();
    assert_eq!(packet.action, 2);
    assert_eq!(packet.chain, 0);
    assert!(packet.payload.is_empty());
}

#[test]
fn test_governance_packet_with_payload() {
    // 35 + 4 bytes of payload
    let mut data = vec![0u8; 32]; // module
    data.push(0x01);               // action
    data.extend_from_slice(&32u16.to_be_bytes()); // chain = 32
    data.extend_from_slice(&[0xDE, 0xAD, 0xBE, 0xEF]); // payload

    let result = GovernancePacket::deserialize(&data);
    assert!(result.is_ok());
    let packet = result.unwrap();
    assert_eq!(packet.action, 1);
    assert_eq!(packet.chain, 32);
    assert_eq!(packet.payload, vec![0xDE, 0xAD, 0xBE, 0xEF]);
}
