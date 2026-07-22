use cosmwasm_std::testing::{message_info, mock_env, MockApi, MockQuerier, MockStorage};
use cosmwasm_std::{from_json, Addr, Empty, OwnedDeps, Uint128, Uint256};

use crate::accumulator::{parse_accumulator_update, PNAU_MAGIC};
use crate::contract::{execute, query};
use crate::msg::{
    ConfigResponse, ExecuteMsg, PriceFeedIdResponse, PriceFeedResponse, PriceResponse, QueryMsg,
};
use crate::oracle::{pyth_price_to_decimal, DataID as OracleDataID, MsgAddPriceEntry};
use crate::state::{Config, DataID, PriceFeed, CONFIG, PRICE_FEED};

type MockDeps = OwnedDeps<MockStorage, MockApi, MockQuerier, Empty>;

fn mock_deps() -> MockDeps {
    OwnedDeps {
        storage: MockStorage::default(),
        api: MockApi::default(),
        querier: MockQuerier::default(),
        custom_query_type: std::marker::PhantomData,
    }
}

fn setup_contract(deps: &mut MockDeps) -> Addr {
    let admin = deps.api.addr_make("admin");

    CONFIG
        .save(
            &mut deps.storage,
            &Config {
                admin: admin.clone(),
                pyth_vaa_contract: deps.api.addr_make("pythvaa"),
                update_fee: Uint256::from(1000u128),
                price_feed_id: "0xtest_pyth_price_feed_id".to_string(),
                default_data_id: DataID::akt_usd(),
            },
        )
        .unwrap();

    PRICE_FEED
        .save(&mut deps.storage, &PriceFeed::new())
        .unwrap();

    admin
}

fn simulate_price_update(deps: &mut MockDeps, price: u128, conf: u128, publish_time: i64) {
    let mut price_feed = PRICE_FEED.load(&deps.storage).unwrap();
    price_feed.prev_publish_time = price_feed.publish_time;
    price_feed.price = Uint128::new(price);
    price_feed.conf = Uint128::new(conf);
    price_feed.expo = -8;
    price_feed.publish_time = publish_time;
    PRICE_FEED.save(&mut deps.storage, &price_feed).unwrap();
}

#[test]
fn e2e_query_initial_state() {
    let mut deps = mock_deps();
    let admin = setup_contract(&mut deps);
    let env = mock_env();

    let config: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();
    assert_eq!(config.admin, admin.to_string());
    assert_eq!(
        config.pyth_vaa_contract,
        deps.api.addr_make("pythvaa").to_string()
    );
    assert_eq!(config.default_denom, "akt");
    assert_eq!(config.default_base_denom, "usd");

    let price: PriceResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetPrice {}).unwrap()).unwrap();
    assert_eq!(price.price, Uint128::zero());
    assert_eq!(price.expo, -8);

    let price_feed: PriceFeedResponse =
        from_json(query(deps.as_ref(), env, QueryMsg::GetPriceFeed {}).unwrap()).unwrap();
    assert_eq!(price_feed.symbol, "AKT/USD");
    assert_eq!(price_feed.price, Uint128::zero());
}

#[test]
fn e2e_query_after_price_update() {
    let mut deps = mock_deps();
    setup_contract(&mut deps);
    let env = mock_env();
    let current_time = env.block.time.seconds() as i64;

    simulate_price_update(&mut deps, 52468300, 100000, current_time);

    let price: PriceResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetPrice {}).unwrap()).unwrap();
    assert_eq!(price.price, Uint128::new(52468300));
    assert_eq!(price.conf, Uint128::new(100000));
    assert_eq!(price.expo, -8);
    assert_eq!(price.publish_time, current_time);

    let price_feed: PriceFeedResponse =
        from_json(query(deps.as_ref(), env, QueryMsg::GetPriceFeed {}).unwrap()).unwrap();
    assert_eq!(price_feed.price, Uint128::new(52468300));
    assert_eq!(price_feed.publish_time, current_time);
}

