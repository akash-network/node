// oracle.rs - Akash x/oracle module integration

use cosmwasm_std::Binary;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// DataID uniquely identifies a price pair by asset and base denomination
/// Matches proto: akash.oracle.v1.DataID
#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
pub struct DataID {
    /// Asset denomination (e.g., "akt")
    /// Note: Oracle module expects "akt" (not "uakt")
    pub denom: String,
    /// Base denomination for the price pair (e.g., "usd")
    pub base_denom: String,
}

impl DataID {
    pub fn new(denom: String, base_denom: String) -> Self {
        Self { denom, base_denom }
    }

    /// Default for AKT/USD pair
    /// Note: Oracle module expects "akt" (not "uakt") and "usd" as denom/base_denom
    pub fn akt_usd() -> Self {
        Self {
            denom: "akt".to_string(),
            base_denom: "usd".to_string(),
        }
    }
}

/// PriceDataState represents the price value and timestamp
/// Matches proto: akash.oracle.v1.PriceDataState
#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
pub struct PriceDataState {
    /// Decimal price value (cosmos.Dec format string)
    pub price: String,
    /// Timestamp seconds (for google.protobuf.Timestamp)
    pub timestamp_seconds: i64,
    /// Timestamp nanoseconds (for google.protobuf.Timestamp)
    pub timestamp_nanos: i32,
}

impl PriceDataState {
    pub fn new(price: String, timestamp_seconds: i64, timestamp_nanos: i32) -> Self {
        Self {
            price,
            timestamp_seconds,
            timestamp_nanos,
        }
    }
}

/// MsgAddPriceEntry defines an SDK message to add oracle price entry
/// Matches proto: akash.oracle.v1.MsgAddPriceEntry
#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename = "akash/oracle/v1/MsgAddPriceEntry")]
pub struct MsgAddPriceEntry {
    /// Signer is the bech32 address of the account
    pub signer: String,
    /// ID uniquely identifies the price data
    pub id: DataID,
    /// Price contains the price value and timestamp
    pub price: PriceDataState,
}

impl MsgAddPriceEntry {
    /// Create a new MsgAddPriceEntry with the new proto format
    pub fn new(
        signer: String,
        denom: String,
        base_denom: String,
        price: String,
        timestamp_seconds: i64,
        timestamp_nanos: i32,
    ) -> Self {
        Self {
            signer,
            id: DataID::new(denom, base_denom),
            price: PriceDataState::new(price, timestamp_seconds, timestamp_nanos),
        }
    }

    /// Create for AKT/USD price submission
    /// Note: Oracle module expects "akt" (not "uakt") and "usd" as denom/base_denom
    pub fn akt_usd(signer: String, price: String, timestamp_seconds: i64) -> Self {
        Self::new(
            signer,
            "akt".to_string(),
            "usd".to_string(),
            price,
            timestamp_seconds,
            0, // nanos default to 0
        )
    }

    /// Encode to protobuf binary for the oracle module
    pub fn encode_to_protobuf(&self) -> Binary {
        self.encode_to_binary()
    }

    /// Encode the message to protobuf binary
    /// Matches proto field numbers:
    /// - Field 1: signer (string)
    /// - Field 2: id (DataID message)
    /// - Field 3: price (PriceDataState message)
    fn encode_to_binary(&self) -> Binary {
        let mut buf = Vec::new();

        // Field 1: signer (tag = 0x0a = (1 << 3) | 2)
        buf.push(0x0a);
        encode_varint(&mut buf, self.signer.len() as u64);
        buf.extend_from_slice(self.signer.as_bytes());

        // Field 2: id (tag = 0x12 = (2 << 3) | 2)
        let id_bytes = self.encode_data_id();
        buf.push(0x12);
        encode_varint(&mut buf, id_bytes.len() as u64);
        buf.extend(id_bytes);

        // Field 3: price (tag = 0x1a = (3 << 3) | 2)
        let price_bytes = self.encode_price_data_state();
        buf.push(0x1a);
        encode_varint(&mut buf, price_bytes.len() as u64);
        buf.extend(price_bytes);

        Binary::from(buf)
    }

