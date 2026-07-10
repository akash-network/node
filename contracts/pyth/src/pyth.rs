use cosmwasm_std::StdError;

/// Pyth price attestation magic bytes "P2WH"
pub const PYTH_MAGIC: &[u8] = b"P2WH";

/// Parsed Pyth price data from VAA payload
#[derive(Debug, Clone)]
pub struct PythPrice {
    /// Price feed ID (32 bytes, hex encoded)
    pub id: String,
    /// Price value (scaled by 10^expo)
    pub price: i64,
    /// Confidence interval
    pub conf: u64,
    /// Price exponent (e.g., -8 means divide by 10^8)
    pub expo: i32,
    /// Unix timestamp when price was published
    pub publish_time: i64,
    /// Exponential moving average price
    pub ema_price: i64,
    /// EMA confidence interval
    pub ema_conf: u64,
}

/// Parse Pyth price attestation from VAA payload
///
/// The Pyth Hermes API returns price updates in a specific binary format.
/// This parser extracts the price data from the VAA payload.
///
/// Reference: https://github.com/pyth-network/pyth-crosschain
pub fn parse_pyth_payload(payload: &[u8]) -> Result<PythPrice, StdError> {
    // Minimum payload size check
    if payload.len() < 4 {
        return Err(StdError::msg("Payload too short"));
    }

    // Check magic bytes "P2WH" for Pyth-to-Wormhole format
    if &payload[0..4] == PYTH_MAGIC {
        return parse_p2wh_format(payload);
    }

    // Try parsing as accumulator/merkle format (newer Hermes API)
    // The accumulator format starts with different magic bytes
    if payload.len() >= 4 && &payload[0..4] == b"AUWV" {
        return parse_accumulator_format(payload);
    }

    // Fallback: try to parse as raw price update
    parse_raw_price_update(payload)
}

/// Parse P2WH (Pyth-to-Wormhole) format
/// This is the batch price attestation format
fn parse_p2wh_format(payload: &[u8]) -> Result<PythPrice, StdError> {
    // P2WH format:
    // 0-4: magic "P2WH"
    // 4-6: major version (u16)
    // 6-8: minor version (u16)
    // 8-10: header size (u16)
    // 10-11: payload type (u8)
    // ... attestation data follows

    if payload.len() < 11 {
        return Err(StdError::msg("P2WH payload too short"));
    }

    let _major_version = u16::from_be_bytes([payload[4], payload[5]]);
    let _minor_version = u16::from_be_bytes([payload[6], payload[7]]);
    let header_size = u16::from_be_bytes([payload[8], payload[9]]) as usize;

    // Skip header to get to attestation data
    let attestation_start = 4 + header_size;
    if attestation_start >= payload.len() {
        return Err(StdError::msg("Invalid header size"));
    }

    let attestation_data = &payload[attestation_start..];

    // Parse batch attestation header
    // 0-2: number of attestations (u16)
    // 2-4: attestation size (u16)
    if attestation_data.len() < 4 {
        return Err(StdError::msg("Attestation data too short"));
    }

    let num_attestations = u16::from_be_bytes([attestation_data[0], attestation_data[1]]);
    let attestation_size = u16::from_be_bytes([attestation_data[2], attestation_data[3]]) as usize;

    if num_attestations == 0 {
        return Err(StdError::msg("No attestations in payload"));
    }

    // Parse first attestation (we only need one price)
    let first_attestation_start = 4;
    if first_attestation_start + attestation_size > attestation_data.len() {
        return Err(StdError::msg("Attestation data truncated"));
    }

    let attestation = &attestation_data[first_attestation_start..first_attestation_start + attestation_size];
    parse_single_attestation(attestation)
}

