// integration_tests.rs - End-to-end integration tests for the price oracle contract
//
// These tests verify the contract logic by testing query responses, admin operations,
// and validation logic using direct state manipulation.
//
// Note: Price update execution requires Wormhole VAA verification which cannot be
// mocked in unit tests. Those flows are tested via the contract's unit tests and
// actual chain integration tests.

#![cfg(test)]

use cosmwasm_std::testing::{message_info, mock_env, MockApi, MockQuerier, MockStorage};
use cosmwasm_std::{from_json, Addr, OwnedDeps, Uint128, Uint256};

use crate::contract::{execute, query};
use crate::msg::{
    ConfigResponse, ExecuteMsg, OracleParamsResponse, PriceFeedIdResponse, PriceFeedResponse,
    PriceResponse, QueryMsg,
};
use crate::oracle::{pyth_price_to_decimal, MsgAddPriceEntry};
use crate::querier::AkashQuery;
use crate::state::{
    CachedOracleParams, Config, DataID, DataSource, PriceFeed,
    CACHED_ORACLE_PARAMS, CONFIG, PRICE_FEED,
};

type MockDeps = OwnedDeps<MockStorage, MockApi, MockQuerier, AkashQuery>;

/// Create mock dependencies with AkashQuery support
fn mock_deps() -> MockDeps {
    OwnedDeps {
        storage: MockStorage::default(),
        api: MockApi::default(),
        querier: MockQuerier::default(),
        custom_query_type: std::marker::PhantomData,
    }
}

/// Set up a fully configured contract state for testing
fn setup_contract(deps: &mut MockDeps) -> Addr {
    let admin = deps.api.addr_make("admin");
    let wormhole = deps.api.addr_make("wormhole");

    let config = Config {
        admin: admin.clone(),
        wormhole_contract: wormhole,
        update_fee: Uint256::from(1000u128),
        price_feed_id: "0xtest_pyth_price_feed_id".to_string(),
        default_data_id: DataID::akt_usd(),
        data_sources: vec![DataSource {
            emitter_chain: 26,
            emitter_address: "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71".to_string(),
        }],
    };
    CONFIG.save(&mut deps.storage, &config).unwrap();

    let cached_params = CachedOracleParams {
        max_price_deviation_bps: 150, // 1.5%
        min_price_sources: 2,
        max_price_staleness_blocks: 50,
        twap_window: 50,
        last_updated_height: 12345,
    };
    CACHED_ORACLE_PARAMS.save(&mut deps.storage, &cached_params).unwrap();

    let price_feed = PriceFeed::new();
    PRICE_FEED.save(&mut deps.storage, &price_feed).unwrap();

    admin
}

/// Helper to simulate a price update by directly modifying state
/// (Used to test query responses without needing Wormhole mock)
fn simulate_price_update(deps: &mut MockDeps, price: u128, conf: u128, publish_time: i64) {
    let mut price_feed = PRICE_FEED.load(&deps.storage).unwrap();
    price_feed.prev_publish_time = price_feed.publish_time;
    price_feed.price = Uint128::new(price);
    price_feed.conf = Uint128::new(conf);
    price_feed.expo = -8;
    price_feed.publish_time = publish_time;
    PRICE_FEED.save(&mut deps.storage, &price_feed).unwrap();
}

// ============================================================================
// E2E Test: Query initial state
// ============================================================================

#[test]
fn e2e_query_initial_state() {
    let mut deps = mock_deps();
    let admin = setup_contract(&mut deps);
    let env = mock_env();

    // Verify config
    let config: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();
    assert_eq!(config.admin, admin.to_string());
    // Oracle module expects "akt" (not "uakt") for denom
    assert_eq!(config.default_denom, "akt");
    assert_eq!(config.default_base_denom, "usd");
    assert!(!config.wormhole_contract.is_empty());
    assert_eq!(config.data_sources.len(), 1);
    assert_eq!(config.data_sources[0].emitter_chain, 26);

    // Query initial price (should be zero)
    let price: PriceResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetPrice {}).unwrap()).unwrap();
    assert_eq!(price.price, Uint128::zero());
    assert_eq!(price.expo, -8);

    // Query price feed
    let price_feed: PriceFeedResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetPriceFeed {}).unwrap()).unwrap();
    assert_eq!(price_feed.symbol, "AKT/USD");
    assert_eq!(price_feed.price, Uint128::zero());
}

// ============================================================================
// E2E Test: Query after simulated price update
// ============================================================================