#[test]
fn e2e_admin_operations_flow() {
    let mut deps = mock_deps();
    let admin = setup_contract(&mut deps);
    let env = mock_env();
    let admin_info = message_info(&admin, &[]);

    execute(
        deps.as_mut(),
        env.clone(),
        admin_info.clone(),
        ExecuteMsg::UpdateFee {
            new_fee: Uint256::from(5000u128),
        },
    )
    .unwrap();

    let config: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();
    assert_eq!(config.update_fee, Uint256::from(5000u128));

    let new_admin = deps.api.addr_make("new_admin");
    execute(
        deps.as_mut(),
        env.clone(),
        admin_info,
        ExecuteMsg::TransferAdmin {
            new_admin: new_admin.to_string(),
        },
    )
    .unwrap();

    let config: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();
    assert_eq!(config.admin, new_admin.to_string());

    let err = execute(
        deps.as_mut(),
        env,
        message_info(&admin, &[]),
        ExecuteMsg::UpdateFee {
            new_fee: Uint256::from(10000u128),
        },
    )
    .unwrap_err();
    assert!(err.to_string().contains("Unauthorized"));
}

#[test]
fn e2e_update_config() {
    let mut deps = mock_deps();
    let admin = setup_contract(&mut deps);
    let env = mock_env();

    let new_pyth_vaa = deps.api.addr_make("newpythvaa");
    execute(
        deps.as_mut(),
        env.clone(),
        message_info(&admin, &[]),
        ExecuteMsg::UpdateConfig {
            pyth_vaa_contract: Some(new_pyth_vaa.to_string()),
            price_feed_id: Some("0xnew_price_feed_id".to_string()),
        },
    )
    .unwrap();

    let config: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();
    assert_eq!(config.price_feed_id, "0xnew_price_feed_id");
    assert_eq!(config.pyth_vaa_contract, new_pyth_vaa.to_string());

    let err = execute(
        deps.as_mut(),
        env,
        message_info(&admin, &[]),
        ExecuteMsg::UpdateConfig {
            pyth_vaa_contract: None,
            price_feed_id: Some(String::new()),
        },
    )
    .unwrap_err();
    assert!(err.to_string().contains("price_feed_id is required"));
}

#[test]
fn e2e_oracle_message_encoding() {
    let msg = MsgAddPriceEntry::new(
        "akash1abc123def456".to_string(),
        "akt".to_string(),
        "usd".to_string(),
        "524683000000000000".to_string(),
        1234567890,
        123456,
    );

    let binary = msg.encode_to_protobuf();
    assert!(!binary.is_empty());
    assert_eq!(binary[0], 0x0a);
}

#[test]
fn e2e_price_conversion() {
    assert_eq!(pyth_price_to_decimal(52468300, -8), "524683000000000000");
    assert_eq!(pyth_price_to_decimal(123456789, -8), "1234567890000000000");
    assert_eq!(pyth_price_to_decimal(0, -8), "0");
}

#[test]
fn e2e_data_id_structure() {
    let data_id = OracleDataID::akt_usd();
    assert_eq!(data_id.denom, "akt");
    assert_eq!(data_id.base_denom, "usd");
}

#[test]
fn e2e_get_price_feed_id() {
    let mut deps = mock_deps();
    setup_contract(&mut deps);

    let feed_id: PriceFeedIdResponse =
        from_json(query(deps.as_ref(), mock_env(), QueryMsg::GetPriceFeedId {}).unwrap()).unwrap();
    assert_eq!(feed_id.price_feed_id, "0xtest_pyth_price_feed_id");
}

#[test]
fn e2e_accumulator_parsing_rejects_invalid_data() {
    assert_eq!(PNAU_MAGIC, b"PNAU");
    assert!(parse_accumulator_update(b"PNA").is_err());
    assert!(parse_accumulator_update(b"TEST0100").is_err());
}