/// Parse a single price attestation
/// Format (150 bytes total):
/// 0-32: product_id
/// 32-64: price_id
/// 64-72: price (i64)
/// 72-80: conf (u64)
/// 80-84: expo (i32)
/// 84-92: ema_price (i64)
/// 92-100: ema_conf (u64)
/// 100-101: status (u8)
/// ... more fields follow
/// 134-142: publish_time (i64)
fn parse_single_attestation(attestation: &[u8]) -> Result<PythPrice, StdError> {
    if attestation.len() < 142 {
        return Err(StdError::msg(format!(
            "Attestation too short: {} bytes, need at least 142",
            attestation.len()
        )));
    }

    // Extract price feed ID (bytes 32-64)
    let id = hex::encode(&attestation[32..64]);

    // Extract price (i64, big-endian, bytes 64-72)
    let price = i64::from_be_bytes([
        attestation[64], attestation[65], attestation[66], attestation[67],
        attestation[68], attestation[69], attestation[70], attestation[71],
    ]);

    // Extract confidence (u64, big-endian, bytes 72-80)
    let conf = u64::from_be_bytes([
        attestation[72], attestation[73], attestation[74], attestation[75],
        attestation[76], attestation[77], attestation[78], attestation[79],
    ]);

    // Extract exponent (i32, big-endian, bytes 80-84)
    let expo = i32::from_be_bytes([
        attestation[80], attestation[81], attestation[82], attestation[83],
    ]);

    // Extract EMA price (i64, big-endian, bytes 84-92)
    let ema_price = i64::from_be_bytes([
        attestation[84], attestation[85], attestation[86], attestation[87],
        attestation[88], attestation[89], attestation[90], attestation[91],
    ]);

    // Extract EMA conf (u64, big-endian, bytes 92-100)
    let ema_conf = u64::from_be_bytes([
        attestation[92], attestation[93], attestation[94], attestation[95],
        attestation[96], attestation[97], attestation[98], attestation[99],
    ]);

    // Extract publish_time (i64, big-endian, bytes 134-142)
    let publish_time = i64::from_be_bytes([
        attestation[134], attestation[135], attestation[136], attestation[137],
        attestation[138], attestation[139], attestation[140], attestation[141],
    ]);

    Ok(PythPrice {
        id,
        price,
        conf,
        expo,
        publish_time,
        ema_price,
        ema_conf,
    })
}

/// Parse accumulator/merkle format (newer Hermes API format)
/// Note: This is for VAA payloads that contain Merkle roots.
/// For PNAU price feed messages, use `parse_price_feed_message` instead.
fn parse_accumulator_format(_payload: &[u8]) -> Result<PythPrice, StdError> {
    // The accumulator format in VAA payload contains Merkle root, not price data.
    // Price data comes from the PNAU message data, parsed via parse_price_feed_message.
    Err(StdError::msg(
        "VAA payload contains Merkle root. Use PNAU format with parse_price_feed_message."
    ))
}

/// Parse a price feed message from PNAU accumulator update
///
/// This parses the message_data from a PriceUpdateWithProof that has been
/// Merkle-verified against the root signed by Wormhole guardians.
///
/// Message format (from Pyth SDK):
/// - Message type (1 byte): 0 = price feed
/// - Price feed ID (32 bytes)
/// - Price (i64, 8 bytes)
/// - Confidence (u64, 8 bytes)
/// - Exponent (i32, 4 bytes)
/// - Publish time (i64, 8 bytes)
/// - Previous publish time (i64, 8 bytes)
/// - EMA price (i64, 8 bytes)
/// - EMA conf (u64, 8 bytes)
pub fn parse_price_feed_message(data: &[u8]) -> Result<PythPrice, StdError> {
    // Minimum size: 1 + 32 + 8 + 8 + 4 + 8 + 8 + 8 + 8 = 85 bytes
    if data.len() < 85 {
        return Err(StdError::msg(format!(
            "Price feed message too short: {} bytes, need at least 85",
            data.len()
        )));
    }

    let message_type = data[0];
    if message_type != 0 {
        return Err(StdError::msg(format!(
            "Invalid message type: {}, expected 0 (price feed)",
            message_type
        )));
    }

    // Price feed ID (bytes 1-33) - add 0x prefix to match config format
    let id = format!("0x{}", hex::encode(&data[1..33]));

    // Price (i64, bytes 33-41)
    let price = i64::from_be_bytes([
        data[33], data[34], data[35], data[36],
        data[37], data[38], data[39], data[40],
    ]);

    // Confidence (u64, bytes 41-49)
    let conf = u64::from_be_bytes([
        data[41], data[42], data[43], data[44],
        data[45], data[46], data[47], data[48],
    ]);

    // Exponent (i32, bytes 49-53)
    let expo = i32::from_be_bytes([
        data[49], data[50], data[51], data[52],
    ]);

    // Publish time (i64, bytes 53-61)
    let publish_time = i64::from_be_bytes([
        data[53], data[54], data[55], data[56],
        data[57], data[58], data[59], data[60],
    ]);

    // Previous publish time (i64, bytes 61-69) - skipped
    // let _prev_publish_time = ...

    // EMA price (i64, bytes 69-77)
    let ema_price = i64::from_be_bytes([
        data[69], data[70], data[71], data[72],
        data[73], data[74], data[75], data[76],
    ]);

    // EMA conf (u64, bytes 77-85)
    let ema_conf = u64::from_be_bytes([
        data[77], data[78], data[79], data[80],
        data[81], data[82], data[83], data[84],
    ]);

    Ok(PythPrice {
        id,
        price,
        conf,
        expo,
        publish_time,
        ema_price,
        ema_conf,
    })
}

