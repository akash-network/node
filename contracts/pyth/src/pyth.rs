use cosmwasm_std::StdError;

/// Parsed Pyth price data from a PNAU price feed message.
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

/// Parse a price feed message from PNAU accumulator update
///
/// This parses the message_data from a PriceUpdateWithProof that has been
/// Merkle-verified against the root signed by the Pyth router quorum.
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
        data[33], data[34], data[35], data[36], data[37], data[38], data[39], data[40],
    ]);

    // Confidence (u64, bytes 41-49)
    let conf = u64::from_be_bytes([
        data[41], data[42], data[43], data[44], data[45], data[46], data[47], data[48],
    ]);

    // Exponent (i32, bytes 49-53)
    let expo = i32::from_be_bytes([data[49], data[50], data[51], data[52]]);

    // Publish time (i64, bytes 53-61)
    let publish_time = i64::from_be_bytes([
        data[53], data[54], data[55], data[56], data[57], data[58], data[59], data[60],
    ]);

    // Previous publish time (i64, bytes 61-69) - skipped
    // let _prev_publish_time = ...

    // EMA price (i64, bytes 69-77)
    let ema_price = i64::from_be_bytes([
        data[69], data[70], data[71], data[72], data[73], data[74], data[75], data[76],
    ]);

    // EMA conf (u64, bytes 77-85)
    let ema_conf = u64::from_be_bytes([
        data[77], data[78], data[79], data[80], data[81], data[82], data[83], data[84],
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_price_feed_message() {
        // Create a test price feed message (85 bytes minimum)
        let mut message = vec![0u8; 85];

        // Message type: 0 (price feed)
        message[0] = 0;

        // Price feed ID (bytes 1-33, 32 bytes of 0xEF)
        message[1..33].fill(0xEF);

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
        assert!(result
            .unwrap_err()
            .to_string()
            .contains("Invalid message type"));
    }

    #[test]
    fn test_parse_price_feed_message_too_short() {
        let message = vec![0u8; 50]; // Too short

        let result = parse_price_feed_message(&message);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("too short"));
    }
}
