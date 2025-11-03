use cosmwasm_std::{
    entry_point, to_json_binary, Binary, Deps, DepsMut, Env, MessageInfo, QuerierWrapper,
    Response, StdResult, Uint128, Uint256,
};

use crate::error::ContractError;
use crate::msg::{
    ConfigResponse, ExecuteMsg, InstantiateMsg, MigrateMsg, PriceFeedIdResponse,
    PriceFeedResponse, PriceResponse, QueryMsg,
};
use crate::querier::{AkashQuerier, AkashQuery};
use crate::state::{Config, PriceFeed, CONFIG, PRICE_FEED};

// Maximum allowed staleness in seconds (5 minutes)
const MAX_STALENESS: i64 = 300;

// Expected exponent for AKT/USD price (8 decimals)
const EXPECTED_EXPO: i32 = -8;

/// Query the price feed ID from the chain's oracle module params using custom query
fn fetch_price_feed_id_from_chain(
    querier: &QuerierWrapper<AkashQuery>,
) -> Result<String, ContractError> {
    let response = querier
        .query_oracle_params()
        .map_err(|e| ContractError::InvalidPriceData {
            reason: format!("Failed to query oracle params from chain: {}", e),
        })?;

    // Validate the price feed ID is not empty
    if response.params.akt_price_feed_id.is_empty() {
        return Err(ContractError::InvalidPriceData {
            reason: "Price feed ID not configured in chain params".to_string(),
        });
    }

    Ok(response.params.akt_price_feed_id)
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn instantiate(
    deps: DepsMut<AkashQuery>,
    _env: Env,
    _info: MessageInfo,
    msg: InstantiateMsg,
) -> Result<Response, ContractError> {
    // Validate admin address
    let admin = deps.api.addr_validate(&msg.admin)?;

    // Fetch price feed ID from chain params at startup using custom query
    let price_feed_id = if msg.price_feed_id.is_empty() {
        // If not provided in msg, fetch from chain params
        fetch_price_feed_id_from_chain(&deps.querier.into())?
    } else {
        // Use provided value
        msg.price_feed_id.clone()
    };

    // Initialize config with price feed ID
    let config = Config {
        admin,
        update_fee: msg.update_fee,
        price_feed_id: price_feed_id.clone(),
    };
    CONFIG.save(deps.storage, &config)?;

    // Initialize price feed with default values
    let price_feed = PriceFeed::new();
    PRICE_FEED.save(deps.storage, &price_feed)?;

    Ok(Response::new()
        .add_attribute("method", "instantiate")
        .add_attribute("admin", msg.admin)
        .add_attribute("update_fee", msg.update_fee)
        .add_attribute("price_feed_id", price_feed_id))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn execute(
    deps: DepsMut<AkashQuery>,
    env: Env,
    info: MessageInfo,
    msg: ExecuteMsg,
) -> Result<Response, ContractError> {
    match msg {
        ExecuteMsg::UpdatePriceFeed {
            price,
            conf,
            expo,
            publish_time,
        } => execute_update_price_feed(deps, env, info, price, conf, expo, publish_time),
        ExecuteMsg::UpdateFee { new_fee } => execute_update_fee(deps, info, new_fee),
        ExecuteMsg::TransferAdmin { new_admin } => execute_transfer_admin(deps, info, new_admin),
    }
}

pub fn execute_update_price_feed(
    deps: DepsMut<AkashQuery>,
    env: Env,
    info: MessageInfo,
    price: Uint128,
    conf: Uint128,
    expo: i32,
    publish_time: i64,
) -> Result<Response, ContractError> {
    let config = CONFIG.load(deps.storage)?;

    // Check if sufficient fee was paid (CosmWasm 3.x uses Uint256 for coin amounts)
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

    // Validate price data
    if price.is_zero() {
        return Err(ContractError::ZeroPrice {});
    }

    // Validate exponent
    if expo != EXPECTED_EXPO {
        return Err(ContractError::InvalidExponent { expo });
    }

    // Check staleness
    let current_time = env.block.time.seconds() as i64;
    if current_time - publish_time > MAX_STALENESS {
        return Err(ContractError::StalePriceData {
            current_time,
            publish_time,
        });
    }

    // Validate confidence interval (should not exceed 5% of price)
    let max_conf = price.multiply_ratio(5u128, 100u128);
    if conf > max_conf {
        return Err(ContractError::HighConfidence {
            conf: conf.to_string(),
        });
    }

    // Load existing price feed to get previous publish time
    let mut price_feed = PRICE_FEED.load(deps.storage)?;

    // Ensure new price is not older than current price
    if publish_time <= price_feed.publish_time {
        return Err(ContractError::InvalidPriceData {
            reason: format!(
                "New publish time {} is not newer than current publish time {}",
                publish_time, price_feed.publish_time
            ),
        });
    }

    // Update price feed
    price_feed.prev_publish_time = price_feed.publish_time;
    price_feed.price = price;
    price_feed.conf = conf;
    price_feed.expo = expo;
    price_feed.publish_time = publish_time;

    PRICE_FEED.save(deps.storage, &price_feed)?;

    Ok(Response::new()
        .add_attribute("method", "update_price_feed")
        .add_attribute("price", price.to_string())
        .add_attribute("conf", conf.to_string())
        .add_attribute("publish_time", publish_time.to_string())
        .add_attribute("updater", info.sender))
}

pub fn execute_update_fee(
    deps: DepsMut<AkashQuery>,
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
    deps: DepsMut<AkashQuery>,
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

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn query(deps: Deps<AkashQuery>, env: Env, msg: QueryMsg) -> StdResult<Binary> {
    match msg {
        QueryMsg::GetPrice {} => to_json_binary(&query_price(deps, env)?),
        QueryMsg::GetPriceFeed {} => to_json_binary(&query_price_feed(deps)?),
        QueryMsg::GetConfig {} => to_json_binary(&query_config(deps)?),
        QueryMsg::GetPriceFeedId {} => to_json_binary(&query_price_feed_id(deps)?),
    }
}

fn query_price(deps: Deps<AkashQuery>, _env: Env) -> StdResult<PriceResponse> {
    let price_feed = PRICE_FEED.load(deps.storage)?;

    Ok(PriceResponse {
        price: price_feed.price,
        conf: price_feed.conf,
        expo: price_feed.expo,
        publish_time: price_feed.publish_time,
    })
}

fn query_price_feed(deps: Deps<AkashQuery>) -> StdResult<PriceFeedResponse> {
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

fn query_config(deps: Deps<AkashQuery>) -> StdResult<ConfigResponse> {
    let config = CONFIG.load(deps.storage)?;

    Ok(ConfigResponse {
        admin: config.admin.to_string(),
        update_fee: config.update_fee,
        price_feed_id: config.price_feed_id,
    })
}

fn query_price_feed_id(deps: Deps<AkashQuery>) -> StdResult<PriceFeedIdResponse> {
    let config = CONFIG.load(deps.storage)?;

    Ok(PriceFeedIdResponse {
        price_feed_id: config.price_feed_id,
    })
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn migrate(_deps: DepsMut<AkashQuery>, _env: Env, _msg: MigrateMsg) -> Result<Response, ContractError> {
    Ok(Response::default())
}

#[cfg(test)]
mod tests {
    use super::*;
    use cosmwasm_std::testing::{message_info, mock_dependencies, mock_env};
    use cosmwasm_std::{coin, from_json};

    #[test]
    fn test_instantiate_with_provided_id() {
        let mut deps = mock_dependencies();
        let msg = InstantiateMsg {
            admin: "admin".to_string(),
            update_fee: Uint256::from(1000u128),
            price_feed_id: "0xabc123def456".to_string(),
        };
        let info = message_info(&deps.api.addr_make("creator"), &[]);
        let env = mock_env();

        let res = instantiate(deps.as_mut(), env.clone(), info, msg).unwrap();
        assert_eq!(4, res.attributes.len());

        let config: ConfigResponse =
            from_json(&query(deps.as_ref(), env, QueryMsg::GetConfig {}).unwrap()).unwrap();
        assert_eq!("admin", config.admin);
        assert_eq!("0xabc123def456", config.price_feed_id);
    }

    #[test]
    fn test_update_price_feed() {
        let mut deps = mock_dependencies();

        let config = Config {
            admin: deps.api.addr_make("admin"),
            update_fee: Uint256::from(1000u128),
            price_feed_id: "0xtest123".to_string(),
        };
        CONFIG.save(&mut deps.storage, &config).unwrap();

        let price_feed = PriceFeed::new();
        PRICE_FEED.save(&mut deps.storage, &price_feed).unwrap();

        let env = mock_env();

        let update_msg = ExecuteMsg::UpdatePriceFeed {
            price: Uint128::new(123000000),
            conf: Uint128::new(1000000),
            expo: -8,
            publish_time: env.block.time.seconds() as i64,
        };
        let info = message_info(&deps.api.addr_make("updater"), &[coin(1000, "uakt")]);
        let res = execute(deps.as_mut(), env.clone(), info, update_msg).unwrap();
        assert_eq!(5, res.attributes.len());

        let price: PriceResponse =
            from_json(&query(deps.as_ref(), env, QueryMsg::GetPrice {}).unwrap()).unwrap();
        assert_eq!(Uint128::new(123000000), price.price);
    }

    #[test]
    fn test_update_fee() {
        let mut deps = mock_dependencies();

        let config = Config {
            admin: deps.api.addr_make("admin"),
            update_fee: Uint256::from(1000u128),
            price_feed_id: "0xtest123".to_string(),
        };
        CONFIG.save(&mut deps.storage, &config).unwrap();

        let msg = ExecuteMsg::UpdateFee {
            new_fee: Uint256::from(2000u128),
        };
        let info = message_info(&deps.api.addr_make("admin"), &[]);
        let res = execute(deps.as_mut(), mock_env(), info, msg).unwrap();
        assert_eq!(2, res.attributes.len());

        let config: ConfigResponse =
            from_json(&query(deps.as_ref(), mock_env(), QueryMsg::GetConfig {}).unwrap())
                .unwrap();
        assert_eq!(Uint256::from(2000u128), config.update_fee);
    }

    #[test]
    fn test_query_price_feed_id() {
        let mut deps = mock_dependencies();

        let config = Config {
            admin: deps.api.addr_make("admin"),
            update_fee: Uint256::from(1000u128),
            price_feed_id: "0xabc123def456".to_string(),
        };
        CONFIG.save(&mut deps.storage, &config).unwrap();

        let response: PriceFeedIdResponse = from_json(
            &query(
                deps.as_ref(),
                mock_env(),
                QueryMsg::GetPriceFeedId {},
            )
            .unwrap(),
        )
        .unwrap();

        assert_eq!("0xabc123def456", response.price_feed_id);
    }
}
