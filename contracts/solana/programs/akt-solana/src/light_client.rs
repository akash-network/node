//! Checks on the pinned ICS07 Tendermint client, and the membership CPI.
//!
//! The upgrade authority chooses the light-client program in
//! `configure_bridge` or `repin_light_client`; `BridgeConfig` stores it. No
//! client account is trusted because a caller passed it: each must be owned by
//! that program, sit at its PDA and deserialize as its account type. The
//! generated bindings in `ics07-tendermint-interface` supply only the account
//! layouts and the CPI.

use crate::{
    constants::{ICS07_CLIENT_SEED, ICS07_CONSENSUS_STATE_SEED},
    errors::AktError,
    state::{BridgeConfig, ProofBuffer},
    wire::SINK_STORE,
};
use anchor_lang::prelude::*;
use ics07_tendermint_interface::ics07_tendermint::{
    self,
    accounts::{ClientState, ConsensusStateStore},
};

const NANOS_PER_SECOND: u128 = 1_000_000_000;

/// Loads the client state of deployment `program`: owned by it, at its
/// `["client"]` PDA, with the ICS07 `ClientState` discriminator.
pub fn load_client_state(account: &AccountInfo, program: &Pubkey) -> Result<ClientState> {
    require_keys_eq!(*account.owner, *program, AktError::InvalidLightClient);
    require_keys_eq!(
        account.key(),
        Pubkey::find_program_address(&[ICS07_CLIENT_SEED], program).0,
        AktError::InvalidLightClient
    );
    ClientState::try_deserialize(&mut account.try_borrow_data()?.as_ref())
}

/// Loads deployment `program`'s consensus state at `height`: owned by it, at
/// its `["consensus_state", height]` PDA, and recording that height.
fn load_consensus_state(
    account: &AccountInfo,
    program: &Pubkey,
    height: u64,
) -> Result<ConsensusStateStore> {
    require_keys_eq!(*account.owner, *program, AktError::InvalidLightClient);
    require_keys_eq!(
        account.key(),
        Pubkey::find_program_address(
            &[ICS07_CONSENSUS_STATE_SEED, &height.to_le_bytes()],
            program
        )
        .0,
        AktError::InvalidLightClient
    );
    let state = ConsensusStateStore::try_deserialize(&mut account.try_borrow_data()?.as_ref())?;
    require!(state.height == height, AktError::InvalidLightClient);
    Ok(state)
}

/// Rejects a client for another chain, a frozen client, and one whose trusting
/// period is zero or not shorter than its unbonding period.
fn check_client_state(client: &ClientState, chain_id: &str) -> Result<()> {
    require!(client.chain_id == chain_id, AktError::WrongChain);
    require!(
        client.frozen_height.revision_height == 0 && client.frozen_height.revision_number == 0,
        AktError::ClientFrozen
    );
    require!(
        client.trusting_period > 0 && client.trusting_period < client.unbonding_period,
        AktError::InvalidLightClient
    );
    Ok(())
}

/// Requires `client` to track `chain_id`, to be unfrozen and to be unexpired:
/// its latest consensus state, loaded from `latest`, must be inside the
/// trusting period on the Solana clock. Runs when a client is pinned and
/// before every proof.
pub fn check_active(
    client: &ClientState,
    latest: &AccountInfo,
    program: &Pubkey,
    chain_id: &str,
) -> Result<()> {
    check_client_state(client, chain_id)?;
    let state = load_consensus_state(latest, program, client.latest_height.revision_height)?;
    let now = u64::try_from(Clock::get()?.unix_timestamp).map_err(|_| AktError::InvalidClock)?;
    check_consensus_age(
        now,
        state.consensus_state.timestamp,
        client.trusting_period,
        client.max_clock_drift,
    )
}

/// Expiry rule for the latest consensus state. The timestamp must be nonzero
/// and at most `now + drift`, and `now` must be before `timestamp + trusting`.
/// That is ibc-go's `IsExpired` boundary, which the consensus-pruning patch
/// also uses, so a state this rule accepts cannot have been pruned. Each term
/// is below 2^64 * 10^9 < 2^94, so the u128 sums cannot overflow.
fn check_consensus_age(
    now_seconds: u64,
    timestamp_nanos: u64,
    trusting_seconds: u64,
    drift_seconds: u64,
) -> Result<()> {
    let now = u128::from(now_seconds) * NANOS_PER_SECOND;
    let timestamp = u128::from(timestamp_nanos);
    require!(
        timestamp > 0 && timestamp <= now + u128::from(drift_seconds) * NANOS_PER_SECOND,
        AktError::InvalidConsensusTimestamp
    );
    require!(
        now < timestamp + u128::from(trusting_seconds) * NANOS_PER_SECOND,
        AktError::ClientExpired
    );
    Ok(())
}

