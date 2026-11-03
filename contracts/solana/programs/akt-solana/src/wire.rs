//! Native x/sink collection keys and protobuf RecordData values.

use crate::errors::AktError;
use anchor_lang::prelude::*;
use solana_sha256_hasher::hash;

pub const SINK_STORE: &[u8] = b"sink";
pub const TRANSFER_KEY_PREFIX: &[u8] = &[0x02, 0x00];
pub const SETTLEMENT_KEY_PREFIX: &[u8] = &[0x03, 0x01];
pub const TRANSFER_KEY_SIZE: usize = 39;
pub const SETTLEMENT_KEY_SIZE: usize = 40;

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Debug, PartialEq, Eq, InitSpace)]
pub struct TransferId {
    pub owner: [u8; 20],
    pub height: u64,
    pub sequence: u64,
}

impl TransferId {
    pub fn validate(&self) -> Result<()> {
        require!(
            self.owner != [0; 20]
                && self.height > 0
                && self.height <= i64::MAX as u64
                && self.sequence > 0,
            AktError::InvalidRecord
        );
        Ok(())
    }

    // Header H commits state H-1, so a deposit at H needs a later header.
    pub fn validate_proof_height(&self, proof_height: u64) -> Result<()> {
        self.validate()?;
        require!(self.height < proof_height, AktError::InvalidRecord);
        Ok(())
    }

    fn key(&self, prefix: &[u8]) -> Result<Vec<u8>> {
        self.validate()?;
        let mut key = Vec::with_capacity(prefix.len() + 37);
        key.extend_from_slice(prefix);
        key.push(20);
        key.extend_from_slice(&self.owner);
        key.extend_from_slice(&self.height.to_be_bytes());
        key.extend_from_slice(&self.sequence.to_be_bytes());
        Ok(key)
    }
}

pub fn transfer_key(id: &TransferId) -> Result<Vec<u8>> {
    id.key(TRANSFER_KEY_PREFIX)
}

pub fn settlement_key(id: &TransferId, status: u8) -> Result<Vec<u8>> {
    require!((1..=5).contains(&status), AktError::InvalidSettlementStatus);
    id.key(&[SETTLEMENT_KEY_PREFIX[0], SETTLEMENT_KEY_PREFIX[1], status])
}

pub fn record_value(recipient: &Pubkey, amount: u64) -> Result<Vec<u8>> {
    require!(amount > 0, AktError::InvalidRecord);
    let recipient = recipient.to_string();
    let amount = amount.to_string();
    // Canonical base58 and decimal u64 keep all native protobuf lengths below 128.
    let mut value = Vec::with_capacity(recipient.len() + amount.len() + 12);
    value.extend_from_slice(&[0x0a, recipient.len() as u8]);
    value.extend_from_slice(recipient.as_bytes());
    value.extend_from_slice(&[0x12, (amount.len() + 8) as u8, 0x0a, 4]);
    value.extend_from_slice(b"uakt");
    value.extend_from_slice(&[0x12, amount.len() as u8]);
    value.extend_from_slice(amount.as_bytes());
    Ok(value)
}

pub fn proof_nonce(key: &[u8]) -> [u8; 32] {
    hash(key).to_bytes()
}

#[cfg(test)]
mod tests {
    use super::*;
    use anchor_lang::InstructionData;

    fn id() -> TransferId {
        TransferId {
            owner: std::array::from_fn(|i| 0xa0 + i as u8),
            height: 0x0102_0304_0506_0708,
            sequence: 0x1112_1314_1516_1718,
        }
    }

    #[test]
    fn native_record_uses_canonical_strings_and_positive_u64() {
        for amount in [1, 127, 128, u64::MAX] {
            let decimal = amount.to_string();
            let mut expected = vec![10, 32];
            expected.extend_from_slice(b"11111111111111111111111111111111");
            expected.extend_from_slice(&[18, (8 + decimal.len()) as u8, 10, 4]);
            expected.extend_from_slice(b"uakt");
            expected.extend_from_slice(&[18, decimal.len() as u8]);
            expected.extend_from_slice(decimal.as_bytes());
            assert_eq!(record_value(&Pubkey::default(), amount).unwrap(), expected);
        }
        assert_eq!(
            record_value(&Pubkey::default(), 0).unwrap_err(),
            AktError::InvalidRecord.into()
        );
        assert_eq!(
            record_value(&Pubkey::new_from_array([255; 32]), u64::MAX)
                .unwrap()
                .len(),
            76
        );
    }

