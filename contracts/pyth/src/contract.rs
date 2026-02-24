use cosmwasm_std::{
    entry_point, to_json_binary, AnyMsg, Binary, CosmosMsg, Deps, DepsMut, Env, MessageInfo,
    QuerierWrapper, Response, StdResult, Uint128, Uint256, WasmQuery, QueryRequest,
};

use crate::accumulator::{parse_accumulator_update, verify_merkle_proof, PNAU_MAGIC};
use crate::error::ContractError;
use crate::msg::{
    ConfigResponse, DataSourceMsg, ExecuteMsg, InstantiateMsg, MigrateMsg, OracleParamsResponse,
    PriceFeedIdResponse, PriceFeedResponse, PriceResponse, QueryMsg,
};
use crate::oracle::{pyth_price_to_decimal, MsgAddPriceEntry};
use crate::pyth::{parse_pyth_payload, parse_price_feed_message};
use crate::querier::{AkashQuerier, AkashQuery, OracleParams};
use crate::state::{
    CachedOracleParams, Config, DataID, DataSource, PriceFeed,
    CONFIG, PRICE_FEED, CACHED_ORACLE_PARAMS,
};
use crate::wormhole::{WormholeQueryMsg, ParsedVAA};

// Expected exponent for AKT/USD price (8 decimals)
const EXPECTED_EXPO: i32 = -8;

// Approximate seconds per block (for staleness conversion)
const SECONDS_PER_BLOCK: i64 = 6;

/// Query full oracle params from the chain's oracle module
fn fetch_oracle_params_from_chain(
    querier: &QuerierWrapper<AkashQuery>,
) -> Result<OracleParams, ContractError> {
    let response = querier
        .query_oracle_params()
        .map_err(|e| ContractError::InvalidPriceData {
            reason: format!("Failed to query oracle params from chain: {}", e),
        })?;

    Ok(response.params)
}

