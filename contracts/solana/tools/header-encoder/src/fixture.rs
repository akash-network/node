//! Deterministic LOCAL TEST validators. Never use these keys as a trust anchor
//! outside the isolated benchmark. The resulting commits have real signatures.
use super::*;
use ed25519_dalek::{Signer, SigningKey};

pub(super) fn generate(
    mut input: Input,
    count: usize,
    signers: usize,
    weighted: bool,
) -> Result<Input> {
    // 256 is the patched verifier's two-batch signature limit.
    ensure!(
        (1..=256).contains(&count) && signers <= count,
        "invalid fixture size"
    );
    let mut keys = Vec::new();
    let mut validators = Vec::new();
    for i in 0..count {
        let seed: [u8; 32] = Sha256::digest(format!("AKT LOCAL BENCHMARK ONLY {i}")).into();
        let key = SigningKey::from_bytes(&seed);
        let pk = tendermint::PublicKey::from_raw_ed25519(&key.verifying_key().to_bytes())
            .context("invalid fixture key")?;
        let power = if weighted {
            if i < 30 {
                100_000u32
            } else {
                1_000u32
            }
        } else {
            10_000u32
        };
        let info = validator::Info::new(pk, power.into());
        keys.push((info.address, key));
        validators.push(info);
    }
    let set = validator::Set::without_proposer(validators);
    let block = &mut input.signed_header.header;
    block.chain_id = "akash-scale-1".parse()?;
    block.validators_hash = set.hash();
    block.next_validators_hash = set.hash();
    block.proposer_address = set.validators()[0].address;
    let commit = &mut input.signed_header.commit;
    commit.height = block.height;
    commit.block_id.hash = block.hash();
    commit.signatures.clear();
    for (i, validator) in set.validators().iter().enumerate() {
        // Preserve one slot per validator, including deliberately absent votes.
        if i >= signers {
            commit.signatures.push(CommitSig::BlockIdFlagAbsent);
            continue;
        }
        let key = &keys
            .iter()
            .find(|(address, _)| *address == validator.address)
            .unwrap()
            .1;
        let vote = CanonicalVote {
            vote_type: tendermint::vote::Type::Precommit,
            height: commit.height,
            round: commit.round,
            block_id: Some(commit.block_id),
            timestamp: Some(block.time),
            chain_id: block.chain_id.clone(),
        };
        let bytes = <CanonicalVote as Protobuf<tendermint_proto::v0_38::types::CanonicalVote>>::encode_length_delimited_vec(vote);
        commit.signatures.push(CommitSig::BlockIdFlagCommit {
            validator_address: validator.address,
            timestamp: block.time,
            signature: Some(key.sign(&bytes).to_bytes().as_slice().try_into()?),
        });
    }
    input.validators = set.validators().clone();
    input.trusted_validators = set.validators().clone();
    Ok(input)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;
    use tendermint_light_client_verifier::{
        options::Options,
        types::{TrustedBlockState, UntrustedBlockState},
        ProdVerifier, Verdict, Verifier,
    };

    fn fixture(signers: usize) -> Input {
        generate(
            serde_json::from_str(include_str!("../fixtures/template.json")).unwrap(),
            100,
            signers,
            false,
        )
        .unwrap()
    }

    fn verify(input: &Input) -> Verdict {
        let header = &input.signed_header.header;
        let validators = validator::Set::without_proposer(input.validators.clone());
        ProdVerifier::default().verify_update_header(
            UntrustedBlockState {
                signed_header: &input.signed_header,
                validators: &validators,
                next_validators: Some(&validators),
            },
            TrustedBlockState {
                chain_id: &header.chain_id,
                header_time: (header.time - Duration::from_secs(10)).unwrap(),
                height: 1u32.into(),
                next_validators: &validators,
                next_validators_hash: validators.hash(),
            },
            &Options {
                trust_threshold: tendermint::trust_threshold::TrustThresholdFraction::ONE_THIRD,
                trusting_period: Duration::from_secs(86400),
                clock_drift: Duration::from_secs(10),
            },
            header.time,
        )
    }

    #[test]
    fn absent_entries_preserve_a_valid_supermajority_commit() {
        for signers in [67, 100] {
            let input = fixture(signers);
            assert_eq!(input.signed_header.commit.signatures.len(), 100);
            assert_eq!(verify(&input), Verdict::Success);
        }
        assert_ne!(verify(&fixture(66)), Verdict::Success);
    }

    #[test]
    fn sets_above_one_batch_keep_a_supermajority_commit() {
        for (count, quorum) in [(129, 87), (256, 171)] {
            let generate = |signers| {
                generate(
                    serde_json::from_str(include_str!("../fixtures/template.json")).unwrap(),
                    count,
                    signers,
                    false,
                )
                .unwrap()
            };
            let input = generate(quorum);
            assert_eq!(input.signed_header.commit.signatures.len(), count);
            assert_eq!(verify(&input), Verdict::Success);
            assert_ne!(verify(&generate(quorum - 1)), Verdict::Success);
        }
        assert!(generate(
            serde_json::from_str(include_str!("../fixtures/template.json")).unwrap(),
            257,
            1,
            false
        )
        .is_err());
    }

    #[test]
    fn altering_the_attested_root_invalidates_the_commit() {
        let mut input = fixture(67);
        input.signed_header.header.app_hash = vec![42u8; 32].try_into().unwrap();
        assert_ne!(verify(&input), Verdict::Success);
    }

    #[test]
    fn weighted_quorum_is_checked_by_power_not_validator_count() {
        for signers in [20, 21, 100] {
            let input = generate(
                serde_json::from_str(include_str!("../fixtures/template.json")).unwrap(),
                100,
                signers,
                true,
            )
            .unwrap();
            assert_eq!(verify(&input) == Verdict::Success, signers >= 21);
            assert_eq!(input.signed_header.commit.signatures.len(), 100);
        }
    }
}
