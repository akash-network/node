use crate::wire::{proof_nonce, record_value, settlement_key, transfer_key, TransferId};
use anchor_lang::prelude::*;
use std::collections::BTreeMap;

fn hex(value: &str) -> Vec<u8> {
    assert_eq!(value.len() % 2, 0);
    (0..value.len())
        .step_by(2)
        .map(|offset| u8::from_str_radix(&value[offset..offset + 2], 16).unwrap())
        .collect()
}

#[test]
fn sdk_vectors_match_rust_keys_values_nonces_and_seed_bytes() {
    let vectors: Vec<BTreeMap<String, String>> =
        serde_json::from_str(include_str!("../tests/fixtures/proof_vectors.json")).unwrap();
    assert_eq!(vectors.len(), 5);
    for vector in vectors {
        let id = TransferId {
            owner: hex(&vector["owner_hex"]).try_into().unwrap(),
            height: vector["height"].parse().unwrap(),
            sequence: vector["sequence"].parse().unwrap(),
        };
        let recipient = Pubkey::new_from_array(hex(&vector["recipient_hex"]).try_into().unwrap());
        let amount = vector["amount"].parse().unwrap();
        id.validate_proof_height(id.height + 1).unwrap();
        assert_eq!(recipient.to_string(), vector["recipient"]);
        assert_eq!(transfer_key(&id).unwrap(), hex(&vector["transfer_key_hex"]));
        assert_eq!(
            record_value(&recipient, amount).unwrap(),
            hex(&vector["record_value_hex"])
        );
        assert_eq!(
            proof_nonce(&transfer_key(&id).unwrap()).as_slice(),
            hex(&vector["mint_proof_nonce_hex"])
        );
        assert_eq!(
            id.height.to_le_bytes().as_slice(),
            hex(&vector["height_le_hex"])
        );
        assert_eq!(
            id.sequence.to_le_bytes().as_slice(),
            hex(&vector["sequence_le_hex"])
        );

        let status = Pubkey::find_program_address(
            &[
                crate::TRANSFER_SEED,
                &id.owner,
                &id.height.to_le_bytes(),
                &id.sequence.to_le_bytes(),
            ],
            &crate::ID,
        )
        .0;
        for other in [
            TransferId {
                owner: [0x55; 20],
                ..id
            },
            TransferId {
                height: id.height - 1,
                ..id
            },
            TransferId {
                sequence: id.sequence - 1,
                ..id
            },
        ] {
            let other_status = Pubkey::find_program_address(
                &[
                    crate::TRANSFER_SEED,
                    &other.owner,
                    &other.height.to_le_bytes(),
                    &other.sequence.to_le_bytes(),
                ],
                &crate::ID,
            )
            .0;
            assert_ne!(status, other_status);
        }
    }
}

#[test]
fn sdk_settlements_bind_every_status_and_preserve_the_authorization() {
    let vectors: Vec<serde_json::Value> =
        serde_json::from_str(include_str!("../tests/fixtures/settlement_vectors.json")).unwrap();
    assert_eq!(vectors.len(), 6);
    for vector in vectors {
        let text = |field| vector[field].as_str().unwrap();
        let id = TransferId {
            owner: hex(text("owner_hex")).try_into().unwrap(),
            height: text("height").parse().unwrap(),
            sequence: text("sequence").parse().unwrap(),
        };
        let status = u8::try_from(vector["status"].as_u64().unwrap()).unwrap();
        let key = settlement_key(&id, status).unwrap();
        assert_eq!(key, hex(text("settlement_key_hex")));
        assert_eq!(proof_nonce(&key).as_slice(), hex(text("proof_nonce_hex")));
        let value = hex(text("settlement_value_hex"));
        let authorization =
            crate::authorization::void_authorization(&value, id.height, id.height + 1);
        let receipt = &vector["settlement"]["void_authorization"];
        if receipt.is_null() {
            assert!(authorization.is_err());
        } else {
            let authorization = authorization.unwrap();
            assert_eq!(
                authorization.requested_height,
                receipt["requested_height"]
                    .as_str()
                    .unwrap()
                    .parse::<u64>()
                    .unwrap()
            );
            assert_eq!(
                authorization.requested_round,
                receipt["requested_round"]
                    .as_str()
                    .unwrap()
                    .parse::<u64>()
                    .unwrap()
            );
        }
        if text("name") == "maximum_evidence" {
            assert_eq!(value.len(), 6500);
        }
    }
}
