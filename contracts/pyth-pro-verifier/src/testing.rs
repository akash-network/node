use cosmwasm_std::testing::{message_info, mock_dependencies, mock_env};
use cosmwasm_std::{from_json, Binary};
use k256::ecdsa::{RecoveryId, Signature, SigningKey, VerifyingKey};
use sha3::{Digest, Keccak256};

use crate::{
    contract::{instantiate, query},
    msg::{InstantiateMsg, ParsedVAA, QueryMsg, RouterAddress},
};

const ROUTER_SET_INDEX: u32 = 7;
const EMITTER_CHAIN: u16 = 26;
const EMITTER_ADDRESS: [u8; 32] = [9u8; 32];
const PRODUCTION_ROUTER_SET_INDEX: u32 = 0;
const PRODUCTION_EMITTER_ADDRESS_HEX: &str =
    "507974686e6574507974686e6574507974686e6574507974686e657450797468";
const AKT_UPGRADED_HERMES_VAA_HEX: &str =
    "010000000003013e1fb8c03541656c8e9f8a3edc9e939c676a07d6e76a6481babf53b3ffe3eb354cea03c9beacc220c6c1b59039d732cd3403762a8eb8c005829ea93175cb6ce20003d2ed8e50e7d7350255ee6974a845c2f83236c9be4969f6a3063f1c87173fe1dc35e07fe6cea3d9341dc8887a858abacc33883acd2f1c6a9a94cfa3718ea8130901043dbc0d384f891945d9e6c8636826d58ad7f74f4162346052f37bfc5cf5da0ea75f1d1795acf9a8685f96308393947a0a9a22e381585e804506aa7c19d3400945006a4f27ed00000000001a507974686e6574507974686e6574507974686e6574507974686e657450797468000000084e2f1e84004155575600000000084e2f1e84000000000f57eee39f1b76403b1094b3a177ecef270e3226";

#[test]
fn verifies_three_of_five_router_signatures() {
    let keys = router_keys();
    let deps = setup(&keys);
    let payload = b"pyth-root".to_vec();
    let vaa = signed_vaa(
        &keys,
        &[0, 2, 4],
        ROUTER_SET_INDEX,
        EMITTER_CHAIN,
        EMITTER_ADDRESS,
        payload.clone(),
    );

    let res = query(
        deps.as_ref(),
        mock_env(),
        QueryMsg::VerifyVAA {
            vaa: Binary::from(vaa),
            block_time: 0,
        },
    )
    .unwrap();

    let parsed: ParsedVAA = from_json(res).unwrap();
    assert_eq!(parsed.guardian_set_index, ROUTER_SET_INDEX);
    assert_eq!(parsed.len_signers, 3);
    assert_eq!(parsed.emitter_chain, EMITTER_CHAIN);
    assert_eq!(parsed.emitter_address, EMITTER_ADDRESS);
    assert_eq!(parsed.payload, payload);
}

#[test]
fn rejects_two_of_five_router_signatures() {
    let keys = router_keys();
    let deps = setup(&keys);
    let vaa = signed_vaa(
        &keys,
        &[0, 1],
        ROUTER_SET_INDEX,
        EMITTER_CHAIN,
        EMITTER_ADDRESS,
        vec![],
    );

    let err = query(
        deps.as_ref(),
        mock_env(),
        QueryMsg::VerifyVAA {
            vaa: Binary::from(vaa),
            block_time: 0,
        },
    )
    .unwrap_err();

    assert!(err.to_string().contains("NoQuorum"));
}

#[test]
fn rejects_wrong_router_signature() {
    let keys = router_keys();
    let deps = setup(&keys);
    let outsider = SigningKey::from_bytes((&[42u8; 32]).into()).unwrap();
    let vaa = signed_vaa_with_keys(
        &[outsider, keys[1].clone(), keys[2].clone()],
        &[0, 1, 2],
        ROUTER_SET_INDEX,
        EMITTER_CHAIN,
        EMITTER_ADDRESS,
        vec![],
    );

    let err = query(
        deps.as_ref(),
        mock_env(),
        QueryMsg::VerifyVAA {
            vaa: Binary::from(vaa),
            block_time: 0,
        },
    )
    .unwrap_err();

    assert!(err.to_string().contains("RouterSignatureError"));
}

