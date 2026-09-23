use cosmwasm_std::{
    entry_point, to_json_binary, Binary, Deps, DepsMut, Env, MessageInfo, Response, StdResult,
    Storage,
};

use crate::error::ContractError;
use crate::msg::{ConfigResponse, ExecuteMsg, InstantiateMsg, MigrateMsg, QueryMsg};
use crate::router;
use crate::state::{
    Config, RouterSet, RouterState, RouterVerifierConfig, CONFIG, LEGACY_CONFIG, ROUTER_SETS,
    ROUTER_STATE,
};

const DEFAULT_GOVERNANCE_TARGET_CHAIN: u16 = 0;

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn instantiate(
    deps: DepsMut,
    _env: Env,
    _info: MessageInfo,
    msg: InstantiateMsg,
) -> Result<Response, ContractError> {
    let admin = deps.api.addr_validate(&msg.admin)?;
    let router_verifier = router::parse_config(msg.router_verifier)?;

    CONFIG.save(
        deps.storage,
        &Config {
            admin,
            governance_target_chain: msg
                .governance_target_chain
                .unwrap_or(DEFAULT_GOVERNANCE_TARGET_CHAIN),
            expected_emitter_chain: router_verifier.expected_emitter_chain,
            expected_emitter_address: router_verifier.expected_emitter_address,
        },
    )?;
    ROUTER_STATE.save(
        deps.storage,
        &RouterState {
            router_set_index: router_verifier.router_set_index,
        },
    )?;
    ROUTER_SETS.save(
        deps.storage,
        router_verifier.router_set_index,
        &RouterSet {
            routers: router_verifier.routers,
        },
    )?;

    Ok(Response::new()
        .add_attribute("method", "instantiate")
        .add_attribute("admin", msg.admin)
        .add_attribute("router_verifier", "configured"))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn migrate(deps: DepsMut, _env: Env, msg: MigrateMsg) -> Result<Response, ContractError> {
    if ROUTER_STATE.may_load(deps.storage)?.is_some() {
        let config = CONFIG.load(deps.storage)?;
        if msg
            .governance_target_chain
            .is_some_and(|target| target != config.governance_target_chain)
        {
            return Err(ContractError::InvalidGovernanceTarget);
        }
        let router_verifier = load_router_verifier(deps.storage)?;
        return Ok(Response::new()
            .add_attribute("method", "migrate")
            .add_attribute(
                "router_set_index",
                router_verifier.router_set_index.to_string(),
            ));
    }

    let legacy = LEGACY_CONFIG.load(deps.storage)?;
    let router_verifier = legacy.router_verifier;
    let router_set_index = router_verifier.router_set_index;

    CONFIG.save(
        deps.storage,
        &Config {
            admin: legacy.admin,
            governance_target_chain: msg
                .governance_target_chain
                .unwrap_or(DEFAULT_GOVERNANCE_TARGET_CHAIN),
            expected_emitter_chain: router_verifier.expected_emitter_chain,
            expected_emitter_address: router_verifier.expected_emitter_address,
        },
    )?;
    ROUTER_STATE.save(deps.storage, &RouterState { router_set_index })?;
    ROUTER_SETS.save(
        deps.storage,
        router_set_index,
        &RouterSet {
            routers: router_verifier.routers,
        },
    )?;

    Ok(Response::new()
        .add_attribute("method", "migrate")
        .add_attribute("router_set_index", router_set_index.to_string()))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn execute(
    deps: DepsMut,
    _env: Env,
    info: MessageInfo,
    msg: ExecuteMsg,
) -> Result<Response, ContractError> {
    match msg {
        ExecuteMsg::TransferAdmin { new_admin } => execute_transfer_admin(deps, info, new_admin),
        ExecuteMsg::SubmitVAA { vaa } => execute_submit_vaa(deps, vaa),
    }
}

fn execute_transfer_admin(
    deps: DepsMut,
    info: MessageInfo,
    new_admin: String,
) -> Result<Response, ContractError> {
    let mut config = CONFIG.load(deps.storage)?;
    if info.sender != config.admin {
        return Err(ContractError::Unauthorized {});
    }

    config.admin = deps.api.addr_validate(&new_admin)?;
    CONFIG.save(deps.storage, &config)?;

    Ok(Response::new()
        .add_attribute("method", "transfer_admin")
        .add_attribute("new_admin", new_admin))
}

fn execute_submit_vaa(deps: DepsMut, vaa: Binary) -> Result<Response, ContractError> {
    let config = CONFIG.load(deps.storage)?;
    let mut router_state = ROUTER_STATE.load(deps.storage)?;
    let current_router_verifier = load_router_verifier(deps.storage)?;
    let parsed_vaa = router::verify_governance_vaa(&current_router_verifier, vaa.as_slice())?;
    let router_set_update =
        router::parse_router_set_update(&parsed_vaa.payload, config.governance_target_chain)?;
    let expected_next_index = router_state
        .router_set_index
        .checked_add(1)
        .ok_or(ContractError::RouterSetIndexIncreaseError)?;

    if router_set_update.router_set_index != expected_next_index {
        return Err(ContractError::RouterSetIndexIncreaseError);
    }
    if ROUTER_SETS.has(deps.storage, router_set_update.router_set_index) {
        return Err(ContractError::RouterSetAlreadyExists);
    }

    let old_router_set_index = router_state.router_set_index;
    router_state.router_set_index = router_set_update.router_set_index;
    ROUTER_SETS.save(
        deps.storage,
        router_set_update.router_set_index,
        &RouterSet {
            routers: router_set_update.routers,
        },
    )?;
    ROUTER_STATE.save(deps.storage, &router_state)?;

    Ok(Response::new()
        .add_attribute("method", "submit_v_a_a")
        .add_attribute("old_router_set_index", old_router_set_index.to_string())
        .add_attribute(
            "new_router_set_index",
            router_state.router_set_index.to_string(),
        ))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn query(deps: Deps, _env: Env, msg: QueryMsg) -> StdResult<Binary> {
    match msg {
        QueryMsg::VerifyVAA { vaa, block_time: _ } => to_json_binary(&verify_vaa(deps, vaa)?),
        QueryMsg::GetConfig {} => to_json_binary(&query_config(deps)?),
    }
}

fn verify_vaa(deps: Deps, vaa: Binary) -> Result<crate::vaa::ParsedVAA, ContractError> {
    let config = load_router_verifier(deps.storage)?;
    router::verify_vaa(&config, vaa.as_slice())
}

fn query_config(deps: Deps) -> StdResult<ConfigResponse> {
    let config = CONFIG.load(deps.storage)?;
    let router_verifier = load_router_verifier(deps.storage)?;

    Ok(ConfigResponse {
        admin: config.admin.to_string(),
        governance_target_chain: config.governance_target_chain,
        router_verifier: router::config_to_msg(&router_verifier),
    })
}

fn load_router_verifier(storage: &dyn Storage) -> Result<RouterVerifierConfig, ContractError> {
    let config = CONFIG.load(storage)?;
    let router_state = ROUTER_STATE.load(storage)?;
    let router_set = ROUTER_SETS.load(storage, router_state.router_set_index)?;

    Ok(RouterVerifierConfig {
        router_set_index: router_state.router_set_index,
        routers: router_set.routers,
        expected_emitter_chain: config.expected_emitter_chain,
        expected_emitter_address: config.expected_emitter_address,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use cosmwasm_std::testing::{message_info, mock_dependencies, mock_env};
    use cosmwasm_std::{to_json_binary, Binary};
    use k256::ecdsa::{RecoveryId, Signature, SigningKey, VerifyingKey};
    use sha3::{Digest, Keccak256};

    use crate::msg::{RouterAddress, RouterVerifierConfigMsg};

    const PRICE_EMITTER_CHAIN: u16 = 26;
    const PRICE_EMITTER_ADDRESS: [u8; 32] = *b"PythnetPythnetPythnetPythnetPyth";
    const EMITTER_CHAIN: u16 = 1;
    const EMITTER_ADDRESS: [u8; 32] = [
        0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
        0, 4,
    ];
    const GOVERNANCE_TARGET_CHAIN: u16 = 29;
    // Unmodified VAA from pyth-network/pyth-crosschain commit 859113ec53e59a3abaeeb3333ae71ef1fa09615e:
    // contract_manager/src/store/guardian_sets/ProCompatibleProductionGuardianSetVaas.json
    const PRODUCTION_ROUTER_SET_UPGRADE_VAA: &str =
        include_str!("../testdata/production-router-set-1.hex");

    fn router_config() -> RouterVerifierConfigMsg {
        RouterVerifierConfigMsg {
            router_set_index: 0,
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
            expected_emitter_chain: 26,
            expected_emitter_address: Binary::from(
                hex::decode("507974686e6574507974686e6574507974686e6574507974686e657450797468")
                    .unwrap(),
            ),
        }
    }

    #[test]
    fn instantiate_stores_router_config() {
        let mut deps = mock_dependencies();
        let msg = InstantiateMsg {
            admin: deps.api.addr_make("admin").to_string(),
            governance_target_chain: None,
            router_verifier: router_config(),
        };
        let sender = deps.api.addr_make("sender");

        instantiate(deps.as_mut(), mock_env(), message_info(&sender, &[]), msg).unwrap();

        let response = query_config(deps.as_ref()).unwrap();
        assert_eq!(response.router_verifier.routers.len(), 5);
        assert_eq!(response.router_verifier.expected_emitter_chain, 26);
    }

    #[test]
    fn submit_v_a_a_rotates_to_next_signed_router_set() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);

        instantiate(
            deps.as_mut(),
            mock_env(),
            message_info(&sender, &[]),
            InstantiateMsg {
                admin: admin.to_string(),
                governance_target_chain: None,
                router_verifier: RouterVerifierConfigMsg {
                    router_set_index: 0,
                    routers: current_keys.iter().map(router_address_msg).collect(),
                    expected_emitter_chain: PRICE_EMITTER_CHAIN,
                    expected_emitter_address: Binary::from(PRICE_EMITTER_ADDRESS),
                },
            },
        )
        .unwrap();

        let msg = ExecuteMsg::SubmitVAA {
            vaa: Binary::default(),
        };
        let json = to_json_binary(&msg).unwrap();
        assert_eq!(
            std::str::from_utf8(json.as_slice()).unwrap(),
            r#"{"submit_v_a_a":{"vaa":""}}"#
        );

        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(1, &next_keys)),
        );

        execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap();

        let response = query_config(deps.as_ref()).unwrap();
        assert_eq!(response.router_verifier.router_set_index, 1);
        assert_eq!(
            response
                .router_verifier
                .routers
                .iter()
                .map(|router| router.bytes.to_vec())
                .collect::<Vec<_>>(),
            next_keys
                .iter()
                .map(|key| address_from_key(key.verifying_key()).to_vec())
                .collect::<Vec<_>>()
        );
    }

    #[test]
    fn submit_v_a_a_rejects_admin_without_current_router_signatures() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);
        let outsider_keys = router_keys(20);

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let router_update_vaa = signed_vaa(
            &outsider_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(1, &next_keys)),
        );

        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&admin, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap_err();

        assert!(matches!(err, ContractError::RouterSignatureError));
        assert_active_router_set(deps.as_ref(), 0, &current_keys);
    }

    #[test]
    fn submit_v_a_a_rejects_skipped_router_set_index() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(2, &next_keys)),
        );

        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap_err();

        assert!(matches!(err, ContractError::RouterSetIndexIncreaseError));
        assert_active_router_set(deps.as_ref(), 0, &current_keys);
    }

    #[test]
    fn submit_v_a_a_rejects_wrong_emitter() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN + 1,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(1, &next_keys)),
        );

        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap_err();

        assert!(matches!(err, ContractError::InvalidEmitter));
        assert_active_router_set(deps.as_ref(), 0, &current_keys);
    }

    #[test]
    fn submit_v_a_a_accepts_configured_governance_target_chain() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);

        instantiate_with_keys_and_target(
            deps.as_mut(),
            &sender,
            &admin,
            &current_keys,
            Some(GOVERNANCE_TARGET_CHAIN),
        );

        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet_for_chain(
                GOVERNANCE_TARGET_CHAIN,
                router_set_update_payload(1, &next_keys),
            ),
        );

        execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap();

        assert_active_router_set(deps.as_ref(), 1, &next_keys);
    }

    #[test]
    fn submit_v_a_a_rejects_wrong_governance_target_chain() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);

        instantiate_with_keys_and_target(
            deps.as_mut(),
            &sender,
            &admin,
            &current_keys,
            Some(GOVERNANCE_TARGET_CHAIN),
        );

        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet_for_chain(
                GOVERNANCE_TARGET_CHAIN + 1,
                router_set_update_payload(1, &next_keys),
            ),
        );

        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap_err();

        assert!(matches!(err, ContractError::InvalidGovernanceTarget));
        assert_active_router_set(deps.as_ref(), 0, &current_keys);
    }

    #[test]
    fn submit_v_a_a_rejects_replay_after_rotation() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(1, &next_keys)),
        );

        execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa.clone()),
            },
        )
        .unwrap();

        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap_err();

        assert!(matches!(err, ContractError::InvalidRouterSetIndex));
        assert_active_router_set(deps.as_ref(), 1, &next_keys);
    }

    #[test]
    fn submit_v_a_a_accepts_published_production_rotation() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        instantiate(
            deps.as_mut(),
            mock_env(),
            message_info(&sender, &[]),
            InstantiateMsg {
                admin: admin.to_string(),
                governance_target_chain: Some(GOVERNANCE_TARGET_CHAIN),
                router_verifier: router_config(),
            },
        )
        .unwrap();
        let vaa = Binary::from(hex::decode(PRODUCTION_ROUTER_SET_UPGRADE_VAA.trim()).unwrap());
        let err = verify_vaa(deps.as_ref(), vaa.clone()).unwrap_err();
        assert!(matches!(err, ContractError::InvalidEmitter));

        let mut tampered = vaa.to_vec();
        *tampered.last_mut().unwrap() ^= 1;
        assert!(execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: tampered.into()
            }
        )
        .is_err());
        assert_eq!(
            query_config(deps.as_ref()).unwrap().router_verifier,
            router_config()
        );

        execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA { vaa: vaa.clone() },
        )
        .unwrap();

        let config = query_config(deps.as_ref()).unwrap().router_verifier;
        assert_eq!(config.router_set_index, 1);
        assert_eq!(config.expected_emitter_chain, PRICE_EMITTER_CHAIN);
        assert_eq!(
            config.expected_emitter_address.as_slice(),
            PRICE_EMITTER_ADDRESS
        );
        assert_eq!(
            config
                .routers
                .iter()
                .map(|router| hex::encode(&router.bytes))
                .collect::<Vec<_>>(),
            [
                "41534bb176e461a3fb30479400f210549ecce638",
                "6502987b62f21cab7eb5ccd8f0173084b60d5b41",
                "44a3e8f6a382412cf6bb90a3f8106e68977476c9",
                "13edc776d3063549fdb0702af182edc905a539d4",
                "3af088854bb768065f4929e5bbdfa4cbb04c9af9",
            ]
        );
        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA { vaa },
        )
        .unwrap_err();
        assert!(matches!(err, ContractError::InvalidRouterSetIndex));
    }

    #[test]
    fn submit_v_a_a_rejects_price_emitter_and_wrong_governance_address() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("anyone");
        let admin = deps.api.addr_make("admin");
        let keys = router_keys(1);
        instantiate_with_keys(deps.as_mut(), &sender, &admin, &keys);
        for (chain, address) in [
            (PRICE_EMITTER_CHAIN, PRICE_EMITTER_ADDRESS),
            (EMITTER_CHAIN, [9; 32]),
        ] {
            let vaa = signed_vaa(
                &keys,
                &[0, 1, 2],
                0,
                chain,
                address,
                governance_packet(router_set_update_payload(1, &router_keys(6))),
            );
            let err = execute(
                deps.as_mut(),
                mock_env(),
                message_info(&sender, &[]),
                ExecuteMsg::SubmitVAA { vaa: vaa.into() },
            )
            .unwrap_err();
            assert!(matches!(err, ContractError::InvalidEmitter));
            assert_active_router_set(deps.as_ref(), 0, &keys);
        }
    }

    #[test]
    fn verify_v_a_a_uses_rotated_router_set() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(1, &next_keys)),
        );
        execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap();

        let old_set_price_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            PRICE_EMITTER_CHAIN,
            PRICE_EMITTER_ADDRESS,
            b"price-update".to_vec(),
        );
        let err = verify_vaa(deps.as_ref(), Binary::from(old_set_price_vaa)).unwrap_err();
        assert!(matches!(err, ContractError::InvalidRouterSetIndex));

        let new_set_price_vaa = signed_vaa(
            &next_keys,
            &[0, 1, 2],
            1,
            PRICE_EMITTER_CHAIN,
            PRICE_EMITTER_ADDRESS,
            b"price-update".to_vec(),
        );
        verify_vaa(deps.as_ref(), Binary::from(new_set_price_vaa)).unwrap();
    }

    #[test]
    fn submit_v_a_a_rejects_next_rotation_signed_by_previous_router_set() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);
        let third_keys = router_keys(11);

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let first_rotation = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(1, &next_keys)),
        );
        execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(first_rotation),
            },
        )
        .unwrap();

        let old_set_signed_second_rotation = signed_vaa_with_keys(
            &[
                current_keys[0].clone(),
                current_keys[1].clone(),
                current_keys[2].clone(),
            ],
            &[0, 1, 2],
            1,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(2, &third_keys)),
        );
        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(old_set_signed_second_rotation),
            },
        )
        .unwrap_err();

        assert!(matches!(err, ContractError::RouterSignatureError));
        assert_active_router_set(deps.as_ref(), 1, &next_keys);
    }

    #[test]
    fn submit_v_a_a_accepts_multiple_sequential_rotations() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);
        let third_keys = router_keys(11);

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let first_rotation = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(1, &next_keys)),
        );
        execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(first_rotation),
            },
        )
        .unwrap();

        let second_rotation = signed_vaa(
            &next_keys,
            &[0, 1, 2],
            1,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(2, &third_keys)),
        );
        execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(second_rotation),
            },
        )
        .unwrap();

        assert_active_router_set(deps.as_ref(), 2, &third_keys);
    }

    #[test]
    fn submit_v_a_a_rejects_wrong_governance_module() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let mut payload = governance_packet(router_set_update_payload(1, &next_keys));
        payload[28..32].copy_from_slice(b"Pyth");
        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            payload,
        );

        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap_err();

        assert!(matches!(err, ContractError::InvalidVAAAction));
        assert_active_router_set(deps.as_ref(), 0, &current_keys);
    }

    #[test]
    fn submit_v_a_a_rejects_wrong_governance_action() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let next_keys = router_keys(6);

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let mut payload = governance_packet(router_set_update_payload(1, &next_keys));
        payload[32] = 1;
        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            payload,
        );

        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap_err();

        assert!(matches!(err, ContractError::InvalidVAAAction));
        assert_active_router_set(deps.as_ref(), 0, &current_keys);
    }

    #[test]
    fn submit_v_a_a_rejects_duplicate_routers_in_update() {
        let mut deps = mock_dependencies();
        let sender = deps.api.addr_make("sender");
        let admin = deps.api.addr_make("admin");
        let submitter = deps.api.addr_make("anyone");
        let current_keys = router_keys(1);
        let mut next_keys = router_keys(6);
        next_keys[1] = next_keys[0].clone();

        instantiate_with_keys(deps.as_mut(), &sender, &admin, &current_keys);

        let router_update_vaa = signed_vaa(
            &current_keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(1, &next_keys)),
        );

        let err = execute(
            deps.as_mut(),
            mock_env(),
            message_info(&submitter, &[]),
            ExecuteMsg::SubmitVAA {
                vaa: Binary::from(router_update_vaa),
            },
        )
        .unwrap_err();

        assert!(matches!(err, ContractError::InvalidConfig));
        assert_active_router_set(deps.as_ref(), 0, &current_keys);
    }

    #[test]
    fn migrate_splits_legacy_router_config_into_state_and_sets() {
        let mut deps = mock_dependencies();
        let admin = deps.api.addr_make("admin");
        let current_keys = router_keys(1);
        let router_verifier = RouterVerifierConfig {
            router_set_index: 0,
            routers: current_keys
                .iter()
                .map(|key| address_from_key(key.verifying_key()).to_vec())
                .collect(),
            expected_emitter_chain: PRICE_EMITTER_CHAIN,
            expected_emitter_address: PRICE_EMITTER_ADDRESS.to_vec(),
        };

        LEGACY_CONFIG
            .save(
                deps.as_mut().storage,
                &crate::state::LegacyConfig {
                    admin: admin.clone(),
                    router_verifier,
                },
            )
            .unwrap();

        migrate(
            deps.as_mut(),
            mock_env(),
            MigrateMsg {
                governance_target_chain: Some(GOVERNANCE_TARGET_CHAIN),
            },
        )
        .unwrap();

        let config = CONFIG.load(deps.as_ref().storage).unwrap();
        assert_eq!(config.admin, admin);
        assert_eq!(config.governance_target_chain, GOVERNANCE_TARGET_CHAIN);
        assert_eq!(config.expected_emitter_chain, PRICE_EMITTER_CHAIN);
        assert_eq!(config.expected_emitter_address, PRICE_EMITTER_ADDRESS);
        assert_active_router_set(deps.as_ref(), 0, &current_keys);
    }

    #[test]
    fn migrate_preserves_current_schema_and_rotated_sets() {
        use cosmwasm_std::Order;
        let mut deps = mock_dependencies();
        let admin = deps.api.addr_make("admin");
        let keys = router_keys(1);
        instantiate_with_keys_and_target(
            deps.as_mut(),
            &admin,
            &admin,
            &keys,
            Some(GOVERNANCE_TARGET_CHAIN),
        );
        let vaa = signed_vaa(
            &keys,
            &[0, 1, 2],
            0,
            EMITTER_CHAIN,
            EMITTER_ADDRESS,
            governance_packet(router_set_update_payload(1, &router_keys(6))),
        );
        execute(
            deps.as_mut(),
            mock_env(),
            message_info(&admin, &[]),
            ExecuteMsg::SubmitVAA { vaa: vaa.into() },
        )
        .unwrap();
        let before: Vec<_> = deps.storage.range(None, None, Order::Ascending).collect();
        for target in [None, Some(GOVERNANCE_TARGET_CHAIN)] {
            migrate(
                deps.as_mut(),
                mock_env(),
                MigrateMsg {
                    governance_target_chain: target,
                },
            )
            .unwrap();
            assert_eq!(
                before,
                deps.storage
                    .range(None, None, Order::Ascending)
                    .collect::<Vec<_>>()
            );
        }
        let err = migrate(
            deps.as_mut(),
            mock_env(),
            MigrateMsg {
                governance_target_chain: Some(30),
            },
        )
        .unwrap_err();
        assert!(matches!(err, ContractError::InvalidGovernanceTarget));
        assert_eq!(
            before,
            deps.storage
                .range(None, None, Order::Ascending)
                .collect::<Vec<_>>()
        );
    }

    fn instantiate_with_keys(
        deps: DepsMut,
        sender: &cosmwasm_std::Addr,
        admin: &cosmwasm_std::Addr,
        keys: &[SigningKey],
    ) {
        instantiate_with_keys_and_target(deps, sender, admin, keys, None);
    }

    fn instantiate_with_keys_and_target(
        deps: DepsMut,
        sender: &cosmwasm_std::Addr,
        admin: &cosmwasm_std::Addr,
        keys: &[SigningKey],
        governance_target_chain: Option<u16>,
    ) {
        instantiate(
            deps,
            mock_env(),
            message_info(sender, &[]),
            InstantiateMsg {
                admin: admin.to_string(),
                governance_target_chain,
                router_verifier: RouterVerifierConfigMsg {
                    router_set_index: 0,
                    routers: keys.iter().map(router_address_msg).collect(),
                    expected_emitter_chain: PRICE_EMITTER_CHAIN,
                    expected_emitter_address: Binary::from(PRICE_EMITTER_ADDRESS),
                },
            },
        )
        .unwrap();
    }

    fn assert_active_router_set(deps: Deps, index: u32, keys: &[SigningKey]) {
        let response = query_config(deps).unwrap();
        assert_eq!(response.router_verifier.router_set_index, index);
        assert_eq!(
            response
                .router_verifier
                .routers
                .iter()
                .map(|router| router.bytes.to_vec())
                .collect::<Vec<_>>(),
            keys.iter()
                .map(|key| address_from_key(key.verifying_key()).to_vec())
                .collect::<Vec<_>>()
        );
    }

    fn router_keys(start: u8) -> Vec<SigningKey> {
        (start..start + 5)
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

    fn router_set_update_payload(router_set_index: u32, keys: &[SigningKey]) -> Vec<u8> {
        let mut payload = Vec::with_capacity(5 + keys.len() * 20);
        payload.extend_from_slice(&router_set_index.to_be_bytes());
        payload.push(keys.len() as u8);
        for key in keys {
            payload.extend_from_slice(&address_from_key(key.verifying_key()));
        }
        payload
    }

    fn governance_packet(inner_payload: Vec<u8>) -> Vec<u8> {
        governance_packet_for_chain(0, inner_payload)
    }

    fn governance_packet_for_chain(target_chain: u16, inner_payload: Vec<u8>) -> Vec<u8> {
        let mut payload = vec![0u8; 32];
        payload[28..].copy_from_slice(b"Core");
        payload.push(2);
        payload.extend_from_slice(&target_chain.to_be_bytes());
        payload.extend_from_slice(&inner_payload);
        payload
    }

    fn signed_vaa(
        keys: &[SigningKey],
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

        for index in signer_indexes {
            let (signature, recovery_id) = sign_hash(&keys[*index as usize], &hash);
            vaa.push(*index);
            vaa.extend_from_slice(&signature.to_bytes());
            vaa.push(recovery_id.to_byte());
        }

        vaa.extend_from_slice(&body);
        vaa
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
            vaa.extend_from_slice(&signature.to_bytes());
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
