use cosmwasm_std::{
    entry_point, to_json_binary, AnyMsg, Binary, CosmosMsg, Deps, DepsMut, Env, MessageInfo,
    Response, StdResult, Uint128, Uint256,
};

use crate::accumulator::{parse_accumulator_update, verify_merkle_proof};
use crate::error::ContractError;
use crate::msg::{
    ConfigResponse, ExecuteMsg, InstantiateMsg, MigrateMsg, PriceFeedIdResponse, PriceFeedResponse,
    PriceResponse, QueryMsg, RouterVerifierConfigMsg,
};
use crate::oracle::{pyth_price_to_decimal, MsgAddPriceEntry};
use crate::pyth::parse_price_feed_message;
use crate::router;
use crate::state::{Config, DataID, PriceFeed, CONFIG, PRICE_FEED};

// Expected exponent for AKT/USD price (8 decimals)
const EXPECTED_EXPO: i32 = -8;

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn instantiate(
    deps: DepsMut,
    _env: Env,
    _info: MessageInfo,
    msg: InstantiateMsg,
) -> Result<Response, ContractError> {
    // Validate admin address
    let admin = deps.api.addr_validate(&msg.admin)?;

    let router_verifier = router::parse_config(msg.router_verifier)?;

    // Require price feed ID
    if msg.price_feed_id.is_empty() {
        return Err(ContractError::InvalidPriceData {
            reason: "price_feed_id is required".to_string(),
        });
    }

    let config = Config {
        admin,
        router_verifier,
        update_fee: msg.update_fee,
        price_feed_id: msg.price_feed_id.clone(),
        default_data_id: DataID::akt_usd(),
    };
    CONFIG.save(deps.storage, &config)?;

    // Initialize price feed with default values
    let price_feed = PriceFeed::new();
    PRICE_FEED.save(deps.storage, &price_feed)?;

    let mut response = Response::new()
        .add_attribute("method", "instantiate")
        .add_attribute("admin", msg.admin)
        .add_attribute("update_fee", msg.update_fee)
        .add_attribute("price_feed_id", msg.price_feed_id);

    response = response.add_attribute("router_verifier", "configured");

    Ok(response)
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn execute(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    msg: ExecuteMsg,
) -> Result<Response, ContractError> {
    match msg {
        ExecuteMsg::UpdatePriceFeed { vaa } => execute_update_price_feed(deps, env, info, vaa),
        ExecuteMsg::UpdateFee { new_fee } => execute_update_fee(deps, info, new_fee),
        ExecuteMsg::TransferAdmin { new_admin } => execute_transfer_admin(deps, info, new_admin),
        ExecuteMsg::UpdateConfig {
            router_verifier,
            price_feed_id,
        } => execute_update_config(deps, info, router_verifier, price_feed_id),
    }
}

/// Validate that a Pyth price is non-negative and convert to Uint128.
/// Rejects negative prices that would otherwise be silently converted
/// to their absolute value by unsigned_abs().
fn validate_pyth_price(raw_price: i64) -> Result<Uint128, ContractError> {
    if raw_price < 0 {
        return Err(ContractError::InvalidPriceData {
            reason: "negative price".to_string(),
        });
    }
    Ok(Uint128::new(raw_price as u128))
}

/// Execute price feed update with upgraded Pyth PNAU accumulator data.
pub fn execute_update_price_feed(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    vaa: Binary,
) -> Result<Response, ContractError> {
    let config = CONFIG.load(deps.storage)?;

    // Check if sufficient fee was paid
    let sent_amount = info
        .funds
        .iter()
        .find(|coin| coin.denom == "uakt")
        .map(|coin| coin.amount)
        .unwrap_or_else(Uint256::zero);

    if sent_amount < config.update_fee {
        return Err(ContractError::InsufficientFunds {
            required: config.update_fee.to_string(),
            sent: sent_amount.to_string(),
        });
    }

    let data_bytes = vaa.as_slice();

    let accumulator =
        parse_accumulator_update(data_bytes).map_err(|_| ContractError::InvalidPriceData {
            reason: "failed to parse accumulator update".to_string(),
        })?;

    if accumulator.price_updates.is_empty() {
        return Err(ContractError::InvalidPriceData {
            reason: "No price updates in accumulator".to_string(),
        });
    }

    let verified_vaa = router::verify_vaa(&config.router_verifier, accumulator.vaa.as_slice())?;

    let price_update = &accumulator.price_updates[0];
    if !verify_merkle_proof(
        &price_update.message_data,
        &price_update.merkle_proof,
        &accumulator.merkle_root,
    ) {
        return Err(ContractError::InvalidPriceData {
            reason: "Merkle proof verification failed".to_string(),
        });
    }

    let pyth_price = parse_price_feed_message(&price_update.message_data).map_err(|_| {
        ContractError::InvalidPriceData {
            reason: "failed to parse price feed message".to_string(),
        }
    })?;

    // Step 4: Validate price feed ID matches expected
    if pyth_price.id != config.price_feed_id {
        return Err(ContractError::InvalidPriceData {
            reason: "price feed ID mismatch".to_string(),
        });
    }

    // Convert Pyth price types
    let price = validate_pyth_price(pyth_price.price)?;
    let conf = Uint128::new(pyth_price.conf as u128);
    let expo = pyth_price.expo;
    let publish_time = pyth_price.publish_time;

    // Validate price data
    if price.is_zero() {
        return Err(ContractError::ZeroPrice {});
    }

    // Validate exponent
    if expo != EXPECTED_EXPO {
        return Err(ContractError::InvalidExponent { expo });
    }

    // Load existing price feed to get previous publish time
    let mut price_feed = PRICE_FEED.load(deps.storage)?;

    // Reject truly older prices
    if publish_time < price_feed.publish_time {
        return Err(ContractError::InvalidPriceData {
            reason: "price data is older than current data".to_string(),
        });
    }

    // Update contract storage for same or newer timestamps.
    // Pyth may submit multiple prices within the same timestamp
    // but from different slots — all should be accepted.
    if publish_time >= price_feed.publish_time {
        price_feed.prev_publish_time = price_feed.publish_time;
        price_feed.price = price;
        price_feed.conf = conf;
        price_feed.expo = expo;
        price_feed.publish_time = publish_time;

        PRICE_FEED.save(deps.storage, &price_feed)?;
    }

    // Convert Pyth price to decimal string for x/oracle module
    let price_decimal = pyth_price_to_decimal(pyth_price.price, expo);

    // Create oracle message with proto format
    let oracle_msg = MsgAddPriceEntry::new(
        env.contract.address.to_string(),
        config.default_data_id.denom.clone(),
        config.default_data_id.base_denom.clone(),
        price_decimal.clone(),
        publish_time,
        0,
    );

    // Encode to protobuf for x/oracle module
    let oracle_data = oracle_msg.encode_to_protobuf();

    // Create Any message to submit price to x/oracle module
    let oracle_cosmos_msg: CosmosMsg = CosmosMsg::Any(AnyMsg {
        type_url: "/akash.oracle.v2.MsgAddPriceEntry".to_string(),
        value: oracle_data.clone(),
    });

    Ok(Response::new()
        .add_message(oracle_cosmos_msg)
        .add_attribute("method", "update_price_feed")
        .add_attribute("price", price.to_string())
        .add_attribute("conf", conf.to_string())
        .add_attribute("publish_time", publish_time.to_string())
        .add_attribute("oracle_price", price_decimal.clone())
        .add_attribute("oracle_denom", &config.default_data_id.denom)
        .add_attribute("oracle_base_denom", &config.default_data_id.base_denom)
        .add_attribute("oracle_data", oracle_data.to_base64())
        .add_attribute("vaa_emitter_chain", verified_vaa.emitter_chain.to_string())
        .add_attribute("updater", info.sender))
}

pub fn execute_update_fee(
    deps: DepsMut,
    info: MessageInfo,
    new_fee: Uint256,
) -> Result<Response, ContractError> {
    let mut config = CONFIG.load(deps.storage)?;

    // Only admin can update fee
    if info.sender != config.admin {
        return Err(ContractError::Unauthorized {});
    }

    config.update_fee = new_fee;
    CONFIG.save(deps.storage, &config)?;

    Ok(Response::new()
        .add_attribute("method", "update_fee")
        .add_attribute("new_fee", new_fee.to_string()))
}

pub fn execute_transfer_admin(
    deps: DepsMut,
    info: MessageInfo,
    new_admin: String,
) -> Result<Response, ContractError> {
    let mut config = CONFIG.load(deps.storage)?;

    // Only current admin can transfer admin rights
    if info.sender != config.admin {
        return Err(ContractError::Unauthorized {});
    }

    let new_admin_addr = deps.api.addr_validate(&new_admin)?;
    config.admin = new_admin_addr;
    CONFIG.save(deps.storage, &config)?;

    Ok(Response::new()
        .add_attribute("method", "transfer_admin")
        .add_attribute("new_admin", new_admin))
}

pub fn execute_update_config(
    deps: DepsMut,
    info: MessageInfo,
    router_verifier: Option<RouterVerifierConfigMsg>,
    price_feed_id: Option<String>,
) -> Result<Response, ContractError> {
    let mut config = CONFIG.load(deps.storage)?;

    // Only admin can update config
    if info.sender != config.admin {
        return Err(ContractError::Unauthorized {});
    }

    if let Some(router_config) = router_verifier {
        config.router_verifier = router::parse_config(router_config)?;
    }

    if let Some(feed_id) = price_feed_id {
        if feed_id.is_empty() {
            return Err(ContractError::InvalidPriceData {
                reason: "price_feed_id is required".to_string(),
            });
        }
        config.price_feed_id = feed_id;
    }

    CONFIG.save(deps.storage, &config)?;

    Ok(Response::new()
        .add_attribute("method", "update_config")
        .add_attribute("price_feed_id", config.price_feed_id))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn query(deps: Deps, _env: Env, msg: QueryMsg) -> StdResult<Binary> {
    match msg {
        QueryMsg::GetPrice {} => to_json_binary(&query_price(deps)?),
        QueryMsg::GetPriceFeed {} => to_json_binary(&query_price_feed(deps)?),
        QueryMsg::GetConfig {} => to_json_binary(&query_config(deps)?),
        QueryMsg::GetPriceFeedId {} => to_json_binary(&query_price_feed_id(deps)?),
    }
}

fn query_price(deps: Deps) -> StdResult<PriceResponse> {
    let price_feed = PRICE_FEED.load(deps.storage)?;

    Ok(PriceResponse {
        price: price_feed.price,
        conf: price_feed.conf,
        expo: price_feed.expo,
        publish_time: price_feed.publish_time,
    })
}

fn query_price_feed(deps: Deps) -> StdResult<PriceFeedResponse> {
    let price_feed = PRICE_FEED.load(deps.storage)?;

    Ok(PriceFeedResponse {
        symbol: price_feed.symbol,
        price: price_feed.price,
        conf: price_feed.conf,
        expo: price_feed.expo,
        publish_time: price_feed.publish_time,
        prev_publish_time: price_feed.prev_publish_time,
    })
}

fn query_config(deps: Deps) -> StdResult<ConfigResponse> {
    let config = CONFIG.load(deps.storage)?;

    Ok(ConfigResponse {
        admin: config.admin.to_string(),
        router_verifier: router::config_to_msg(&config.router_verifier),
        update_fee: config.update_fee,
        price_feed_id: config.price_feed_id,
        default_denom: config.default_data_id.denom,
        default_base_denom: config.default_data_id.base_denom,
    })
}

fn query_price_feed_id(deps: Deps) -> StdResult<PriceFeedIdResponse> {
    let config = CONFIG.load(deps.storage)?;

    Ok(PriceFeedIdResponse {
        price_feed_id: config.price_feed_id,
    })
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn migrate(deps: DepsMut, _env: Env, msg: MigrateMsg) -> Result<Response, ContractError> {
    let mut response = Response::new()
        .add_attribute("method", "migrate")
        .add_attribute("version", "3.0.0");

    if let Some(router_config) = msg.router_verifier {
        let mut config = CONFIG.load(deps.storage)?;
        config.router_verifier = router::parse_config(router_config)?;
        CONFIG.save(deps.storage, &config)?;

        response = response.add_attribute("router_verifier", "configured");
    }

    Ok(response)
}

#[cfg(test)]
mod tests {
    use super::*;
    use cosmwasm_std::testing::{message_info, mock_env, MockApi, MockQuerier, MockStorage};
    use cosmwasm_std::{coin, from_json, Empty, OwnedDeps};

    use crate::msg::{RouterAddress, RouterVerifierConfigMsg};
    use crate::state::RouterVerifierConfig;

    type MockDeps = OwnedDeps<MockStorage, MockApi, MockQuerier, Empty>;

    fn mock_deps() -> MockDeps {
        OwnedDeps {
            storage: MockStorage::default(),
            api: MockApi::default(),
            querier: MockQuerier::default(),
            custom_query_type: std::marker::PhantomData,
        }
    }

    fn setup_config(deps: &mut MockDeps) {
        let config = Config {
            admin: deps.api.addr_make("admin"),
            router_verifier: production_router_config(),
            update_fee: Uint256::from(1000u128),
            price_feed_id: "0xtest123".to_string(),
            default_data_id: DataID::akt_usd(),
        };
        CONFIG.save(&mut deps.storage, &config).unwrap();
    }

    fn production_router_config_msg() -> RouterVerifierConfigMsg {
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

    fn production_router_config() -> RouterVerifierConfig {
        router::parse_config(production_router_config_msg()).unwrap()
    }

    #[test]
    fn test_update_fee() {
        let mut deps = mock_deps();
        setup_config(&mut deps);

        let msg = ExecuteMsg::UpdateFee {
            new_fee: Uint256::from(2000u128),
        };
        let info = message_info(&deps.api.addr_make("admin"), &[]);
        let res = execute(deps.as_mut(), mock_env(), info, msg).unwrap();
        assert_eq!(2, res.attributes.len());

        let config: ConfigResponse =
            from_json(query(deps.as_ref(), mock_env(), QueryMsg::GetConfig {}).unwrap()).unwrap();
        assert_eq!(Uint256::from(2000u128), config.update_fee);
    }

    #[test]
    fn test_query_price_feed_id() {
        let mut deps = mock_deps();
        setup_config(&mut deps);

        // Update config with specific price feed id
        let mut config = CONFIG.load(&deps.storage).unwrap();
        config.price_feed_id = "0xabc123def456".to_string();
        CONFIG.save(&mut deps.storage, &config).unwrap();

        let response: PriceFeedIdResponse =
            from_json(query(deps.as_ref(), mock_env(), QueryMsg::GetPriceFeedId {}).unwrap())
                .unwrap();

        assert_eq!("0xabc123def456", response.price_feed_id);
    }

    #[test]
    fn test_query_config_includes_router_verifier() {
        let mut deps = mock_deps();
        setup_config(&mut deps);

        let response: ConfigResponse =
            from_json(query(deps.as_ref(), mock_env(), QueryMsg::GetConfig {}).unwrap()).unwrap();

        // Oracle module expects "akt" (not "uakt") for denom
        assert_eq!("akt", response.default_denom);
        assert_eq!("usd", response.default_base_denom);
        assert_eq!(0, response.router_verifier.router_set_index);
        assert_eq!(5, response.router_verifier.routers.len());
        assert_eq!(26, response.router_verifier.expected_emitter_chain);
    }

    #[test]
    fn test_migrate_can_update_router_verifier() {
        let mut deps = mock_deps();
        setup_config(&mut deps);

        let mut router_config = production_router_config_msg();
        router_config.router_set_index = 1;
        let res = migrate(
            deps.as_mut(),
            mock_env(),
            MigrateMsg {
                router_verifier: Some(router_config),
            },
        )
        .unwrap();

        assert!(res
            .attributes
            .iter()
            .any(|attr| attr.key == "router_verifier" && attr.value == "configured"));

        let config = CONFIG.load(&deps.storage).unwrap();
        assert_eq!(1, config.router_verifier.router_set_index);
    }

    #[test]
    fn test_update_price_feed_with_router_verified_pnau() {
        let mut deps = mock_deps();
        let config = Config {
            admin: deps.api.addr_make("admin"),
            router_verifier: production_router_config(),
            update_fee: Uint256::from(1000u128),
            price_feed_id: "0x4ea5bb4d2f5900cc2e97ba534240950740b4d3b89fe712a94a7304fd2fd92702"
                .to_string(),
            default_data_id: DataID::akt_usd(),
        };
        CONFIG.save(&mut deps.storage, &config).unwrap();
        PRICE_FEED
            .save(&mut deps.storage, &PriceFeed::new())
            .unwrap();

        let update =
            Binary::from(hex::decode(crate::accumulator::AKT_UPGRADED_HERMES_PNAU_HEX).unwrap());
        let info = message_info(&deps.api.addr_make("updater"), &[coin(1000, "uakt")]);
        let res = execute_update_price_feed(deps.as_mut(), mock_env(), info, update).unwrap();

        assert!(res
            .attributes
            .iter()
            .any(|attr| attr.key == "method" && attr.value == "update_price_feed"));

        let price_feed = PRICE_FEED.load(&deps.storage).unwrap();
        assert!(!price_feed.price.is_zero());
        assert_eq!(price_feed.expo, -8);
        assert!(price_feed.publish_time > 0);
    }

    #[test]
    fn test_update_price_feed_rejects_non_pnau_input() {
        let mut deps = mock_deps();
        let config = Config {
            admin: deps.api.addr_make("admin"),
            router_verifier: production_router_config(),
            update_fee: Uint256::from(1000u128),
            price_feed_id: "0x4ea5bb4d2f5900cc2e97ba534240950740b4d3b89fe712a94a7304fd2fd92702"
                .to_string(),
            default_data_id: DataID::akt_usd(),
        };
        CONFIG.save(&mut deps.storage, &config).unwrap();
        PRICE_FEED
            .save(&mut deps.storage, &PriceFeed::new())
            .unwrap();

        let info = message_info(&deps.api.addr_make("updater"), &[coin(1000, "uakt")]);
        let err =
            execute_update_price_feed(deps.as_mut(), mock_env(), info, Binary::from(vec![1, 2, 3]))
                .unwrap_err();

        assert!(err
            .to_string()
            .contains("failed to parse accumulator update"));
    }

    #[test]
    fn test_negative_price_rejected() {
        let result = validate_pyth_price(-500);
        assert!(result.is_err(), "negative price should be rejected");
        let err = result.unwrap_err();
        match err {
            ContractError::InvalidPriceData { reason } => {
                assert_eq!(reason, "negative price");
            }
            _ => panic!("expected InvalidPriceData, got {:?}", err),
        }
    }

    #[test]
    fn test_positive_price_accepted() {
        let result = validate_pyth_price(123456789);
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), Uint128::new(123456789));
    }

    #[test]
    fn test_zero_price_accepted_by_converter() {
        // validate_pyth_price allows zero; the ZeroPrice check is separate
        let result = validate_pyth_price(0);
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), Uint128::zero());
    }
}