    #[test]
    fn proof_keys_bind_identity_status_and_full_nonce() {
        let key = [
            0x02, 0x00, 0x14, 0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xa9, 0xaa,
            0xab, 0xac, 0xad, 0xae, 0xaf, 0xb0, 0xb1, 0xb2, 0xb3, 0x01, 0x02, 0x03, 0x04, 0x05,
            0x06, 0x07, 0x08, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
        ];
        assert_eq!(transfer_key(&id()).unwrap(), key);
        assert_eq!(key.len(), TRANSFER_KEY_SIZE);
        let mut nonces = std::collections::BTreeSet::from([proof_nonce(&key)]);
        for status in 1..=5 {
            let expected = [&[3, 1, status][..], &key[2..]].concat();
            assert_eq!(settlement_key(&id(), status).unwrap(), expected);
            assert_eq!(expected.len(), SETTLEMENT_KEY_SIZE);
            assert!(nonces.insert(proof_nonce(&expected)));
        }
        for status in [0, 6, 255] {
            assert!(settlement_key(&id(), status).is_err());
        }
        for other in [
            TransferId {
                owner: [1; 20],
                ..id()
            },
            TransferId {
                height: id().height + 1,
                ..id()
            },
            TransferId {
                sequence: id().sequence + 1,
                ..id()
            },
        ] {
            assert_ne!(
                proof_nonce(&transfer_key(&other).unwrap()),
                proof_nonce(&key)
            );
        }
    }

    #[test]
    fn identity_bounds_and_strict_header_height() {
        let valid = id();
        assert!(valid.validate_proof_height(valid.height + 1).is_ok());
        for proof_height in [0, valid.height - 1, valid.height] {
            assert_eq!(
                valid.validate_proof_height(proof_height).unwrap_err(),
                AktError::InvalidRecord.into()
            );
        }
        let max = TransferId {
            height: i64::MAX as u64,
            sequence: u64::MAX,
            ..valid
        };
        assert!(max.validate_proof_height(i64::MAX as u64 + 1).is_ok());
        assert_eq!(
            &transfer_key(&max).unwrap()[23..],
            &[&i64::MAX.to_be_bytes()[..], &u64::MAX.to_be_bytes()[..]].concat()
        );
        for invalid in [
            TransferId {
                owner: [0; 20],
                ..valid
            },
            TransferId { height: 0, ..valid },
            TransferId {
                height: i64::MAX as u64 + 1,
                ..valid
            },
            TransferId {
                height: u64::MAX,
                ..valid
            },
            TransferId {
                sequence: 0,
                ..valid
            },
        ] {
            assert_eq!(
                invalid.validate_proof_height(u64::MAX).unwrap_err(),
                AktError::InvalidRecord.into()
            );
            assert!(transfer_key(&invalid).is_err());
            assert!(settlement_key(&invalid, 3).is_err());
        }
    }

    #[test]
    fn instruction_arguments_have_no_record_version() {
        let id = id();
        let recipient = Pubkey::new_unique();
        let amount = u64::MAX;
        let proof_height = id.height + 1;
        let mint = crate::instruction::MintTransfer {
            id,
            recipient_key: recipient,
            amount,
            proof_height,
        }
        .data();
        assert_eq!(mint.len(), 92);
        assert_eq!(
            &mint[8..],
            [
                &id.try_to_vec().unwrap()[..],
                recipient.as_ref(),
                &amount.to_le_bytes(),
                &proof_height.to_le_bytes()
            ]
            .concat()
        );
        let void = crate::instruction::VoidTransfer {
            id,
            settlement_status: 5,
            proof_height,
        }
        .data();
        assert_eq!(void.len(), 53);
        assert_eq!(
            &void[8..],
            [
                &id.try_to_vec().unwrap()[..],
                &[5],
                &proof_height.to_le_bytes()
            ]
            .concat()
        );
        assert_eq!(crate::instruction::GrowProofBuffer {}.data().len(), 8);
    }
}