#[test]
fn e2e_query_after_price_update() {
    let mut deps = mock_deps();
    let _admin = setup_contract(&mut deps);
    let env = mock_env();
    let current_time = env.block.time.seconds() as i64;

    // Simulate a price update
    simulate_price_update(&mut deps, 52468300, 100000, current_time);

    // Query updated price
    let price: PriceResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetPrice {}).unwrap()).unwrap();
    assert_eq!(price.price, Uint128::new(52468300));
    assert_eq!(price.conf, Uint128::new(100000));
    assert_eq!(price.expo, -8);
    assert_eq!(price.publish_time, current_time);

    // Query full price feed
    let price_feed: PriceFeedResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetPriceFeed {}).unwrap()).unwrap();
    assert_eq!(price_feed.symbol, "AKT/USD");
    assert_eq!(price_feed.price, Uint128::new(52468300));
    assert_eq!(price_feed.conf, Uint128::new(100000));
    assert_eq!(price_feed.publish_time, current_time);
}

// ============================================================================
// E2E Test: Sequential price updates tracking
// ============================================================================

#[test]
fn e2e_sequential_price_updates_tracking() {
    let mut deps = mock_deps();
    let _admin = setup_contract(&mut deps);
    let env = mock_env();
    let base_time = env.block.time.seconds() as i64;

    // First update
    simulate_price_update(&mut deps, 50000000, 100000, base_time);

    // Second update
    simulate_price_update(&mut deps, 51000000, 110000, base_time + 10);

    // Third update
    simulate_price_update(&mut deps, 52000000, 120000, base_time + 20);

    // Verify final state with prev_publish_time tracking
    let price_feed: PriceFeedResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetPriceFeed {}).unwrap()).unwrap();
    assert_eq!(price_feed.price, Uint128::new(52000000));
    assert_eq!(price_feed.publish_time, base_time + 20);
    assert_eq!(price_feed.prev_publish_time, base_time + 10);
}

// ============================================================================
// E2E Test: Oracle params flow
// ============================================================================

#[test]
fn e2e_oracle_params_flow() {
    let mut deps = mock_deps();
    let _admin = setup_contract(&mut deps);
    let env = mock_env();

    // Query oracle params
    let params: OracleParamsResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetOracleParams {}).unwrap())
            .unwrap();

    // Verify default params are cached
    assert_eq!(params.max_price_deviation_bps, 150);
    assert_eq!(params.min_price_sources, 2);
    assert_eq!(params.max_price_staleness_blocks, 50);
    assert_eq!(params.twap_window, 50);
    assert_eq!(params.last_updated_height, 12345);
}

// ============================================================================
// E2E Test: Admin operations flow
// ============================================================================

#[test]
fn e2e_admin_operations_flow() {
    let mut deps = mock_deps();
    let admin = setup_contract(&mut deps);
    let env = mock_env();

    // Step 1: Update fee as admin
    let admin_info = message_info(&admin, &[]);
    let update_fee_msg = ExecuteMsg::UpdateFee {
        new_fee: Uint256::from(5000u128),
    };

    let res = execute(deps.as_mut(), env.clone(), admin_info.clone(), update_fee_msg).unwrap();
    assert!(res
        .attributes
        .iter()
        .any(|a| a.key == "new_fee" && a.value == "5000"));

    // Verify fee updated
    let config: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();
    assert_eq!(config.update_fee, Uint256::from(5000u128));

    // Step 2: Transfer admin
    let new_admin = deps.api.addr_make("new_admin");
    let transfer_msg = ExecuteMsg::TransferAdmin {
        new_admin: new_admin.to_string(),
    };

    let res = execute(deps.as_mut(), env.clone(), admin_info, transfer_msg).unwrap();
    assert!(res
        .attributes
        .iter()
        .any(|a| a.key == "new_admin" && a.value == new_admin.to_string()));

    // Step 3: Verify new admin
    let config: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();
    assert_eq!(config.admin, new_admin.to_string());

    // Step 4: Old admin cannot update fee anymore
    let old_admin_info = message_info(&admin, &[]);
    let update_fee_msg = ExecuteMsg::UpdateFee {
        new_fee: Uint256::from(10000u128),
    };

    let res = execute(deps.as_mut(), env.clone(), old_admin_info, update_fee_msg);
    assert!(res.is_err());
}

// ============================================================================
// E2E Test: Oracle message encoding
// ============================================================================