/// Parse raw price update format (fallback)
/// This is for testing or when price data is provided directly
fn parse_raw_price_update(payload: &[u8]) -> Result<PythPrice, StdError> {
    // Expected format for raw updates:
    // 0-32: price_feed_id
    // 32-40: price (i64)
    // 40-48: conf (u64)
    // 48-52: expo (i32)
    // 52-60: publish_time (i64)

    if payload.len() < 60 {
        return Err(StdError::msg(format!(
            "Raw payload too short: {} bytes, need at least 60",
            payload.len()
        )));
    }

    // Add 0x prefix to match config format
    let id = format!("0x{}", hex::encode(&payload[0..32]));

    let price = i64::from_be_bytes([
        payload[32], payload[33], payload[34], payload[35],
        payload[36], payload[37], payload[38], payload[39],
    ]);

    let conf = u64::from_be_bytes([
        payload[40], payload[41], payload[42], payload[43],
        payload[44], payload[45], payload[46], payload[47],
    ]);

    let expo = i32::from_be_bytes([
        payload[48], payload[49], payload[50], payload[51],
    ]);

    let publish_time = i64::from_be_bytes([
        payload[52], payload[53], payload[54], payload[55],
        payload[56], payload[57], payload[58], payload[59],
    ]);

    Ok(PythPrice {
        id,
        price,
        conf,
        expo,
        publish_time,
        ema_price: price,  // Use same as current price
        ema_conf: conf,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_raw_price_update() {
        // Create a test payload with known values
        let mut payload = vec![0u8; 60];

        // Price feed ID (32 bytes of 0xAB)
        for i in 0..32 {
            payload[i] = 0xAB;
        }

        // Price: 123456789 (i64)
        let price: i64 = 123456789;
        payload[32..40].copy_from_slice(&price.to_be_bytes());

        // Conf: 1000 (u64)
        let conf: u64 = 1000;
        payload[40..48].copy_from_slice(&conf.to_be_bytes());

        // Expo: -8 (i32)
        let expo: i32 = -8;
        payload[48..52].copy_from_slice(&expo.to_be_bytes());

        // Publish time: 1704067200 (i64)
        let publish_time: i64 = 1704067200;
        payload[52..60].copy_from_slice(&publish_time.to_be_bytes());

        let result = parse_pyth_payload(&payload).unwrap();

        assert_eq!(result.price, 123456789);
        assert_eq!(result.conf, 1000);
        assert_eq!(result.expo, -8);
        assert_eq!(result.publish_time, 1704067200);
        assert_eq!(result.id, format!("0x{}", "ab".repeat(32)));
    }

    #[test]
    fn test_payload_too_short() {
        let payload = vec![0u8; 10];
        let result = parse_pyth_payload(&payload);
        assert!(result.is_err());
    }

    #[test]
    fn test_parse_price_feed_message() {
        // Create a test price feed message (85 bytes minimum)
        let mut message = vec![0u8; 85];

        // Message type: 0 (price feed)
        message[0] = 0;

        // Price feed ID (bytes 1-33, 32 bytes of 0xEF)
        for i in 1..33 {
            message[i] = 0xEF;
        }

        // Price: 234567890 (i64, bytes 33-41)
        let price: i64 = 234567890;
        message[33..41].copy_from_slice(&price.to_be_bytes());

        // Conf: 2000 (u64, bytes 41-49)
        let conf: u64 = 2000;
        message[41..49].copy_from_slice(&conf.to_be_bytes());

        // Expo: -8 (i32, bytes 49-53)
        let expo: i32 = -8;
        message[49..53].copy_from_slice(&expo.to_be_bytes());

        // Publish time: 1704153600 (i64, bytes 53-61)
        let publish_time: i64 = 1704153600;
        message[53..61].copy_from_slice(&publish_time.to_be_bytes());

        // Previous publish time (bytes 61-69) - just zeros

        // EMA price: 234000000 (i64, bytes 69-77)
        let ema_price: i64 = 234000000;
        message[69..77].copy_from_slice(&ema_price.to_be_bytes());

        // EMA conf: 1500 (u64, bytes 77-85)
        let ema_conf: u64 = 1500;
        message[77..85].copy_from_slice(&ema_conf.to_be_bytes());

        let result = parse_price_feed_message(&message).unwrap();

        assert_eq!(result.id, format!("0x{}", "ef".repeat(32)));
        assert_eq!(result.price, 234567890);
        assert_eq!(result.conf, 2000);
        assert_eq!(result.expo, -8);
        assert_eq!(result.publish_time, 1704153600);
        assert_eq!(result.ema_price, 234000000);
        assert_eq!(result.ema_conf, 1500);
    }

    #[test]
    fn test_parse_price_feed_message_invalid_type() {
        let mut message = vec![0u8; 85];
        message[0] = 1; // Invalid type

        let result = parse_price_feed_message(&message);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("Invalid message type"));
    }

    #[test]
    fn test_parse_price_feed_message_too_short() {
        let message = vec![0u8; 50]; // Too short

        let result = parse_price_feed_message(&message);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("too short"));
    }
}
