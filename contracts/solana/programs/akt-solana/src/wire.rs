//! x/sink v1 record encoding and proof keys.
//!
//! The Akash x/sink module writes these bytes. Its store keys are fixed-width
//! binary: a two-byte prefix, then the record ID as a big-endian u64.
//! docs/bridge-protocol.md documents them and the tests below pin them. The
//! program builds every proven key and value itself from instruction
//! arguments.

use crate::errors::AktError;
use anchor_lang::prelude::*;

/// Name of the Akash store that holds sink records: the first proof path
/// segment.
pub const SINK_STORE: &[u8] = b"sink";

/// Sink store prefix of transfer record keys.
pub const TRANSFER_KEY_PREFIX: &[u8] = &[0x02, 0x00];

/// Sink store prefix of void order keys.
pub const VOID_KEY_PREFIX: &[u8] = &[0x02, 0x01];

/// The value x/sink stores at the record's transfer key. Its borsh encoding
/// is the 77-byte value proven against Akash state, so field order and types
/// are fixed.
#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct TransferRecord {
    /// Encoding version; must be 1.
    pub version: u8,
    /// x/sink record ID, unique on the chain and never reused.
    pub id: u64,
    /// Akash sender's 20 address bytes.
    pub sender: [u8; 20],
    /// Owner of the associated token account that receives the tokens.
    pub recipient: Pubkey,
    /// Base units (uakt) to mint.
    pub amount: u64,
    /// Akash height at which the record was created.
    pub height: u64,
}

impl TransferRecord {
    /// Checks made before the proof: version 1, nonzero ID and amount, and a
    /// creation height between 1 and the proof height.
    pub fn validate(&self, proof_height: u64) -> Result<()> {
        require!(
            self.version == 1
                && self.id > 0
                && self.amount > 0
                && self.height > 0
                && self.height <= proof_height,
            AktError::InvalidRecord
        );
        Ok(())
    }

    /// This record's key in the sink store.
    pub fn key(&self) -> Vec<u8> {
        transfer_key(self.id)
    }
}

/// The 10-byte key of transfer record `id`: `TRANSFER_KEY_PREFIX`, then the
/// ID as a big-endian u64.
pub fn transfer_key(id: u64) -> Vec<u8> {
    [TRANSFER_KEY_PREFIX, &id.to_be_bytes()].concat()
}

/// The 10-byte key of the Akash governance void order for `id`:
/// `VOID_KEY_PREFIX`, then the ID as a big-endian u64.
pub fn void_key(id: u64) -> Vec<u8> {
    [VOID_KEY_PREFIX, &id.to_be_bytes()].concat()
}

/// A void order's value: version 1, then the ID as a little-endian u64.
pub fn void_value(id: u64) -> Vec<u8> {
    let mut value = vec![1];
    value.extend_from_slice(&id.to_le_bytes());
    value
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The 77-byte layout in docs/bridge-protocol.md, written out by hand.
    const ENCODED: [u8; 77] = [
        // 0, 1 byte: version.
        0x01, //
        // 1, 8 bytes: ID 0x0102030405060708, little-endian.
        0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01,
        // 9, 20 bytes: Akash sender address bytes, no length prefix.
        0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xa9, //
        0xaa, 0xab, 0xac, 0xad, 0xae, 0xaf, 0xb0, 0xb1, 0xb2, 0xb3, //
        // 29, 32 bytes: Solana recipient.
        0xc0, 0xc1, 0xc2, 0xc3, 0xc4, 0xc5, 0xc6, 0xc7, //
        0xc8, 0xc9, 0xca, 0xcb, 0xcc, 0xcd, 0xce, 0xcf, //
        0xd0, 0xd1, 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, //
        0xd8, 0xd9, 0xda, 0xdb, 0xdc, 0xdd, 0xde, 0xdf, //
        // 61, 8 bytes: amount 0x1112131415161718 uakt, little-endian.
        0x18, 0x17, 0x16, 0x15, 0x14, 0x13, 0x12, 0x11,
        // 69, 8 bytes: Akash height 0x2122232425262728, little-endian.
        0x28, 0x27, 0x26, 0x25, 0x24, 0x23, 0x22, 0x21,
    ];

    fn record() -> TransferRecord {
        TransferRecord {
            version: 1,
            id: 0x0102_0304_0506_0708,
            sender: std::array::from_fn(|i| 0xa0 + i as u8),
            recipient: Pubkey::new_from_array(std::array::from_fn(|i| 0xc0 + i as u8)),
            amount: 0x1112_1314_1516_1718,
            height: 0x2122_2324_2526_2728,
        }
    }

    #[test]
    fn transfer_record_encodes_to_the_documented_77_bytes() {
        assert_eq!(record().try_to_vec().unwrap(), ENCODED);
        let decoded = TransferRecord::try_from_slice(&ENCODED).unwrap();
        assert_eq!(decoded.version, 1);
        assert_eq!(decoded.id, 0x0102_0304_0506_0708);
        assert_eq!(decoded.sender, ENCODED[9..29]);
        assert_eq!(decoded.recipient.to_bytes(), ENCODED[29..61]);
        assert_eq!(decoded.amount, 0x1112_1314_1516_1718);
        assert_eq!(decoded.height, 0x2122_2324_2526_2728);
        // Trailing or missing bytes are not a record.
        assert!(TransferRecord::try_from_slice(&[&ENCODED[..], &[0]].concat()).is_err());
        assert!(TransferRecord::try_from_slice(&ENCODED[..76]).is_err());
    }

    #[test]
    fn proof_keys_and_void_value_use_the_documented_format() {
        assert_eq!(SINK_STORE, b"sink");
        // Prefix, then the ID as a big-endian u64.
        assert_eq!(
            record().key(),
            [0x02, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08]
        );
        assert_eq!(
            transfer_key(1),
            [0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01]
        );
        assert_eq!(
            transfer_key(u64::MAX),
            [0x02, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff]
        );
        assert_eq!(
            void_key(1),
            [0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01]
        );
        assert_eq!(
            void_key(u64::MAX),
            [0x02, 0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff]
        );
        assert_eq!(
            void_value(0x0102_0304_0506_0708),
            [0x01, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01]
        );
    }

    #[test]
    fn records_must_be_version_1_with_nonzero_fields_at_or_below_the_proof_height() {
        let valid = record();
        assert!(valid.validate(valid.height).is_ok());
        let invalid = [
            TransferRecord {
                version: 2,
                ..valid.clone()
            },
            TransferRecord {
                id: 0,
                ..valid.clone()
            },
            TransferRecord {
                amount: 0,
                ..valid.clone()
            },
            TransferRecord {
                height: 0,
                ..valid.clone()
            },
        ];
        for record in invalid {
            assert_eq!(
                record.validate(valid.height).unwrap_err(),
                AktError::InvalidRecord.into()
            );
        }
        assert_eq!(
            valid.validate(valid.height - 1).unwrap_err(),
            AktError::InvalidRecord.into()
        );
    }
}
