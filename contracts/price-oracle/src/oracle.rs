// oracle.rs - Akash x/oracle module integration

use cosmwasm_std::Binary;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// PriceEntry represents a price entry for a denomination in the oracle module
#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
pub struct PriceEntry {
    /// Denom is the denomination of the asset (e.g., "akt")
    pub denom: String,

    /// Price is the price of the asset in USD
    /// Represented as a decimal string (e.g., "0.52468300")
    pub price: String,
}

/// MsgAddDenomPriceEntry defines an SDK message to add oracle price entry
/// This will be passed to the chain via a custom handler
#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, JsonSchema)]
#[serde(rename = "akash/oracle/MsgAddDenomPriceEntry")]
pub struct MsgAddDenomPriceEntry {
    /// Signer is the bech32 address of the provider
    pub signer: String,

    /// Price entry containing denom and price
    pub price: PriceEntry,
}

impl MsgAddDenomPriceEntry {
    /// Create a new MsgAddDenomPriceEntry
    pub fn new(signer: String, denom: String, price: String) -> Self {
        Self {
            signer,
            price: PriceEntry { denom, price },
        }
    }

    /// Encode to protobuf binary for the oracle module
    pub fn encode_to_protobuf(self) -> Binary {
        self.encode_to_binary()
    }

    /// Encode the message to protobuf binary
    fn encode_to_binary(&self) -> cosmwasm_std::Binary {
        // Manually encode the protobuf message
        // Field 1: signer (string)
        // Field 2: price (PriceEntry)

        let mut buf = Vec::new();

        // Field 1: signer (tag = 1, wire_type = 2 for length-delimited)
        buf.push(0x0a); // (1 << 3) | 2
        buf.push(self.signer.len() as u8);
        buf.extend_from_slice(self.signer.as_bytes());

        // Field 2: price (tag = 2, wire_type = 2 for length-delimited)
        let price_bytes = self.encode_price_entry();
        buf.push(0x12); // (2 << 3) | 2
        buf.push(price_bytes.len() as u8);
        buf.extend(price_bytes);

        cosmwasm_std::Binary::from(buf)
    }

    /// Encode PriceEntry submessage
    fn encode_price_entry(&self) -> Vec<u8> {
        let mut buf = Vec::new();

        // Field 1: denom (string)
        buf.push(0x0a); // (1 << 3) | 2
        buf.push(self.price.denom.len() as u8);
        buf.extend_from_slice(self.price.denom.as_bytes());

        // Field 2: price (string)
        buf.push(0x12); // (2 << 3) | 2
        buf.push(self.price.price.len() as u8);
        buf.extend_from_slice(self.price.price.as_bytes());

        buf
    }
}

/// Helper function to convert Pyth price data to decimal string
pub fn pyth_price_to_decimal(price: i64, expo: i32) -> String {
    // Convert price with exponent to decimal string
    // Example: price=52468300, expo=-8 -> "0.52468300"

    let abs_price = price.abs();
    let is_negative = price < 0;

    if expo >= 0 {
        // Positive exponent: multiply by 10^expo
        let multiplier = 10_i64.pow(expo as u32);
        let result = abs_price * multiplier;
        if is_negative {
            format!("-{}", result)
        } else {
            result.to_string()
        }
    } else {
        // Negative exponent: divide by 10^|expo|
        let abs_expo = expo.abs() as u32;
        let divisor = 10_i64.pow(abs_expo);

        let integer_part = abs_price / divisor;
        let fractional_part = abs_price % divisor;

        // Format with proper decimal places
        let fractional_str = format!("{:0width$}", fractional_part, width = abs_expo as usize);

        if is_negative {
            format!("-{}.{}", integer_part, fractional_str)
        } else {
            format!("{}.{}", integer_part, fractional_str)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_pyth_price_to_decimal() {
        // Test positive price with negative exponent
        assert_eq!(pyth_price_to_decimal(52468300, -8), "0.52468300");

        // Test price with more decimals
        assert_eq!(pyth_price_to_decimal(123456789, -8), "1.23456789");

        // Test price with fewer decimals
        assert_eq!(pyth_price_to_decimal(100000000, -8), "1.00000000");

        // Test negative price
        assert_eq!(pyth_price_to_decimal(-52468300, -8), "-0.52468300");

        // Test zero
        assert_eq!(pyth_price_to_decimal(0, -8), "0.00000000");
    }

    #[test]
    fn test_msg_add_denom_price_entry_creation() {
        let msg = MsgAddDenomPriceEntry::new(
            "akash1abc123".to_string(),
            "akt".to_string(),
            "0.52468300".to_string(),
        );

        assert_eq!(msg.signer, "akash1abc123");
        assert_eq!(msg.price.denom, "akt");
        assert_eq!(msg.price.price, "0.52468300");

        // Test protobuf encoding
        let binary = msg.encode_to_protobuf();
        assert!(!binary.is_empty());
    }

    #[test]
    fn test_encode_to_binary() {
        let msg = MsgAddDenomPriceEntry::new(
            "akash1test".to_string(),
            "akt".to_string(),
            "1.23".to_string(),
        );

        let binary = msg.encode_to_binary();

        // Verify it's not empty
        assert!(!binary.is_empty());

        // Verify it starts with correct field tag for signer (0x0a)
        assert_eq!(binary[0], 0x0a);
    }
}
