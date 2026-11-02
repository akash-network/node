use anyhow::{bail, ensure, Context, Result};
use borsh::BorshSerialize;
use ibc_client_tendermint::types::Header;
use ibc_core_client_types::Height;
use serde::Deserialize;
use serde_json::json;
use sha2::{Digest, Sha256};
use solana_ibc_borsh_header::conversions::header_to_borsh;
use std::io::{self, Read};
use tendermint::{block::CommitSig, validator, vote::CanonicalVote};
use tendermint_proto::Protobuf;
mod fixture;

#[derive(Deserialize)]
struct Input {
    signed_header: tendermint::block::signed_header::SignedHeader,
    validators: Vec<validator::Info>,
    trusted_validators: Vec<validator::Info>,
    trusted_height: u64,
    revision: u64,
}

fn main() -> Result<()> {
    let mut raw = String::new();
    io::stdin().take(4 * 1024 * 1024).read_to_string(&mut raw)?;
    let mut input: Input = serde_json::from_str(&raw)?;
    let args: Vec<String> = std::env::args().skip(1).collect();
    if !args.is_empty() {
        ensure!(
            (args.len() == 3 || args.len() == 4) && args[0] == "--fixture",
            "usage: --fixture VALIDATORS SIGNERS [equal|weighted]"
        );
        let distribution = args.get(3).map(String::as_str).unwrap_or("equal");
        ensure!(
            ["equal", "weighted"].contains(&distribution),
            "unknown distribution"
        );
        input = fixture::generate(
            input,
            args[1].parse()?,
            args[2].parse()?,
            distribution == "weighted",
        )?;
    }
    ensure!(
        input.signed_header.commit.round.value() <= u16::MAX as u32,
        "round exceeds the pinned Borsh header format"
    );
    let set = validator::Set::without_proposer(input.validators);
    ensure!(
        set.hash() == input.signed_header.header.validators_hash,
        "validator hash mismatch"
    );
    ensure!(
        input.signed_header.header.hash() == input.signed_header.commit.block_id.hash,
        "commit does not bind header"
    );
    ensure!(
        input.signed_header.header.height == input.signed_header.commit.height,
        "commit height mismatch"
    );
    let header = Header {
        signed_header: input.signed_header,
        validator_set: set,
        trusted_height: Height::new(input.revision, input.trusted_height)?,
        trusted_next_validator_set: validator::Set::without_proposer(input.trusted_validators),
    };
    let block = &header.signed_header.header;
    let mut signatures = Vec::new();
    for sig in &header.signed_header.commit.signatures {
        // Nil votes do not authorize this block. The original signature vector
        // stays intact in the header for the upstream light-client verifier.
        let CommitSig::BlockIdFlagCommit {
            validator_address,
            timestamp,
            signature,
        } = sig
        else {
            continue;
        };
        let validator = header
            .validator_set
            .validators()
            .iter()
            .find(|v| v.address == *validator_address)
            .context("signature references unknown validator")?;
        let tendermint::PublicKey::Ed25519(pubkey) = validator.pub_key else {
            bail!("only Ed25519 is supported")
        };
        let signature = signature.as_ref().context("missing commit signature")?;
        let vote = CanonicalVote {
            vote_type: tendermint::vote::Type::Precommit,
            height: header.signed_header.commit.height,
            round: header.signed_header.commit.round,
            block_id: Some(header.signed_header.commit.block_id),
            timestamp: Some(*timestamp),
            chain_id: block.chain_id.clone(),
        };
        let message = <CanonicalVote as Protobuf<tendermint_proto::v0_38::types::CanonicalVote>>::encode_length_delimited_vec(vote);
        let mut digest = Sha256::new();
        digest.update(pubkey.as_bytes());
        digest.update(&message);
        digest.update(signature.as_bytes());
        signatures.push(json!({"pubkey": hex::encode(pubkey.as_bytes()), "message": hex::encode(message),
            "signature": hex::encode(signature.as_bytes()), "hash": hex::encode(digest.finalize())}));
    }
    let consensus = json!({"height": block.height.value().to_string(), "timestamp": block.time.unix_timestamp_nanos().to_string(),
        "root": hex::encode(block.app_hash.as_bytes()), "nextValidatorsHash": hex::encode(block.next_validators_hash.as_bytes()),
        "blockHash": hex::encode(block.hash().as_bytes())});
    let chain_id = block.chain_id.to_string();
    let bytes = header_to_borsh(header).try_to_vec()?;
    println!(
        "{}",
        json!({"chainId": chain_id, "header": hex::encode(bytes), "consensus": consensus, "signatures": signatures})
    );
    Ok(())
}