/// Extract and validate the price feed ID from chain params
fn get_price_feed_id_from_params(params: &OracleParams) -> Result<String, ContractError> {
    params
        .get_akt_price_feed_id()
        .map(|id| id.to_string())
        .ok_or_else(|| ContractError::InvalidPriceData {
            reason: "Price feed ID not configured in chain params".to_string(),
        })
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn instantiate(
    deps: DepsMut<AkashQuery>,
    env: Env,
    _info: MessageInfo,
    msg: InstantiateMsg,
) -> Result<Response, ContractError> {
    // Validate admin address
    let admin = deps.api.addr_validate(&msg.admin)?;

    // Validate Wormhole contract address
    let wormhole_contract = deps.api.addr_validate(&msg.wormhole_contract)?;

    // Fetch full oracle params from chain
    let oracle_params = fetch_oracle_params_from_chain(&deps.querier.into())?;

    // Get price feed ID - use provided value or fetch from chain params
    let price_feed_id = if msg.price_feed_id.is_empty() {
        get_price_feed_id_from_params(&oracle_params)?
    } else {
        msg.price_feed_id.clone()
    };

    // Convert data sources from message format to storage format
    let data_sources: Vec<DataSource> = msg
        .data_sources
        .into_iter()
        .map(|ds| DataSource {
            emitter_chain: ds.emitter_chain,
            emitter_address: ds.emitter_address,
        })
        .collect();

    // Initialize config with Wormhole contract and data sources
    let config = Config {
        admin,
        wormhole_contract,
        update_fee: msg.update_fee,
        price_feed_id: price_feed_id.clone(),
        default_data_id: DataID::akt_usd(),
        data_sources,
    };
    CONFIG.save(deps.storage, &config)?;

    // Cache oracle params for validation
    let cached_params = CachedOracleParams {
        max_price_deviation_bps: oracle_params.max_price_deviation_bps,
        min_price_sources: oracle_params.min_price_sources,
        max_price_staleness_blocks: oracle_params.max_price_staleness_blocks,
        twap_window: oracle_params.twap_window,
        last_updated_height: env.block.height,
    };
    CACHED_ORACLE_PARAMS.save(deps.storage, &cached_params)?;

    // Initialize price feed with default values
    let price_feed = PriceFeed::new();
    PRICE_FEED.save(deps.storage, &price_feed)?;

    Ok(Response::new()
        .add_attribute("method", "instantiate")
        .add_attribute("admin", msg.admin)
        .add_attribute("wormhole_contract", msg.wormhole_contract)
        .add_attribute("update_fee", msg.update_fee)
        .add_attribute("price_feed_id", price_feed_id)
        .add_attribute("max_deviation_bps", oracle_params.max_price_deviation_bps.to_string()))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn execute(
    deps: DepsMut<AkashQuery>,
    env: Env,
    info: MessageInfo,
    msg: ExecuteMsg,
) -> Result<Response, ContractError> {
    match msg {
        ExecuteMsg::UpdatePriceFeed { vaa } => {
            execute_update_price_feed(deps, env, info, vaa)
        }
        ExecuteMsg::UpdateFee { new_fee } => execute_update_fee(deps, info, new_fee),
        ExecuteMsg::TransferAdmin { new_admin } => execute_transfer_admin(deps, info, new_admin),
        ExecuteMsg::RefreshOracleParams {} => execute_refresh_oracle_params(deps, env, info),
        ExecuteMsg::UpdateConfig {
            wormhole_contract,
            price_feed_id,
            data_sources,
        } => execute_update_config(deps, info, wormhole_contract, price_feed_id, data_sources),
    }
}

/// Execute price feed update with VAA verification
/// Accepts either:
/// - PNAU accumulator format (from Pyth Hermes v2 API)
/// - Raw Wormhole VAA (legacy format)
pub fn execute_update_price_feed(
    deps: DepsMut<AkashQuery>,
    env: Env,
    info: MessageInfo,
    vaa: Binary,
) -> Result<Response, ContractError> {
    let config = CONFIG.load(deps.storage)?;
    let cached_params = CACHED_ORACLE_PARAMS.load(deps.storage)?;

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

    // Detect format: PNAU accumulator or raw VAA
    let (actual_vaa, price_message_data) = if data_bytes.len() >= 4 && &data_bytes[0..4] == PNAU_MAGIC {
        // Parse PNAU accumulator format from Hermes v2 API
        let accumulator = parse_accumulator_update(data_bytes)
            .map_err(|e| ContractError::InvalidPriceData {
                reason: format!("Failed to parse PNAU accumulator: {}", e),
            })?;

        // Must have at least one price update
        if accumulator.price_updates.is_empty() {
            return Err(ContractError::InvalidPriceData {
                reason: "No price updates in accumulator".to_string(),
            });
        }

        // Get the first price update and verify its Merkle proof
        let price_update = &accumulator.price_updates[0];

        // Verify Merkle proof
        if !verify_merkle_proof(
            &price_update.message_data,
            &price_update.merkle_proof,
            &accumulator.merkle_root,
        ) {
            return Err(ContractError::InvalidPriceData {
                reason: "Merkle proof verification failed".to_string(),
            });
        }

        (accumulator.vaa, Some(price_update.message_data.clone()))
    } else {
        // Assume raw VAA format (legacy)
        (vaa, None)
    };

    // Step 1: Verify VAA via Wormhole contract
    let verify_query = WormholeQueryMsg::VerifyVAA {
        vaa: actual_vaa.clone(),
        block_time: env.block.time.seconds(),
    };

    let verified_vaa: ParsedVAA = deps.querier.query(&QueryRequest::Wasm(WasmQuery::Smart {
        contract_addr: config.wormhole_contract.to_string(),
        msg: to_json_binary(&verify_query)?,
    }))?;

    // Step 2: Validate emitter is from Pythnet (chain 26) for accumulator updates
    // For accumulator updates, the VAA contains a Merkle root signed by Wormhole
    // The emitter is Pythnet's accumulator program, not a specific data source
    if price_message_data.is_some() {
        // For PNAU format, verify emitter is Pythnet (chain 26)
        if verified_vaa.emitter_chain != 26 {
            return Err(ContractError::InvalidDataSource {
                emitter_chain: verified_vaa.emitter_chain,
                emitter_address: hex::encode(&verified_vaa.emitter_address),
            });
        }
    } else {
        // For raw VAA format, validate against configured data sources
        let is_valid_source = config.data_sources.iter().any(|ds| {
            ds.matches(verified_vaa.emitter_chain, &verified_vaa.emitter_address)
        });

        if !is_valid_source {
            return Err(ContractError::InvalidDataSource {
                emitter_chain: verified_vaa.emitter_chain,
                emitter_address: hex::encode(&verified_vaa.emitter_address),
            });
        }
    }

    // Step 3: Parse Pyth price data
    let pyth_price = if let Some(ref message_data) = price_message_data {
        // Parse from PNAU price message (Merkle-proven)
        parse_price_feed_message(message_data)
            .map_err(|e| ContractError::InvalidPriceData {
                reason: format!("Failed to parse price feed message: {}", e),
            })?
    } else {
        // Parse from raw VAA payload (legacy)
        parse_pyth_payload(&verified_vaa.payload)
            .map_err(|e| ContractError::InvalidPriceData {
                reason: format!("Failed to parse Pyth payload: {}", e),
            })?
    };

    // Step 4: Validate price feed ID matches expected
    if pyth_price.id != config.price_feed_id {
        return Err(ContractError::InvalidPriceData {
            reason: format!(
                "Price feed ID mismatch: expected {}, got {}",
                config.price_feed_id, pyth_price.id
            ),
        });
    }

    // Convert Pyth price types
    let price = Uint128::new(pyth_price.price.unsigned_abs() as u128);
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

    // Check staleness using chain's max_price_staleness_blocks converted to seconds
    let current_time = env.block.time.seconds() as i64;
    let max_staleness_seconds = cached_params.max_price_staleness_blocks * SECONDS_PER_BLOCK;
    if current_time - publish_time > max_staleness_seconds {
        return Err(ContractError::StalePriceData {
            current_time,
            publish_time,
        });
    }

    // Validate confidence interval using chain's max_price_deviation_bps
    let max_conf = price.multiply_ratio(cached_params.max_price_deviation_bps as u128, 10000u128);
    if conf > max_conf {
        return Err(ContractError::HighConfidence {
            conf: conf.to_string(),
            max_allowed: max_conf.to_string(),
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

    // Update price feed in contract storage
    price_feed.prev_publish_time = price_feed.publish_time;
    price_feed.price = price;
    price_feed.conf = conf;
    price_feed.expo = expo;
    price_feed.publish_time = publish_time;

    PRICE_FEED.save(deps.storage, &price_feed)?;

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
        type_url: "/akash.oracle.v1.MsgAddPriceEntry".to_string(),
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

pub fn execute_refresh_oracle_params(
    deps: DepsMut<AkashQuery>,
    env: Env,
    info: MessageInfo,
) -> Result<Response, ContractError> {
    let config = CONFIG.load(deps.storage)?;

    // Only admin can refresh params
    if info.sender != config.admin {
        return Err(ContractError::Unauthorized {});
    }

    // Fetch fresh params from chain
    let oracle_params = fetch_oracle_params_from_chain(&deps.querier.into())?;

    // Update cached params
    let cached_params = CachedOracleParams {
        max_price_deviation_bps: oracle_params.max_price_deviation_bps,
        min_price_sources: oracle_params.min_price_sources,
        max_price_staleness_blocks: oracle_params.max_price_staleness_blocks,
        twap_window: oracle_params.twap_window,
        last_updated_height: env.block.height,
    };
    CACHED_ORACLE_PARAMS.save(deps.storage, &cached_params)?;

    Ok(Response::new()
        .add_attribute("method", "refresh_oracle_params")
        .add_attribute("max_deviation_bps", cached_params.max_price_deviation_bps.to_string())
        .add_attribute("max_staleness_blocks", cached_params.max_price_staleness_blocks.to_string())
        .add_attribute("min_price_sources", cached_params.min_price_sources.to_string())
        .add_attribute("twap_window", cached_params.twap_window.to_string()))
}

pub fn execute_update_config(
    deps: DepsMut<AkashQuery>,
    info: MessageInfo,
    wormhole_contract: Option<String>,
    price_feed_id: Option<String>,
    data_sources: Option<Vec<DataSourceMsg>>,
) -> Result<Response, ContractError> {
    let mut config = CONFIG.load(deps.storage)?;

    // Only admin can update config
    if info.sender != config.admin {
        return Err(ContractError::Unauthorized {});
    }

    if let Some(wormhole) = wormhole_contract {
        config.wormhole_contract = deps.api.addr_validate(&wormhole)?;
    }

    if let Some(feed_id) = price_feed_id {
        config.price_feed_id = feed_id;
    }

    if let Some(sources) = data_sources {
        config.data_sources = sources
            .into_iter()
            .map(|ds| DataSource {
                emitter_chain: ds.emitter_chain,
                emitter_address: ds.emitter_address,
            })
            .collect();
    }

    CONFIG.save(deps.storage, &config)?;

    Ok(Response::new()
        .add_attribute("method", "update_config")
        .add_attribute("wormhole_contract", config.wormhole_contract.to_string())
        .add_attribute("price_feed_id", config.price_feed_id))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn query(deps: Deps<AkashQuery>, env: Env, msg: QueryMsg) -> StdResult<Binary> {
    match msg {
        QueryMsg::GetPrice {} => to_json_binary(&query_price(deps, env)?),
        QueryMsg::GetPriceFeed {} => to_json_binary(&query_price_feed(deps)?),
        QueryMsg::GetConfig {} => to_json_binary(&query_config(deps)?),
        QueryMsg::GetPriceFeedId {} => to_json_binary(&query_price_feed_id(deps)?),
        QueryMsg::GetOracleParams {} => to_json_binary(&query_oracle_params(deps)?),
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
        wormhole_contract: config.wormhole_contract.to_string(),
        update_fee: config.update_fee,
        price_feed_id: config.price_feed_id,
        default_denom: config.default_data_id.denom,
        default_base_denom: config.default_data_id.base_denom,
        data_sources: config
            .data_sources
            .into_iter()
            .map(|ds| DataSourceMsg {
                emitter_chain: ds.emitter_chain,
                emitter_address: ds.emitter_address,
            })
            .collect(),
    })
}

fn query_price_feed_id(deps: Deps<AkashQuery>) -> StdResult<PriceFeedIdResponse> {
    let config = CONFIG.load(deps.storage)?;

    Ok(PriceFeedIdResponse {
        price_feed_id: config.price_feed_id,
    })
}

fn query_oracle_params(deps: Deps<AkashQuery>) -> StdResult<OracleParamsResponse> {
    let cached_params = CACHED_ORACLE_PARAMS.load(deps.storage)?;

    Ok(OracleParamsResponse {
        max_price_deviation_bps: cached_params.max_price_deviation_bps,
        min_price_sources: cached_params.min_price_sources,
        max_price_staleness_blocks: cached_params.max_price_staleness_blocks,
        twap_window: cached_params.twap_window,
        last_updated_height: cached_params.last_updated_height,
    })
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn migrate(deps: DepsMut<AkashQuery>, env: Env, _msg: MigrateMsg) -> Result<Response, ContractError> {
    // Check if cached oracle params exist, if not initialize them
    if CACHED_ORACLE_PARAMS.may_load(deps.storage)?.is_none() {
        // Fetch params from chain during migration
        let oracle_params = fetch_oracle_params_from_chain(&deps.querier.into())?;

        let cached_params = CachedOracleParams {
            max_price_deviation_bps: oracle_params.max_price_deviation_bps,
            min_price_sources: oracle_params.min_price_sources,
            max_price_staleness_blocks: oracle_params.max_price_staleness_blocks,
            twap_window: oracle_params.twap_window,
            last_updated_height: env.block.height,
        };
        CACHED_ORACLE_PARAMS.save(deps.storage, &cached_params)?;
    }

    Ok(Response::new()
        .add_attribute("method", "migrate")
        .add_attribute("version", "3.0.0"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use cosmwasm_std::testing::{message_info, mock_env, MockApi, MockQuerier, MockStorage};
    use cosmwasm_std::{from_json, OwnedDeps};

    type MockDeps = OwnedDeps<MockStorage, MockApi, MockQuerier, AkashQuery>;

    fn mock_dependencies_with_akash_query() -> MockDeps {
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
            wormhole_contract: deps.api.addr_make("wormhole"),
            update_fee: Uint256::from(1000u128),
            price_feed_id: "0xtest123".to_string(),
            default_data_id: DataID::akt_usd(),
            data_sources: vec![DataSource {
                emitter_chain: 26,
                emitter_address: "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71".to_string(),
            }],
        };
        CONFIG.save(&mut deps.storage, &config).unwrap();

        let cached_params = CachedOracleParams::default();
        CACHED_ORACLE_PARAMS.save(&mut deps.storage, &cached_params).unwrap();
    }

    #[test]
    fn test_update_fee() {
        let mut deps = mock_dependencies_with_akash_query();
        setup_config(&mut deps);

        let msg = ExecuteMsg::UpdateFee {
            new_fee: Uint256::from(2000u128),
        };
        let info = message_info(&deps.api.addr_make("admin"), &[]);
        let res = execute(deps.as_mut(), mock_env(), info, msg).unwrap();
        assert_eq!(2, res.attributes.len());

        let config: ConfigResponse =
            from_json(query(deps.as_ref(), mock_env(), QueryMsg::GetConfig {}).unwrap())
                .unwrap();
        assert_eq!(Uint256::from(2000u128), config.update_fee);
    }

    #[test]
    fn test_query_price_feed_id() {
        let mut deps = mock_dependencies_with_akash_query();
        setup_config(&mut deps);

        // Update config with specific price feed id
        let mut config = CONFIG.load(&deps.storage).unwrap();
        config.price_feed_id = "0xabc123def456".to_string();
        CONFIG.save(&mut deps.storage, &config).unwrap();

        let response: PriceFeedIdResponse = from_json(
            query(
                deps.as_ref(),
                mock_env(),
                QueryMsg::GetPriceFeedId {},
            )
            .unwrap(),
        )
        .unwrap();

        assert_eq!("0xabc123def456", response.price_feed_id);
    }

    #[test]
    fn test_query_oracle_params() {
        let mut deps = mock_dependencies_with_akash_query();
        setup_config(&mut deps);

        let response: OracleParamsResponse = from_json(
            query(
                deps.as_ref(),
                mock_env(),
                QueryMsg::GetOracleParams {},
            )
            .unwrap(),
        )
        .unwrap();

        // Check default values
        assert_eq!(150, response.max_price_deviation_bps);
        assert_eq!(2, response.min_price_sources);
        assert_eq!(50, response.max_price_staleness_blocks);
        assert_eq!(50, response.twap_window);
    }

    #[test]
    fn test_query_config_includes_wormhole() {
        let mut deps = mock_dependencies_with_akash_query();
        setup_config(&mut deps);

        let response: ConfigResponse = from_json(
            query(
                deps.as_ref(),
                mock_env(),
                QueryMsg::GetConfig {},
            )
            .unwrap(),
        )
        .unwrap();

        // Oracle module expects "akt" (not "uakt") for denom
        assert_eq!("akt", response.default_denom);
        assert_eq!("usd", response.default_base_denom);
        assert!(!response.wormhole_contract.is_empty());
        assert_eq!(1, response.data_sources.len());
        assert_eq!(26, response.data_sources[0].emitter_chain);
    }
}
