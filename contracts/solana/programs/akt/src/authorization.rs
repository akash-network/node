use crate::{constants::MAX_PROOF_BYTES, errors::AktError};
use anchor_lang::prelude::*;

// Policy and vote sizes come from the proven native state. Bound parser work by
// the proof upload limit instead of imposing a separate reporter-count limit.
const MAX_OBSERVATION_BYTES: usize = 79;
const MAX_VETO_REASON_BYTES: usize = 256;
const MAX_AUTHORIZATION_BYTES: usize = 21;

#[derive(Debug, PartialEq, Eq)]
pub(crate) struct VoidAuthorization {
    pub requested_height: u64,
    pub requested_round: u64,
}

struct Fields<'a>(&'a [u8]);

impl<'a> Fields<'a> {
    fn varint(&mut self) -> Result<u64> {
        let mut value = 0;
        for shift in (0..70).step_by(7) {
            let (&byte, rest) = self
                .0
                .split_first()
                .ok_or(AktError::InvalidVoidAuthorization)?;
            self.0 = rest;
            require!(shift != 63 || byte <= 1, AktError::InvalidVoidAuthorization);
            value |= u64::from(byte & 0x7f) << shift;
            if byte & 0x80 == 0 {
                require!(shift == 0 || byte != 0, AktError::InvalidVoidAuthorization);
                return Ok(value);
            }
        }
        err!(AktError::InvalidVoidAuthorization)
    }

    fn message(&mut self, maximum: usize) -> Result<&'a [u8]> {
        let length =
            usize::try_from(self.varint()?).map_err(|_| AktError::InvalidVoidAuthorization)?;
        require!(
            length <= maximum && length <= self.0.len(),
            AktError::InvalidVoidAuthorization
        );
        let (value, rest) = self.0.split_at(length);
        self.0 = rest;
        Ok(value)
    }
}

impl VoidAuthorization {
    fn decode(value: &[u8]) -> Result<Self> {
        let mut fields = Fields(value);
        require!(fields.varint()? == 8, AktError::InvalidVoidAuthorization);
        let requested_height = fields.varint()?;
        require!(fields.varint()? == 16, AktError::InvalidVoidAuthorization);
        let requested_round = fields.varint()?;
        require!(
            fields.0.is_empty()
                && requested_height > 0
                && requested_height <= i64::MAX as u64
                && requested_round > 0,
            AktError::InvalidVoidAuthorization
        );
        Ok(Self {
            requested_height,
            requested_round,
        })
    }
}

pub(crate) fn void_authorization(
    value: &[u8],
    deposit_height: u64,
    proof_height: u64,
) -> Result<VoidAuthorization> {
    require!(
        value.len() <= MAX_PROOF_BYTES,
        AktError::InvalidVoidAuthorization
    );
    let mut fields = Fields(value);
    let mut previous = 0;
    let mut round = 0;
    let mut authorization = None;
    while !fields.0.is_empty() {
        let tag = fields.varint()?;
        let field = tag >> 3;
        require!(
            (1..=10).contains(&field) && field >= previous && (field != previous || field == 4),
            AktError::InvalidVoidAuthorization
        );
        previous = field;
        match field {
            1 | 2 | 5 | 10 => {
                require!(tag & 7 == 0, AktError::InvalidVoidAuthorization);
                let scalar = fields.varint()?;
                require!(scalar > 0, AktError::InvalidVoidAuthorization);
                if field == 2 {
                    round = scalar;
                }
            }
            3 | 4 | 6 | 7 | 8 | 9 => {
                require!(tag & 7 == 2, AktError::InvalidVoidAuthorization);
                let maximum = match field {
                    3 | 4 | 7 => MAX_PROOF_BYTES,
                    6 => MAX_OBSERVATION_BYTES,
                    8 => MAX_VETO_REASON_BYTES,
                    _ => MAX_AUTHORIZATION_BYTES,
                };
                let value = fields.message(maximum)?;
                if field == 9 {
                    authorization = Some(VoidAuthorization::decode(value)?);
                }
            }
            _ => return err!(AktError::InvalidVoidAuthorization),
        }
    }
    let authorization = authorization.ok_or(AktError::InvalidVoidAuthorization)?;
    require!(
        authorization.requested_round <= round
            && authorization.requested_height >= deposit_height
            && authorization.requested_height < proof_height,
        AktError::InvalidVoidAuthorization
    );
    Ok(authorization)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn permanent_order_survives_later_rounds_but_requires_a_later_header() {
        let value = [16, 4, 74, 4, 8, 2, 16, 3];
        assert_eq!(
            void_authorization(&value, 2, 3).unwrap(),
            VoidAuthorization {
                requested_height: 2,
                requested_round: 3
            }
        );
        assert!(void_authorization(&value, 3, 4).is_err());
        assert!(void_authorization(&value, 1, 2).is_err());
    }

    #[test]
    fn rejects_ambiguous_malformed_and_overflowing_receipts() {
        for value in [
            vec![],
            vec![16, 4],
            vec![74, 4, 8, 2, 16, 3],
            vec![16, 4, 74, 4, 8, 0, 16, 3],
            vec![16, 4, 74, 4, 8, 2, 16, 0],
            vec![16, 4, 74, 4, 8, 3, 16, 3],
            vec![16, 4, 74, 4, 8, 2, 16, 5],
            vec![16, 4, 74, 5, 8, 2, 16, 3],
            vec![16, 4, 74, 5, 8, 0x82, 0, 16, 3],
            vec![16, 4, 74, 6, 8, 2, 16, 3, 16, 3],
            vec![16, 4, 74, 4, 8, 2, 16, 3, 74, 4, 8, 2, 16, 3],
            vec![16, 4, 74, 4, 8, 2, 16, 3, 0],
            vec![16, 4, 74, 4, 8, 2, 16, 3, 90, 0],
            vec![18, 1, 4, 74, 4, 8, 2, 16, 3],
            vec![
                16, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 2, 74, 4, 8, 2, 16, 3,
            ],
            vec![
                16, 4, 74, 13, 8, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 1, 16, 3,
            ],
            vec![16, 4, 74, 4, 16, 3, 8, 2],
            vec![16, 4, 74, 4, 9, 2, 16, 3],
        ] {
            assert!(void_authorization(&value, 1, 3).is_err(), "{value:?}");
        }
    }
}
