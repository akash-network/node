use cosmwasm_std::Binary;
use k256::ecdsa::{RecoveryId, Signature, VerifyingKey};
use sha3::{Digest, Keccak256};

use crate::{
    error::ContractError,
    msg::{RouterAddress, RouterVerifierConfigMsg},
    state::RouterVerifierConfig,
    vaa::ParsedVAA,
};

const ROUTER_COUNT: usize = 5;
const ROUTER_QUORUM: usize = 3;
const ROUTER_ADDRESS_LEN: usize = 20;
const GOVERNANCE_PACKET_LEN: usize = 35;
const GOVERNANCE_MODULE_LEN: usize = 32;
const GOVERNANCE_ACTION_POS: usize = 32;
const GOVERNANCE_TARGET_CHAIN_POS: usize = 33;
const GOVERNANCE_PAYLOAD_POS: usize = 35;
const GOVERNANCE_ACTION_ROUTER_SET_UPGRADE: u8 = 2;
const GOVERNANCE_TARGET_CHAIN_GLOBAL: u16 = 0;

pub struct RouterSetUpdate {
    pub router_set_index: u32,
    pub routers: Vec<Vec<u8>>,
}

pub fn parse_config(msg: RouterVerifierConfigMsg) -> Result<RouterVerifierConfig, ContractError> {
    if msg.routers.len() != ROUTER_COUNT {
        return Err(ContractError::InvalidConfig);
    }

    let mut routers: Vec<Vec<u8>> = Vec::with_capacity(ROUTER_COUNT);
    for router in msg.routers {
        push_router_address(&mut routers, router.bytes.as_slice())?;
    }

    if msg.expected_emitter_address.len() != 32 {
        return Err(ContractError::InvalidAddressLength);
    }
    let expected_emitter_address = msg.expected_emitter_address.to_vec();

    Ok(RouterVerifierConfig {
        router_set_index: msg.router_set_index,
        routers,
        expected_emitter_chain: msg.expected_emitter_chain,
        expected_emitter_address,
    })
}

pub fn config_to_msg(config: &RouterVerifierConfig) -> RouterVerifierConfigMsg {
    RouterVerifierConfigMsg {
        router_set_index: config.router_set_index,
        routers: config
            .routers
            .iter()
            .map(|router| RouterAddress {
                bytes: Binary::from(router.clone()),
            })
            .collect(),
        expected_emitter_chain: config.expected_emitter_chain,
        expected_emitter_address: Binary::from(config.expected_emitter_address.clone()),
    }
}

pub fn parse_router_set_update(
    data: &[u8],
    governance_target_chain: u16,
) -> Result<RouterSetUpdate, ContractError> {
    let data = parse_governance_router_set_update(data, governance_target_chain)?;

    if data.len() < 5 {
        return Err(ContractError::InvalidRouterSetUpdate);
    }

    let router_set_index = u32::from_be_bytes(
        data[0..4]
            .try_into()
            .map_err(|_| ContractError::InvalidRouterSetUpdate)?,
    );
    let router_count = data[4] as usize;
    if router_count != ROUTER_COUNT {
        return Err(ContractError::InvalidConfig);
    }

    let expected_len = 5 + router_count * ROUTER_ADDRESS_LEN;
    if data.len() != expected_len {
        return Err(ContractError::InvalidRouterSetUpdate);
    }

    let mut routers = Vec::with_capacity(router_count);
    for i in 0..router_count {
        let pos = 5 + i * ROUTER_ADDRESS_LEN;
        push_router_address(&mut routers, &data[pos..pos + ROUTER_ADDRESS_LEN])?;
    }

    Ok(RouterSetUpdate {
        router_set_index,
        routers,
    })
}

fn parse_governance_router_set_update(
    data: &[u8],
    governance_target_chain: u16,
) -> Result<&[u8], ContractError> {
    if data.len() < GOVERNANCE_PACKET_LEN {
        return Err(ContractError::InvalidRouterSetUpdate);
    }

    let module = String::from_utf8(data[..GOVERNANCE_MODULE_LEN].to_vec())
        .map_err(|_| ContractError::InvalidVAAAction)?;
    let module = module.trim_matches(char::from(0));
    if module != "Core" {
        return Err(ContractError::InvalidVAAAction);
    }

    if data[GOVERNANCE_ACTION_POS] != GOVERNANCE_ACTION_ROUTER_SET_UPGRADE {
        return Err(ContractError::InvalidVAAAction);
    }

    let target_chain = u16::from_be_bytes(
        data[GOVERNANCE_TARGET_CHAIN_POS..GOVERNANCE_PAYLOAD_POS]
            .try_into()
            .map_err(|_| ContractError::InvalidRouterSetUpdate)?,
    );
    if target_chain != GOVERNANCE_TARGET_CHAIN_GLOBAL && target_chain != governance_target_chain {
        return Err(ContractError::InvalidGovernanceTarget);
    }

    Ok(&data[GOVERNANCE_PAYLOAD_POS..])
}