    /// Encode DataID submessage
    /// Fields: 1=denom, 2=base_denom
    fn encode_data_id(&self) -> Vec<u8> {
        let mut buf = Vec::new();

        // Field 1: denom
        buf.push(0x0a);
        encode_varint(&mut buf, self.id.denom.len() as u64);
        buf.extend_from_slice(self.id.denom.as_bytes());

        // Field 2: base_denom
        buf.push(0x12);
        encode_varint(&mut buf, self.id.base_denom.len() as u64);
        buf.extend_from_slice(self.id.base_denom.as_bytes());

        buf
    }

    /// Encode PriceDataState submessage
    /// Fields: 1=price (string), 2=timestamp (google.protobuf.Timestamp)
    fn encode_price_data_state(&self) -> Vec<u8> {
        let mut buf = Vec::new();

        // Field 1: price (string, cosmos.Dec format)
        buf.push(0x0a);
        encode_varint(&mut buf, self.price.price.len() as u64);
        buf.extend_from_slice(self.price.price.as_bytes());

        // Field 2: timestamp (google.protobuf.Timestamp message)
        let timestamp_bytes = self.encode_timestamp();
        if !timestamp_bytes.is_empty() {
            buf.push(0x12);
            encode_varint(&mut buf, timestamp_bytes.len() as u64);
            buf.extend(timestamp_bytes);
        }

        buf
    }

    /// Encode google.protobuf.Timestamp
    /// Fields: 1=seconds (int64), 2=nanos (int32)
    fn encode_timestamp(&self) -> Vec<u8> {
        let mut buf = Vec::new();

        // Field 1: seconds (tag = 0x08 = (1 << 3) | 0 for varint)
        if self.price.timestamp_seconds != 0 {
            buf.push(0x08);
            encode_varint(&mut buf, self.price.timestamp_seconds as u64);
        }

        // Field 2: nanos (tag = 0x10 = (2 << 3) | 0 for varint)
        if self.price.timestamp_nanos != 0 {
            buf.push(0x10);
            encode_varint(&mut buf, self.price.timestamp_nanos as u64);
        }

        buf
    }
}

/// Helper to encode unsigned varint
fn encode_varint(buf: &mut Vec<u8>, mut value: u64) {
    loop {
        let mut byte = (value & 0x7F) as u8;
        value >>= 7;
        if value != 0 {
            byte |= 0x80;
        }
        buf.push(byte);
        if value == 0 {
            break;
        }
    }
}