#[test]
fn e2e_oracle_message_encoding() {
    // Test that the MsgAddPriceEntry encoding works correctly
    // Note: Oracle module expects "akt" (not "uakt") for denom
    let msg = MsgAddPriceEntry::new(
        "akash1abc123def456".to_string(),
        "akt".to_string(),
        "usd".to_string(),
        "524683000000000000".to_string(), // LegacyDec format
        1234567890,
        123456,
    );

    let binary = msg.encode_to_protobuf();

    // Verify the binary is non-empty and starts with correct tag
    assert!(!binary.is_empty());
    assert_eq!(binary[0], 0x0a); // Field 1 tag for signer

    // Test with AKT/USD helper
    // Note: MsgAddPriceEntry takes the price as-is; conversion happens before calling this
    // Price should be in Cosmos LegacyDec format (18 decimal integer string)
    let msg = MsgAddPriceEntry::akt_usd(
        "akash1test".to_string(),
        "1234567890000000000".to_string(), // 1.23456789 in LegacyDec format
        1700000000,
    );

    assert_eq!(msg.id.denom, "akt");
    assert_eq!(msg.id.base_denom, "usd");
    assert_eq!(msg.price.price, "1234567890000000000");
    assert_eq!(msg.price.timestamp_seconds, 1700000000);
    assert_eq!(msg.price.timestamp_nanos, 0);

    let binary = msg.encode_to_protobuf();
    assert!(!binary.is_empty());
}

// ============================================================================
// E2E Test: Price conversion
// ============================================================================

#[test]
fn e2e_price_conversion() {
    // Test various price conversions to Cosmos LegacyDec format (18 decimals)
    //
    // Pyth price with exponent is converted to 18-decimal integer string.
    // Formula: result = price * 10^(18 + expo)

    // price=52468300, expo=-8 -> 0.52468300 -> 524683000000000000
    assert_eq!(pyth_price_to_decimal(52468300, -8), "524683000000000000");
    // price=123456789, expo=-8 -> 1.23456789 -> 1234567890000000000
    assert_eq!(pyth_price_to_decimal(123456789, -8), "1234567890000000000");
    // price=100000000, expo=-8 -> 1.0 -> 1000000000000000000
    assert_eq!(pyth_price_to_decimal(100000000, -8), "1000000000000000000");
    // price=1000000000, expo=-8 -> 10.0 -> 10000000000000000000
    assert_eq!(pyth_price_to_decimal(1000000000, -8), "10000000000000000000");
    // negative price
    assert_eq!(pyth_price_to_decimal(-52468300, -8), "-524683000000000000");
    // zero
    assert_eq!(pyth_price_to_decimal(0, -8), "0");

    // Test with different exponents
    // price=12345, expo=-4 -> 1.2345 -> 12345 * 10^14 = 1234500000000000000
    assert_eq!(pyth_price_to_decimal(12345, -4), "1234500000000000000");
    // price=12345, expo=-2 -> 123.45 -> 12345 * 10^16 = 123450000000000000000
    assert_eq!(pyth_price_to_decimal(12345, -2), "123450000000000000000");
    // price=12345, expo=0 -> 12345 -> 12345 * 10^18 = 12345000000000000000000
    assert_eq!(pyth_price_to_decimal(12345, 0), "12345000000000000000000");
    // price=12345, expo=2 -> 1234500 -> 12345 * 10^20 = 1234500000000000000000000
    assert_eq!(pyth_price_to_decimal(12345, 2), "1234500000000000000000000");
}

// ============================================================================
// E2E Test: Query responses match expected schema
// ============================================================================

#[test]
fn e2e_query_response_schema() {
    let mut deps = mock_deps();
    let _admin = setup_contract(&mut deps);
    let env = mock_env();

    // Test GetConfig response
    let config_response: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();

    // Verify all fields are present and correctly typed
    assert!(!config_response.admin.is_empty());
    assert!(!config_response.price_feed_id.is_empty());
    assert!(!config_response.wormhole_contract.is_empty());
    // Oracle module expects "akt" (not "uakt") for denom
    assert_eq!(config_response.default_denom, "akt");
    assert_eq!(config_response.default_base_denom, "usd");
    assert!(!config_response.data_sources.is_empty());

    // Test GetPriceFeedId response
    let feed_id_response: PriceFeedIdResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetPriceFeedId {}).unwrap()).unwrap();
    assert!(!feed_id_response.price_feed_id.is_empty());

    // Test GetOracleParams response
    let params_response: OracleParamsResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetOracleParams {}).unwrap())
            .unwrap();
    assert!(params_response.max_price_deviation_bps > 0);
    assert!(params_response.min_price_sources > 0);
    assert!(params_response.max_price_staleness_blocks > 0);
    assert!(params_response.twap_window > 0);
}

// ============================================================================
// E2E Test: DataID structure validation
// ============================================================================