pub fn verify_vaa(config: &RouterVerifierConfig, data: &[u8]) -> Result<ParsedVAA, ContractError> {
    let vaa = ParsedVAA::deserialize(data)?;

    if vaa.version != 1 {
        return Err(ContractError::InvalidVersion);
    }
    if vaa.guardian_set_index != config.router_set_index {
        return Err(ContractError::InvalidRouterSetIndex);
    }
    if vaa.emitter_chain != config.expected_emitter_chain
        || vaa.emitter_address != config.expected_emitter_address
    {
        return Err(ContractError::InvalidEmitter);
    }

    verify_router_signatures(config, &vaa, data)?;
    Ok(vaa)
}

fn verify_router_signatures(
    config: &RouterVerifierConfig,
    vaa: &ParsedVAA,
    data: &[u8],
) -> Result<(), ContractError> {
    let signer_count = vaa.len_signers as usize;
    if signer_count < ROUTER_QUORUM {
        return Err(ContractError::NoQuorum);
    }
    if signer_count > config.routers.len() {
        return Err(ContractError::TooManySignatures);
    }

    let mut last_index: i16 = -1;
    let mut pos = ParsedVAA::HEADER_LEN;
    for _ in 0..signer_count {
        if pos + ParsedVAA::SIGNATURE_LEN > data.len() {
            return Err(ContractError::InvalidVAA);
        }

        let router_index = data[pos] as i16;
        if router_index <= last_index {
            return Err(ContractError::WrongRouterIndexOrder);
        }
        last_index = router_index;

        let router_index = router_index as usize;
        if router_index >= config.routers.len() {
            return Err(ContractError::InvalidRouterIndex);
        }

        let signature = Signature::try_from(
            &data[pos + ParsedVAA::SIG_DATA_POS
                ..pos + ParsedVAA::SIG_DATA_POS + ParsedVAA::SIG_DATA_LEN],
        )
        .map_err(|_| ContractError::CannotDecodeSignature)?;
        let recovery_id = RecoveryId::try_from(data[pos + ParsedVAA::SIG_RECOVERY_POS])
            .map_err(|_| ContractError::CannotDecodeSignature)?;
        let verify_key =
            VerifyingKey::recover_from_prehash(vaa.hash.as_slice(), &signature, recovery_id)
                .map_err(|_| ContractError::CannotRecoverKey)?;

        if router_address(&verify_key) != config.routers[router_index] {
            return Err(ContractError::RouterSignatureError);
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

fn push_router_address(routers: &mut Vec<Vec<u8>>, bytes: &[u8]) -> Result<(), ContractError> {
    if bytes.len() != ROUTER_ADDRESS_LEN {
        return Err(ContractError::InvalidAddressLength);
    }
    if routers.iter().any(|existing| existing.as_slice() == bytes) {
        return Err(ContractError::InvalidConfig);
    }
    routers.push(bytes.to_vec());
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use k256::ecdsa::{RecoveryId, Signature, SigningKey, VerifyingKey};

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
        let config = setup(&keys);
        let payload = b"pyth-root".to_vec();
        let vaa = signed_vaa(
            &keys,
            &[0, 2, 4],
            ROUTER_SET_INDEX,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            payload.clone(),
        );

        let parsed = verify_vaa(&config, &vaa).unwrap();
        assert_eq!(parsed.guardian_set_index, ROUTER_SET_INDEX);
        assert_eq!(parsed.len_signers, 3);
        assert_eq!(parsed.emitter_chain, EMITTER_CHAIN);
        assert_eq!(parsed.emitter_address, EMITTER_ADDRESS);
        assert_eq!(parsed.payload, payload);
    }

    #[test]
    fn rejects_two_of_five_router_signatures() {
        let keys = router_keys();
        let config = setup(&keys);
        let vaa = signed_vaa(
            &keys,
            &[0, 1],
            ROUTER_SET_INDEX,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            vec![],
        );

        let err = verify_vaa(&config, &vaa).unwrap_err();
        assert!(err.to_string().contains("NoQuorum"));
    }

    #[test]
    fn rejects_wrong_router_signature() {
        let keys = router_keys();
        let config = setup(&keys);
        let outsider = SigningKey::from_bytes((&[42u8; 32]).into()).unwrap();
        let vaa = signed_vaa_with_keys(
            &[outsider, keys[1].clone(), keys[2].clone()],
            &[0, 1, 2],
            ROUTER_SET_INDEX,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            vec![],
        );

        let err = verify_vaa(&config, &vaa).unwrap_err();
        assert!(err.to_string().contains("RouterSignatureError"));
    }

    #[test]
    fn rejects_wrong_emitter() {
        let keys = router_keys();
        let config = setup(&keys);
        let vaa = signed_vaa(
            &keys,
            &[0, 1, 2],
            ROUTER_SET_INDEX,
            27,
            EMITTER_ADDRESS,
            vec![],
        );

        let err = verify_vaa(&config, &vaa).unwrap_err();
        assert!(err.to_string().contains("InvalidEmitter"));
    }

    #[test]
    fn rejects_wrong_version() {
        let keys = router_keys();
        let config = setup(&keys);
        let mut vaa = signed_vaa(
            &keys,
            &[0, 1, 2],
            ROUTER_SET_INDEX,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            vec![],
        );
        vaa[0] = 2;

        let err = verify_vaa(&config, &vaa).unwrap_err();
        assert!(matches!(err, ContractError::InvalidVersion));
    }

    #[test]
    fn rejects_wrong_router_set_index() {
        let keys = router_keys();
        let config = setup(&keys);
        let vaa = signed_vaa(
            &keys,
            &[0, 1, 2],
            ROUTER_SET_INDEX + 1,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            vec![],
        );

        let err = verify_vaa(&config, &vaa).unwrap_err();
        assert!(matches!(err, ContractError::InvalidRouterSetIndex));
    }

    #[test]
    fn rejects_duplicate_router_addresses() {
        let keys = router_keys();
        let mut routers: Vec<RouterAddress> = keys.iter().map(router_address_msg).collect();
        routers[1] = routers[0].clone();

        let err = parse_config(RouterVerifierConfigMsg {
            router_set_index: ROUTER_SET_INDEX,
            routers,
            expected_emitter_chain: EMITTER_CHAIN,
            expected_emitter_address: Binary::from(EMITTER_ADDRESS),
        })
        .unwrap_err();

        assert!(err.to_string().contains("InvalidConfig"));
    }

    #[test]
    fn rejects_invalid_router_index() {
        let keys = router_keys();
        let config = setup(&keys);
        let vaa = signed_vaa_with_keys(
            &[keys[0].clone(), keys[1].clone(), keys[4].clone()],
            &[0, 1, 5],
            ROUTER_SET_INDEX,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            vec![],
        );

        let err = verify_vaa(&config, &vaa).unwrap_err();
        assert!(matches!(err, ContractError::InvalidRouterIndex));
    }

    #[test]
    fn rejects_duplicate_signature_indexes() {
        let keys = router_keys();
        let config = setup(&keys);
        let vaa = signed_vaa(
            &keys,
            &[0, 0, 1],
            ROUTER_SET_INDEX,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            vec![],
        );

        let err = verify_vaa(&config, &vaa).unwrap_err();
        assert!(matches!(err, ContractError::WrongRouterIndexOrder));
    }

    #[test]
    fn rejects_unsorted_signature_indexes() {
        let keys = router_keys();
        let config = setup(&keys);
        let vaa = signed_vaa(
            &keys,
            &[0, 2, 1],
            ROUTER_SET_INDEX,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            vec![],
        );

        let err = verify_vaa(&config, &vaa).unwrap_err();
        assert!(matches!(err, ContractError::WrongRouterIndexOrder));
    }

    #[test]
    fn rejects_too_many_signatures() {
        let mut keys = router_keys();
        keys.push(SigningKey::from_bytes((&[6u8; 32]).into()).unwrap());
        let config = setup(&keys[..ROUTER_COUNT]);
        let vaa = signed_vaa_with_keys(
            &keys,
            &[0, 1, 2, 3, 4, 5],
            ROUTER_SET_INDEX,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            vec![],
        );

        let err = verify_vaa(&config, &vaa).unwrap_err();
        assert!(matches!(err, ContractError::TooManySignatures));
    }

    #[test]
    fn verifies_live_upgraded_hermes_akt_vaa_fixture() {
        let config = setup_production();
        let vaa = hex::decode(AKT_UPGRADED_HERMES_VAA_HEX).unwrap();

        let parsed = verify_vaa(&config, &vaa).unwrap();
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

    fn setup(keys: &[SigningKey]) -> RouterVerifierConfig {
        parse_config(RouterVerifierConfigMsg {
            router_set_index: ROUTER_SET_INDEX,
            routers: keys.iter().map(router_address_msg).collect(),
            expected_emitter_chain: EMITTER_CHAIN,
            expected_emitter_address: Binary::from(EMITTER_ADDRESS),
        })
        .unwrap()
    }

    fn setup_production() -> RouterVerifierConfig {
        parse_config(RouterVerifierConfigMsg {
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
        })
        .unwrap()
    }

    fn router_keys() -> Vec<SigningKey> {
        (1u8..=5)
            .map(|i| SigningKey::from_bytes((&[i; 32]).into()).unwrap())
            .collect()
    }

    fn router_address_msg(key: &SigningKey) -> RouterAddress {
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
}