/// Convert Pyth price data to Cosmos SDK LegacyDec string format.
///
/// Cosmos SDK LegacyDec uses 18 decimal precision represented as an integer string.
/// For example:
/// - 0.5 becomes "500000000000000000"
/// - 1.0 becomes "1000000000000000000"
/// - 0.524683 becomes "524683000000000000"
///
/// Pyth provides price as an integer with a negative exponent:
/// - price=52468300, expo=-8 means 0.52468300
///
/// To convert: multiply by 10^(18 + expo) to get the 18-decimal representation
pub fn pyth_price_to_decimal(price: i64, expo: i32) -> String {
    const COSMOS_DECIMALS: i32 = 18;

    let abs_price = price.unsigned_abs() as u128;
    let is_negative = price < 0;

    // Calculate the power adjustment needed
    // For expo=-8, we need to multiply by 10^(18-8) = 10^10
    let power_adjustment = COSMOS_DECIMALS + expo;

    let result = if power_adjustment >= 0 {
        // Multiply by 10^power_adjustment
        let multiplier = 10_u128.pow(power_adjustment as u32);
        abs_price * multiplier
    } else {
        // Divide by 10^|power_adjustment| (should be rare for Pyth data)
        let divisor = 10_u128.pow((-power_adjustment) as u32);
        abs_price / divisor
    };

    if is_negative {
        format!("-{}", result)
    } else {
        result.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_pyth_price_to_decimal() {
        // Test positive price with negative exponent
        // price=52468300, expo=-8 means 0.52468300
        // In Cosmos LegacyDec (18 decimals): 0.52468300 * 10^18 = 524683000000000000
        assert_eq!(pyth_price_to_decimal(52468300, -8), "524683000000000000");

        // Test price with more decimals
        // price=123456789, expo=-8 means 1.23456789
        // In Cosmos LegacyDec: 1.23456789 * 10^18 = 1234567890000000000
        assert_eq!(pyth_price_to_decimal(123456789, -8), "1234567890000000000");

        // Test price with fewer decimals
        // price=100000000, expo=-8 means 1.00000000
        // In Cosmos LegacyDec: 1.0 * 10^18 = 1000000000000000000
        assert_eq!(pyth_price_to_decimal(100000000, -8), "1000000000000000000");

        // Test negative price
        // In Cosmos LegacyDec: -0.52468300 * 10^18 = -524683000000000000
        assert_eq!(pyth_price_to_decimal(-52468300, -8), "-524683000000000000");

        // Test zero
        assert_eq!(pyth_price_to_decimal(0, -8), "0");
    }

    #[test]
    fn test_data_id_creation() {
        let data_id = DataID::new("akt".to_string(), "usd".to_string());
        assert_eq!(data_id.denom, "akt");
        assert_eq!(data_id.base_denom, "usd");

        let akt_usd = DataID::akt_usd();
        assert_eq!(akt_usd.denom, "akt");
        assert_eq!(akt_usd.base_denom, "usd");
    }

    #[test]
    fn test_price_data_state_creation() {
        let state = PriceDataState::new("0.52468300".to_string(), 1234567890, 0);
        assert_eq!(state.price, "0.52468300");
        assert_eq!(state.timestamp_seconds, 1234567890);
        assert_eq!(state.timestamp_nanos, 0);
    }

    #[test]
    fn test_msg_add_price_entry_creation() {
        let msg = MsgAddPriceEntry::new(
            "akash1abc123".to_string(),
            "akt".to_string(),
            "usd".to_string(),
            "524683000000000000".to_string(), // LegacyDec format
            1234567890,
            0,
        );

        assert_eq!(msg.signer, "akash1abc123");
        assert_eq!(msg.id.denom, "akt");
        assert_eq!(msg.id.base_denom, "usd");
        assert_eq!(msg.price.price, "524683000000000000");
        assert_eq!(msg.price.timestamp_seconds, 1234567890);

        // Test protobuf encoding
        let binary = msg.encode_to_protobuf();
        assert!(!binary.is_empty());
    }

    #[test]
    fn test_msg_add_price_entry_akt_usd() {
        let msg = MsgAddPriceEntry::akt_usd(
            "akash1test".to_string(),
            "1234567890000000000".to_string(), // 1.23456789 in LegacyDec format
            1234567890,
        );

        assert_eq!(msg.signer, "akash1test");
        assert_eq!(msg.id.denom, "akt");
        assert_eq!(msg.id.base_denom, "usd");
        assert_eq!(msg.price.price, "1234567890000000000");
    }

    #[test]
    fn test_encode_to_binary() {
        let msg = MsgAddPriceEntry::new(
            "akash1test".to_string(),
            "akt".to_string(),
            "usd".to_string(),
            "1230000000000000000".to_string(), // 1.23 in LegacyDec format
            1234567890,
            0,
        );

        let binary = msg.encode_to_binary();

        // Verify it's not empty
        assert!(!binary.is_empty());

        // Verify it starts with correct field tag for signer (0x0a)
        assert_eq!(binary[0], 0x0a);
    }

    #[test]
    fn test_varint_encoding() {
        let mut buf = Vec::new();

        // Test small value (< 128)
        encode_varint(&mut buf, 10);
        assert_eq!(buf, vec![10]);

        // Test larger value (requires multiple bytes)
        buf.clear();
        encode_varint(&mut buf, 300);
        assert_eq!(buf, vec![0xac, 0x02]); // 300 = 0x12c = 0b100101100

        // Test zero
        buf.clear();
        encode_varint(&mut buf, 0);
        assert_eq!(buf, vec![0]);
    }
}