#[test]
fn e2e_data_id_structure() {
    use crate::oracle::DataID as OracleDataID;

    // Test default AKT/USD pair
    // Note: Oracle module expects "akt" (not "uakt") for denom
    let data_id = OracleDataID::akt_usd();
    assert_eq!(data_id.denom, "akt");
    assert_eq!(data_id.base_denom, "usd");

    // Test custom pair
    let custom = OracleDataID::new("atom".to_string(), "usd".to_string());
    assert_eq!(custom.denom, "atom");
    assert_eq!(custom.base_denom, "usd");
}

// ============================================================================
// E2E Test: Protobuf encoding verification
// ============================================================================

#[test]
fn e2e_protobuf_encoding_verification() {
    // Create a message with known values
    // Note: Oracle module expects "akt" (not "uakt") for denom
    let msg = MsgAddPriceEntry::new(
        "akash1test".to_string(),
        "akt".to_string(),
        "usd".to_string(),
        "1000000000000000000".to_string(), // 1.0 in LegacyDec format
        1700000000,
        0,
    );

    let binary = msg.encode_to_protobuf();

    // Verify structure:
    // Field 1 (signer): tag 0x0a, length, "akash1test"
    // Field 2 (id): tag 0x12, length, DataID submessage
    // Field 3 (price): tag 0x1a, length, PriceDataState submessage

    assert_eq!(binary[0], 0x0a); // Field 1 tag

    // Find field 2 tag (0x12)
    let field2_pos = binary.iter().position(|&b| b == 0x12);
    assert!(field2_pos.is_some(), "Field 2 (id) tag not found");

    // Find field 3 tag (0x1a)
    let field3_pos = binary.iter().position(|&b| b == 0x1a);
    assert!(field3_pos.is_some(), "Field 3 (price) tag not found");

    // Verify field order
    assert!(field2_pos.unwrap() < field3_pos.unwrap());
}

// ============================================================================
// E2E Test: Price volatility scenario (query-based)
// ============================================================================

#[test]
fn e2e_price_volatility_scenario() {
    let mut deps = mock_deps();
    let _admin = setup_contract(&mut deps);
    let env = mock_env();

    let base_time = env.block.time.seconds() as i64;

    // Simulate price volatility over time
    let prices = vec![
        (50000000u128, base_time),
        (52000000u128, base_time + 10), // +4%
        (48000000u128, base_time + 20), // -7.7%
        (55000000u128, base_time + 30), // +14.6%
        (53000000u128, base_time + 40), // -3.6%
    ];

    for (price, time) in prices {
        simulate_price_update(&mut deps, price, 100000, time);
    }

    // Verify final state
    let price_feed: PriceFeedResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetPriceFeed {}).unwrap()).unwrap();
    assert_eq!(price_feed.price, Uint128::new(53000000));
    assert_eq!(price_feed.prev_publish_time, base_time + 30);
}

// ============================================================================
// E2E Test: Data source configuration
// ============================================================================

#[test]
fn e2e_data_source_configuration() {
    let mut deps = mock_deps();
    let _admin = setup_contract(&mut deps);
    let env = mock_env();

    // Query config to verify data sources
    let config: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();

    // Verify Pythnet data source is configured
    assert_eq!(config.data_sources.len(), 1);
    assert_eq!(config.data_sources[0].emitter_chain, 26); // Pythnet
    assert_eq!(
        config.data_sources[0].emitter_address,
        "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71"
    );
}

// ============================================================================
// E2E Test: DataSource matching logic
// ============================================================================

#[test]
fn e2e_data_source_matching() {
    let ds = DataSource {
        emitter_chain: 26,
        emitter_address: "e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71".to_string(),
    };

    // Test matching
    let valid_address = hex::decode("e101faedac5851e32b9b23b5f9411a8c2bac4aae3ed4dd7b811dd1a72ea4aa71").unwrap();
    assert!(ds.matches(26, &valid_address));

    // Test wrong chain
    assert!(!ds.matches(1, &valid_address));

    // Test wrong address
    let wrong_address = hex::decode("0000000000000000000000000000000000000000000000000000000000000000").unwrap();
    assert!(!ds.matches(26, &wrong_address));
}

// ============================================================================
// E2E Test: Update config (admin only)
// ============================================================================