/// Light-client accounts of a mint or void.
pub struct MembershipAccounts<'a, 'info> {
    /// The pinned light-client program.
    pub program: &'a AccountInfo<'info>,
    /// Its client state.
    pub client: &'a AccountInfo<'info>,
    /// Its consensus state at the client's latest height, for expiry.
    pub latest: &'a AccountInfo<'info>,
    /// Its consensus state at the proof height, whose root ICS07 verifies
    /// against.
    pub consensus: &'a AccountInfo<'info>,
}

/// Verifies through the pinned client that sink store key `key` holds exactly
/// `value` at verified height `height`. Before the CPI it requires the pinned,
/// executable program, an active client (`check_active`), `1 <= height <=`
/// the client's latest height, the client's consensus state at `height`, and
/// a complete proof buffer. The program builds the path `["sink", key]` and
/// the value itself; the caller supplies only the proof.
pub fn verify_membership(
    accounts: MembershipAccounts<'_, '_>,
    bridge: &BridgeConfig,
    buffer: &ProofBuffer,
    height: u64,
    key: Vec<u8>,
    value: Vec<u8>,
) -> Result<()> {
    // Account constraints already pin the program; this keeps the function
    // safe on its own.
    require_keys_eq!(
        accounts.program.key(),
        bridge.light_client_program,
        AktError::InvalidLightClient
    );
    require!(accounts.program.executable, AktError::InvalidLightClient);
    let client = load_client_state(accounts.client, &bridge.light_client_program)?;
    check_active(
        &client,
        accounts.latest,
        &bridge.light_client_program,
        &bridge.chain_id,
    )?;
    require!(
        height > 0 && height <= client.latest_height.revision_height,
        AktError::InvalidProofHeight
    );
    load_consensus_state(accounts.consensus, &bridge.light_client_program, height)?;
    require!(buffer.is_complete(), AktError::IncompleteProof);

    ics07_tendermint::cpi::verify_membership(
        CpiContext::new(
            accounts.program.clone(),
            ics07_tendermint::cpi::accounts::VerifyMembership {
                client_state: accounts.client.clone(),
                consensus_state_at_height: accounts.consensus.clone(),
            },
        ),
        ics07_tendermint::types::MembershipMsg {
            height,
            proof: buffer.data.clone(),
            path: vec![SINK_STORE.to_vec(), key],
            value,
        },
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use ics07_tendermint::types::IbcHeight;

    fn client() -> ClientState {
        ClientState {
            chain_id: "akash-local-1".into(),
            trust_level_numerator: 1,
            trust_level_denominator: 3,
            trusting_period: 100,
            unbonding_period: 200,
            max_clock_drift: 5,
            frozen_height: IbcHeight {
                revision_number: 0,
                revision_height: 0,
            },
            latest_height: IbcHeight {
                revision_number: 1,
                revision_height: 42,
            },
        }
    }

    #[test]
    fn frozen_and_wrong_chain_clients_are_rejected() {
        let mut client = client();
        assert!(check_client_state(&client, "akash-local-1").is_ok());
        assert_eq!(
            check_client_state(&client, "other-chain").unwrap_err(),
            AktError::WrongChain.into()
        );
        client.frozen_height = IbcHeight {
            revision_number: 1,
            revision_height: 42,
        };
        assert_eq!(
            check_client_state(&client, "akash-local-1").unwrap_err(),
            AktError::ClientFrozen.into()
        );
    }

    #[test]
    fn trusting_period_must_be_nonzero_and_below_unbonding() {
        for (trusting_period, unbonding_period) in [(0, 200), (200, 200), (300, 200)] {
            let client = ClientState {
                trusting_period,
                unbonding_period,
                ..client()
            };
            assert_eq!(
                check_client_state(&client, "akash-local-1").unwrap_err(),
                AktError::InvalidLightClient.into()
            );
        }
    }

    #[test]
    fn expiry_boundary_and_future_time() {
        assert!(check_consensus_age(109, 100_000_000_000, 10, 5).is_ok());
        assert_eq!(
            check_consensus_age(110, 100_000_000_000, 10, 5).unwrap_err(),
            AktError::ClientExpired.into()
        );
        assert_eq!(
            check_consensus_age(94, 100_000_000_000, 10, 5).unwrap_err(),
            AktError::InvalidConsensusTimestamp.into()
        );
        assert!(check_consensus_age(95, 100_000_000_000, 10, 5).is_ok());
        assert_eq!(
            check_consensus_age(95, 0, 10, 5).unwrap_err(),
            AktError::InvalidConsensusTimestamp.into()
        );
        // Extreme inputs neither overflow nor pass by wrapping.
        assert!(check_consensus_age(u64::MAX, u64::MAX, u64::MAX, u64::MAX).is_ok());
        assert_eq!(
            check_consensus_age(u64::MAX, 1, 0, 0).unwrap_err(),
            AktError::ClientExpired.into()
        );
    }
}
