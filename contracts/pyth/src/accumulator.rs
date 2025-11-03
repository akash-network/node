//! PNAU (Pyth Network Accumulator Update) parser
//!
//! Parses the accumulator format returned by Pyth Hermes v2 API.
//! This format wraps a Wormhole VAA containing a Merkle root,
//! along with price updates and their Merkle proofs.
//!
//! The MerklePriceUpdate section uses big-endian for length prefixes.
//!
//! Reference: https://docs.pyth.network/price-feeds/core/how-pyth-works/cross-chain

use cosmwasm_std::{Binary, StdError, StdResult};
use sha3::{Digest, Keccak256};

/// Magic bytes identifying PNAU format
pub const PNAU_MAGIC: &[u8] = b"PNAU";

/// Wormhole Merkle update type
pub const UPDATE_TYPE_WORMHOLE_MERKLE: u8 = 0;

/// Merkle tree constants (matching Pyth's implementation)
const MERKLE_LEAF_PREFIX: u8 = 0;
const MERKLE_NODE_PREFIX: u8 = 1;

/// Parsed PNAU accumulator update
#[derive(Debug)]
pub struct AccumulatorUpdate {
    /// The embedded Wormhole VAA (contains signed Merkle root)
    pub vaa: Binary,
    /// Merkle root from the VAA payload
    pub merkle_root: [u8; 20],
    /// Price updates with Merkle proofs
    pub price_updates: Vec<PriceUpdateWithProof>,
}

/// A price update with its Merkle proof (MerklePriceUpdate in pythnet-sdk)
#[derive(Debug)]
pub struct PriceUpdateWithProof {
    /// Raw message data (price update payload)
    pub message_data: Vec<u8>,
    /// Merkle proof nodes (20 bytes each)
    pub merkle_proof: Vec<[u8; 20]>,
}

/// Parse PNAU accumulator update format from Hermes v2 API
///
/// Format (based on pythnet-sdk wire::v1):
/// - Magic: "PNAU" (4 bytes) [offset 0-3]
/// - Major version (1 byte) [offset 4]
/// - Minor version (1 byte) [offset 5]
/// - Trailing length (1 byte) [offset 6]
/// - Trailing data (trailing_len bytes) [offset 7 to 7+trailing_len-1]
/// - Proof discriminant (1 byte): 0=WormholeMerkle [offset 7+trailing_len]
/// - VAA length (2 bytes, big-endian) [offset 8+trailing_len]
/// - VAA data (vaa_len bytes)
/// - Number of updates (1 byte)
/// - For each MerklePriceUpdate:
///   - Message size (2 bytes, big-endian)
///   - Message data
///   - Proof count (1 byte)
///   - Proof nodes (20 bytes each)
pub fn parse_accumulator_update(data: &[u8]) -> StdResult<AccumulatorUpdate> {
    // Check minimum length for header
    if data.len() < 8 {
        return Err(StdError::msg("PNAU data too short"));
    }

    // Verify magic bytes
    if &data[0..4] != PNAU_MAGIC {
        return Err(StdError::msg(format!(
            "Invalid PNAU magic: expected {:?}, got {:?}",
            PNAU_MAGIC,
            &data[0..4]
        )));
    }

    let major_version = data[4];
    let _minor_version = data[5];
    let trailing_len = data[6] as usize;

    // Validate version
    if major_version != 1 {
        return Err(StdError::msg(format!(
            "Unsupported PNAU major version: {}",
            major_version
        )));
    }

    // Position after trailing data is where proof discriminant lives
    // Format: magic(4) + major(1) + minor(1) + trailing_len(1) + trailing_data(trailing_len) + proof_discriminant(1) + ...
    let mut offset = 7 + trailing_len;

    // Read proof discriminant (update type)
    if offset >= data.len() {
        return Err(StdError::msg("Missing proof discriminant"));
    }
    let update_type = data[offset];
    offset += 1;

    // Only support WormholeMerkle updates
    if update_type != UPDATE_TYPE_WORMHOLE_MERKLE {
        return Err(StdError::msg(format!(
            "Unsupported update type: {}, expected WormholeMerkle (0)",
            update_type
        )));
    }

    // Parse VAA length (u16 big-endian, as PrefixedVec<u16, u8>)
    if offset + 2 > data.len() {
        return Err(StdError::msg("Missing VAA length"));
    }
    let vaa_len = u16::from_be_bytes([data[offset], data[offset + 1]]) as usize;
    offset += 2;

    // Parse VAA data
    if offset + vaa_len > data.len() {
        return Err(StdError::msg(format!(
            "VAA data truncated: need {} bytes, have {}",
            vaa_len,
            data.len() - offset
        )));
    }
    let vaa = Binary::from(&data[offset..offset + vaa_len]);
    offset += vaa_len;

    // Extract Merkle root from VAA payload
    let merkle_root = extract_merkle_root_from_vaa(&vaa)?;

    // Parse number of updates
    if offset >= data.len() {
        return Err(StdError::msg("Missing update count"));
    }
    let num_updates = data[offset] as usize;
    offset += 1;

    // Parse each price update
    let mut price_updates = Vec::with_capacity(num_updates);
    for i in 0..num_updates {
        let (update, new_offset) = parse_price_update(data, offset)
            .map_err(|e| StdError::msg(format!("Failed to parse update {}: {}", i, e)))?;
        price_updates.push(update);
        offset = new_offset;
    }

    Ok(AccumulatorUpdate {
        vaa,
        merkle_root,
        price_updates,
    })
}