#[test]
fn e2e_update_config() {
    let mut deps = mock_deps();
    let admin = setup_contract(&mut deps);
    let env = mock_env();

    // Update price feed ID
    let admin_info = message_info(&admin, &[]);
    let update_msg = ExecuteMsg::UpdateConfig {
        wormhole_contract: None,
        price_feed_id: Some("0xnew_price_feed_id".to_string()),
        data_sources: None,
    };

    let res = execute(deps.as_mut(), env.clone(), admin_info.clone(), update_msg).unwrap();
    assert!(res.attributes.iter().any(|a| a.key == "price_feed_id"));

    // Verify update
    let config: ConfigResponse =
        from_json(query(deps.as_ref(), env.clone(), QueryMsg::GetConfig {}).unwrap()).unwrap();
    assert_eq!(config.price_feed_id, "0xnew_price_feed_id");

    // Non-admin cannot update
    let non_admin = deps.api.addr_make("non_admin");
    let non_admin_info = message_info(&non_admin, &[]);
    let update_msg = ExecuteMsg::UpdateConfig {
        wormhole_contract: None,
        price_feed_id: Some("0xmalicious".to_string()),
        data_sources: None,
    };

    let res = execute(deps.as_mut(), env.clone(), non_admin_info, update_msg);
    assert!(res.is_err());
}

// ============================================================================
// E2E Test: Accumulator format parsing
// ============================================================================

#[test]
fn e2e_accumulator_parsing() {
    use crate::accumulator::{parse_accumulator_update, PNAU_MAGIC};

    // Test PNAU magic detection
    assert_eq!(PNAU_MAGIC, b"PNAU");

    // Test invalid data
    let too_short = b"PNA";
    assert!(parse_accumulator_update(too_short).is_err());

    let wrong_magic = b"TEST0100";
    assert!(parse_accumulator_update(wrong_magic).is_err());
}

// ============================================================================
// E2E Test: Price feed message parsing
// ============================================================================

#[test]
fn e2e_price_feed_message_parsing() {
    use crate::pyth::parse_price_feed_message;

    // Create a valid price feed message
    let mut message = vec![0u8; 85];
    message[0] = 0; // Message type: price feed

    // Price feed ID (bytes 1-33)
    for i in 1..33 {
        message[i] = 0xAB;
    }

    // Price: 52468300 (i64)
    let price: i64 = 52468300;
    message[33..41].copy_from_slice(&price.to_be_bytes());

    // Conf: 100000 (u64)
    let conf: u64 = 100000;
    message[41..49].copy_from_slice(&conf.to_be_bytes());

    // Expo: -8 (i32)
    let expo: i32 = -8;
    message[49..53].copy_from_slice(&expo.to_be_bytes());

    // Publish time: 1700000000 (i64)
    let publish_time: i64 = 1700000000;
    message[53..61].copy_from_slice(&publish_time.to_be_bytes());

    // EMA price (i64)
    let ema_price: i64 = 52400000;
    message[69..77].copy_from_slice(&ema_price.to_be_bytes());

    // EMA conf (u64)
    let ema_conf: u64 = 95000;
    message[77..85].copy_from_slice(&ema_conf.to_be_bytes());

    let result = parse_price_feed_message(&message).unwrap();
    assert_eq!(result.price, 52468300);
    assert_eq!(result.conf, 100000);
    assert_eq!(result.expo, -8);
    assert_eq!(result.publish_time, 1700000000);
    assert_eq!(result.ema_price, 52400000);
    assert_eq!(result.ema_conf, 95000);

    // Test invalid message type
    let mut invalid_type = message.clone();
    invalid_type[0] = 1; // Invalid type
    assert!(parse_price_feed_message(&invalid_type).is_err());

    // Test too short
    let too_short = vec![0u8; 50];
    assert!(parse_price_feed_message(&too_short).is_err());
}

// ============================================================================
// E2E Test: Merkle proof verification
// ============================================================================

#[test]
fn e2e_merkle_proof_verification() {
    use crate::accumulator::verify_merkle_proof;
    use sha3::{Digest, Keccak256};

    // Create a simple merkle tree for testing
    // Leaf: message data
    // Root: hash of leaf (for single-element tree)

    let message_data = b"test price data";

    // Compute leaf hash: keccak256(0x00 || message_data)[0..20]
    let mut hasher = Keccak256::new();
    hasher.update([0u8]); // MERKLE_LEAF_PREFIX
    hasher.update(message_data);
    let hash = hasher.finalize();
    let mut root = [0u8; 20];
    root.copy_from_slice(&hash[0..20]);

    // Empty proof for single-leaf tree
    let empty_proof: Vec<[u8; 20]> = vec![];

    // Verify should succeed with correct root
    assert!(verify_merkle_proof(message_data, &empty_proof, &root));

    // Verify should fail with wrong root
    let wrong_root = [0u8; 20];
    assert!(!verify_merkle_proof(message_data, &empty_proof, &wrong_root));

    // Verify should fail with wrong data
    assert!(!verify_merkle_proof(b"wrong data", &empty_proof, &root));
}