#[test]
fn rejects_wrong_emitter() {
    let keys = router_keys();
    let deps = setup(&keys);
    let vaa = signed_vaa(
        &keys,
        &[0, 1, 2],
        ROUTER_SET_INDEX,
        27,
        EMITTER_ADDRESS,
        vec![],
    );

    let err = query(
        deps.as_ref(),
        mock_env(),
        QueryMsg::VerifyVAA {
            vaa: Binary::from(vaa),
            block_time: 0,
        },
    )
    .unwrap_err();

    assert!(err.to_string().contains("InvalidEmitter"));
}

#[test]
fn rejects_wrong_router_set_index() {
    let keys = router_keys();
    let deps = setup(&keys);
    let vaa = signed_vaa(
        &keys,
        &[0, 1, 2],
        ROUTER_SET_INDEX + 1,
        EMITTER_CHAIN,
        EMITTER_ADDRESS,
        vec![],
    );

    let err = query(
        deps.as_ref(),
        mock_env(),
        QueryMsg::VerifyVAA {
            vaa: Binary::from(vaa),
            block_time: 0,
        },
    )
    .unwrap_err();

    assert!(err.to_string().contains("InvalidRouterSetIndex"));
}

#[test]
fn rejects_duplicate_router_index() {
    let keys = router_keys();
    let deps = setup(&keys);
    let vaa = signed_vaa_with_keys(
        &[keys[0].clone(), keys[0].clone(), keys[1].clone()],
        &[0, 0, 1],
        ROUTER_SET_INDEX,
        EMITTER_CHAIN,
        EMITTER_ADDRESS,
        vec![],
    );

    let err = query(
        deps.as_ref(),
        mock_env(),
        QueryMsg::VerifyVAA {
            vaa: Binary::from(vaa),
            block_time: 0,
        },
    )
    .unwrap_err();

    assert!(err.to_string().contains("WrongRouterIndexOrder"));
}

#[test]
fn rejects_duplicate_router_addresses_at_instantiate() {
    let keys = router_keys();
    let mut routers: Vec<RouterAddress> = keys.iter().map(router_address).collect();
    routers[1] = routers[0].clone();

    let mut deps = mock_dependencies();
    let info = message_info(&deps.api.addr_make("creator"), &[]);
    let err = instantiate(
        deps.as_mut(),
        mock_env(),
        info,
        InstantiateMsg {
            router_set_index: ROUTER_SET_INDEX,
            routers,
            expected_emitter_chain: EMITTER_CHAIN,
            expected_emitter_address: Binary::from(EMITTER_ADDRESS),
        },
    )
    .unwrap_err();

    assert!(err.to_string().contains("InvalidConfig"));
}

#[test]
fn verifies_live_upgraded_hermes_akt_vaa_fixture() {
    let deps = setup_production();
    let vaa = hex::decode(AKT_UPGRADED_HERMES_VAA_HEX).unwrap();

    let res = query(
        deps.as_ref(),
        mock_env(),
        QueryMsg::VerifyVAA {
            vaa: Binary::from(vaa),
            block_time: 0,
        },
    )
    .unwrap();

    let parsed: ParsedVAA = from_json(res).unwrap();
    assert_eq!(parsed.version, 1);
    assert_eq!(parsed.guardian_set_index, PRODUCTION_ROUTER_SET_INDEX);
    assert_eq!(parsed.len_signers, 3);
    assert_eq!(parsed.emitter_chain, EMITTER_CHAIN);
    assert_eq!(
        parsed.emitter_address,
        hex::decode(PRODUCTION_EMITTER_ADDRESS_HEX).unwrap()
    );
    assert_eq!(&parsed.payload[..4], b"AUWV");
}

fn setup(
    keys: &[SigningKey],
) -> cosmwasm_std::OwnedDeps<
    cosmwasm_std::testing::MockStorage,
    cosmwasm_std::testing::MockApi,
    cosmwasm_std::testing::MockQuerier,
> {
    let mut deps = mock_dependencies();
    let info = message_info(&deps.api.addr_make("creator"), &[]);
    instantiate(
        deps.as_mut(),
        mock_env(),
        info,
        InstantiateMsg {
            router_set_index: ROUTER_SET_INDEX,
            routers: keys.iter().map(router_address).collect(),
            expected_emitter_chain: EMITTER_CHAIN,
            expected_emitter_address: Binary::from(EMITTER_ADDRESS),
        },
    )
    .unwrap();
    deps
}