/// Extract the Merkle root from a Wormhole VAA payload
fn extract_merkle_root_from_vaa(vaa: &[u8]) -> StdResult<[u8; 20]> {
    // VAA structure:
    // - Version (1 byte)
    // - Guardian set index (4 bytes)
    // - Signature count (1 byte)
    // - Signatures (66 bytes each)
    // - Body starts after signatures

    if vaa.len() < 6 {
        return Err(StdError::msg("VAA too short"));
    }

    let sig_count = vaa[5] as usize;
    let body_offset = 6 + (sig_count * 66);

    if body_offset + 51 > vaa.len() {
        return Err(StdError::msg("VAA body too short"));
    }

    // Body structure:
    // - Timestamp (4 bytes)
    // - Nonce (4 bytes)
    // - Emitter chain (2 bytes)
    // - Emitter address (32 bytes)
    // - Sequence (8 bytes)
    // - Consistency level (1 byte)
    // - Payload starts at offset 51

    let payload_offset = body_offset + 51;
    let payload = &vaa[payload_offset..];

    // Payload for Merkle root:
    // - Magic "AUWV" (4 bytes) - Accumulator Update Wormhole Verification
    // - Update type (1 byte)
    // - Slot (8 bytes)
    // - Ring size (4 bytes)
    // - Root (20 bytes)

    if payload.len() < 37 {
        return Err(StdError::msg("Merkle payload too short"));
    }

    // Check magic "AUWV"
    if &payload[0..4] != b"AUWV" {
        return Err(StdError::msg(format!(
            "Invalid Merkle root magic: expected AUWV, got {:?}",
            String::from_utf8_lossy(&payload[0..4])
        )));
    }

    // Extract root (bytes 17-37)
    let mut root = [0u8; 20];
    root.copy_from_slice(&payload[17..37]);

    Ok(root)
}

/// Parse a single price update with its Merkle proof (MerklePriceUpdate)
///
/// MerklePriceUpdate format (Pyth wire format):
/// - message: 2-byte length prefix (big-endian) + data
/// - proof: 1-byte count + 20-byte nodes
fn parse_price_update(data: &[u8], mut offset: usize) -> StdResult<(PriceUpdateWithProof, usize)> {
    // Message size (2 bytes, big-endian - Pyth wire format)
    if offset + 2 > data.len() {
        return Err(StdError::msg("Missing message size"));
    }
    let message_size = u16::from_be_bytes([data[offset], data[offset + 1]]) as usize;
    offset += 2;

    // Message data
    if offset + message_size > data.len() {
        return Err(StdError::msg(format!(
            "Message data truncated: need {} bytes, have {}",
            message_size,
            data.len() - offset
        )));
    }
    let message_data = data[offset..offset + message_size].to_vec();
    offset += message_size;

    // Merkle proof size (1 byte = number of 20-byte nodes)
    if offset >= data.len() {
        return Err(StdError::msg("Missing proof size"));
    }
    let proof_size = data[offset] as usize;
    offset += 1;

    // Merkle proof nodes
    let mut merkle_proof = Vec::with_capacity(proof_size);
    for _ in 0..proof_size {
        if offset + 20 > data.len() {
            return Err(StdError::msg("Merkle proof truncated"));
        }
        let mut node = [0u8; 20];
        node.copy_from_slice(&data[offset..offset + 20]);
        merkle_proof.push(node);
        offset += 20;
    }

    Ok((
        PriceUpdateWithProof {
            message_data,
            merkle_proof,
        },
        offset,
    ))
}

/// Verify a Merkle proof for a price update
///
/// The proof demonstrates that the message is included in the tree
/// whose root was signed by Wormhole guardians.
pub fn verify_merkle_proof(
    message_data: &[u8],
    proof: &[[u8; 20]],
    expected_root: &[u8; 20],
) -> bool {
    // Compute leaf hash: keccak256(MERKLE_LEAF_PREFIX || message_data)[0..20]
    let mut hasher = Keccak256::new();
    hasher.update([MERKLE_LEAF_PREFIX]);
    hasher.update(message_data);
    let leaf_hash = hasher.finalize();
    let mut current: [u8; 20] = [0; 20];
    current.copy_from_slice(&leaf_hash[0..20]);

    // Walk up the tree
    for sibling in proof {
        let mut hasher = Keccak256::new();
        hasher.update([MERKLE_NODE_PREFIX]);

        // Sort children to ensure consistent ordering
        if current < *sibling {
            hasher.update(current);
            hasher.update(sibling);
        } else {
            hasher.update(sibling);
            hasher.update(current);
        }

        let node_hash = hasher.finalize();
        current.copy_from_slice(&node_hash[0..20]);
    }

    current == *expected_root
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_merkle_leaf_hash() {
        // Test that leaf hashing works correctly
        let message = b"test message";
        let mut hasher = Keccak256::new();
        hasher.update([MERKLE_LEAF_PREFIX]);
        hasher.update(message);
        let hash = hasher.finalize();

        // Should produce a valid hash
        assert_eq!(hash.len(), 32);
    }

    #[test]
    fn test_pnau_magic_detection() {
        let valid_magic = b"PNAU";
        let invalid_magic = b"TEST";

        assert_eq!(valid_magic, PNAU_MAGIC);
        assert_ne!(invalid_magic, PNAU_MAGIC);
    }

    #[test]
    fn test_parse_accumulator_too_short() {
        let data = b"PNAU";
        let result = parse_accumulator_update(data);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("too short"));
    }

    #[test]
    fn test_parse_accumulator_invalid_magic() {
        let data = b"TEST0100";
        let result = parse_accumulator_update(data);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("Invalid PNAU magic"));
    }
}
