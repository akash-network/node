use cosmwasm_std::{
    entry_point, to_json_binary, Binary, Deps, DepsMut, Env, MessageInfo, Response, StdResult,
};

use crate::error::ContractError;
use crate::msg::{ConfigResponse, ExecuteMsg, InstantiateMsg, QueryMsg};
use crate::router;
use crate::state::{Config, CONFIG};

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
            router_verifier,
        },
    )?;

    Ok(Response::new()
        .add_attribute("method", "instantiate")
        .add_attribute("admin", msg.admin)
        .add_attribute("router_verifier", "configured"))
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
        ExecuteMsg::UpdateConfig { router_verifier } => {
            execute_update_config(deps, info, router_verifier)
        }
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

fn execute_update_config(
    deps: DepsMut,
    info: MessageInfo,
    router_verifier: crate::msg::RouterVerifierConfigMsg,
) -> Result<Response, ContractError> {
    let mut config = CONFIG.load(deps.storage)?;
    if info.sender != config.admin {
        return Err(ContractError::Unauthorized {});
    }

    config.router_verifier = router::parse_config(router_verifier)?;
    CONFIG.save(deps.storage, &config)?;

    Ok(Response::new()
        .add_attribute("method", "update_config")
        .add_attribute("router_verifier", "configured"))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn query(deps: Deps, _env: Env, msg: QueryMsg) -> StdResult<Binary> {
    match msg {
        QueryMsg::VerifyVAA { vaa, block_time: _ } => to_json_binary(&verify_vaa(deps, vaa)?),
        QueryMsg::GetConfig {} => to_json_binary(&query_config(deps)?),
    }
}

fn verify_vaa(deps: Deps, vaa: Binary) -> Result<crate::vaa::ParsedVAA, ContractError> {
    let config = CONFIG.load(deps.storage)?;
    router::verify_vaa(&config.router_verifier, vaa.as_slice())
}

fn query_config(deps: Deps) -> StdResult<ConfigResponse> {
    let config = CONFIG.load(deps.storage)?;

    Ok(ConfigResponse {
        admin: config.admin.to_string(),
        router_verifier: router::config_to_msg(&config.router_verifier),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use cosmwasm_std::testing::{message_info, mock_dependencies, mock_env};
    use cosmwasm_std::Binary;

    use crate::msg::{RouterAddress, RouterVerifierConfigMsg};

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
            router_verifier: router_config(),
        };
        let sender = deps.api.addr_make("sender");

        instantiate(deps.as_mut(), mock_env(), message_info(&sender, &[]), msg).unwrap();

        let response = query_config(deps.as_ref()).unwrap();
        assert_eq!(response.router_verifier.routers.len(), 5);
        assert_eq!(response.router_verifier.expected_emitter_chain, 26);
    }
}