fn setup_production() -> cosmwasm_std::OwnedDeps<
    cosmwasm_std::testing::MockStorage,
    cosmwasm_std::testing::MockApi,
    cosmwasm_std::testing::MockQuerier,
> {
    let mut deps = mock_dependencies();
    let info = message_info(&deps.api.addr_make("creator"), &[]);
    instantiate(
        deps.as_mut(),
        mock_env(),
        info,
        InstantiateMsg {
            router_set_index: PRODUCTION_ROUTER_SET_INDEX,
            routers: [
                "41534bb176e461a3fb30479400f210549ecce638",
                "6502987b62f21cab7eb5ccd8f0173084b60d5b41",
                "44a3e8f6a382412cf6bb90a3f8106e68977476c9",
                "d9d7d4529577864352c9a6539a48238fcd447052",
                "1663a5a822336ece48559b1dfb1e93a017a7dac3",
            ]
            .iter()
            .map(|addr| RouterAddress {
                bytes: Binary::from(hex::decode(addr).unwrap()),
            })
            .collect(),
            expected_emitter_chain: EMITTER_CHAIN,
            expected_emitter_address: Binary::from(
                hex::decode(PRODUCTION_EMITTER_ADDRESS_HEX).unwrap(),
            ),
        },
    )
    .unwrap();
    deps
}

fn router_keys() -> Vec<SigningKey> {
    (1u8..=5)
        .map(|i| SigningKey::from_bytes((&[i; 32]).into()).unwrap())
        .collect()
}

fn router_address(key: &SigningKey) -> RouterAddress {
    RouterAddress {
        bytes: Binary::from(address_from_key(key.verifying_key())),
    }
}

fn address_from_key(key: &VerifyingKey) -> [u8; 20] {
    let point = key.to_encoded_point(false);
    let hash = Keccak256::digest(&point.as_bytes()[1..]);
    hash[12..].try_into().unwrap()
}

fn signed_vaa(
    keys: &[SigningKey],
    signer_indexes: &[u8],
    router_set_index: u32,
    emitter_chain: u16,
    emitter_address: [u8; 32],
    payload: Vec<u8>,
) -> Vec<u8> {
    let signing_keys: Vec<SigningKey> = signer_indexes
        .iter()
        .map(|index| keys[*index as usize].clone())
        .collect();
    signed_vaa_with_keys(
        &signing_keys,
        signer_indexes,
        router_set_index,
        emitter_chain,
        emitter_address,
        payload,
    )
}

fn signed_vaa_with_keys(
    signing_keys: &[SigningKey],
    signer_indexes: &[u8],
    router_set_index: u32,
    emitter_chain: u16,
    emitter_address: [u8; 32],
    payload: Vec<u8>,
) -> Vec<u8> {
    let body = vaa_body(emitter_chain, emitter_address, payload);
    let hash = body_hash(&body);

    let mut vaa = vec![1u8];
    vaa.extend_from_slice(&router_set_index.to_be_bytes());
    vaa.push(signer_indexes.len() as u8);

    for (key, index) in signing_keys.iter().zip(signer_indexes.iter()) {
        let (signature, recovery_id) = sign_hash(key, &hash);
        vaa.push(*index);
        let signature_bytes = signature.to_bytes();
        vaa.extend_from_slice(&signature_bytes);
        vaa.push(recovery_id.to_byte());
    }

    vaa.extend_from_slice(&body);
    vaa
}

fn vaa_body(emitter_chain: u16, emitter_address: [u8; 32], payload: Vec<u8>) -> Vec<u8> {
    let mut body = Vec::new();
    body.extend_from_slice(&123u32.to_be_bytes());
    body.extend_from_slice(&456u32.to_be_bytes());
    body.extend_from_slice(&emitter_chain.to_be_bytes());
    body.extend_from_slice(&emitter_address);
    body.extend_from_slice(&789u64.to_be_bytes());
    body.push(0);
    body.extend_from_slice(&payload);
    body
}

fn body_hash(body: &[u8]) -> [u8; 32] {
    let first = Keccak256::digest(body);
    Keccak256::digest(first).into()
}

fn sign_hash(key: &SigningKey, hash: &[u8; 32]) -> (Signature, RecoveryId) {
    key.sign_prehash_recoverable(hash).unwrap()
}
