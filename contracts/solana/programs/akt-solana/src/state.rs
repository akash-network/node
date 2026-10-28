//! Accounts owned by this program and the rules that update them.
//!
//! Account names, field order and field types are part of the deployed
//! interface; `tests/idl.test.ts` compares them with the deployed IDL.

use crate::{constants::*, errors::AktError};
use anchor_lang::prelude::*;

/// Launch record at `["config"]`. `initialize` creates it once and nothing
/// closes it, so genesis cannot run again.
#[account]
#[derive(InitSpace)]
pub struct LaunchConfig {
    /// Layout version, `LAUNCH_CONFIG_VERSION`.
    pub version: u8,
    /// Upgrade authority that ran genesis.
    pub initializer: Pubkey,
    /// The AKT mint at `["mint"]`.
    pub mint: Pubkey,
    /// Whether `mint_transfer` may run. Only the program upgrade authority can
    /// enable it; the upgrade authority or the pause guardian can disable it.
    pub bridge_enabled: bool,
    /// Slot of genesis.
    pub initialized_at_slot: u64,
    /// Bump of `["config"]`.
    pub bump: u8,
    /// Bump of `["mint_authority"]`, used to sign mint and metadata CPIs.
    pub mint_authority_bump: u8,
}

/// Bridge settings at `["bridge"]`, created once by `configure_bridge`.
#[account]
#[derive(InitSpace)]
pub struct BridgeConfig {
    /// ICS07 deployment whose verified state authorizes mints and voids.
    /// Only `repin_light_client` changes it.
    pub light_client_program: Pubkey,
    /// Akash chain ID the pinned client must track.
    #[max_len(MAX_CHAIN_ID_LEN)]
    pub chain_id: String,
    /// Mint limits and vault allowlist.
    pub policy: MintPolicy,
    /// UTC day number (Solana unix time / 86,400) of `minted_today`.
    pub mint_day: u64,
    /// Base units minted by `mint_transfer` on `mint_day`.
    pub minted_today: u64,
}

impl BridgeConfig {
    /// Returns the day's minted total after minting `amount` on `day`, or the
    /// limit that blocks it. A day before `mint_day` means the clock moved
    /// backwards. The caller stores the result only once the mint succeeds.
    pub(crate) fn next_mint_total(&self, day: u64, amount: u64) -> Result<u64> {
        require!(day >= self.mint_day, AktError::InvalidClock);
        require!(
            amount <= self.policy.per_transfer_max,
            AktError::TransferLimit
        );
        let used = if self.mint_day == day {
            self.minted_today
        } else {
            0
        };
        let total = used.checked_add(amount).ok_or(AktError::DailyLimit)?;
        require!(total <= self.policy.daily_mint_cap, AktError::DailyLimit);
        Ok(total)
    }
}

/// Limits on bridge issuance, set by the upgrade authority.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, InitSpace)]
pub struct MintPolicy {
    /// Largest amount one record may mint, in base units.
    pub per_transfer_max: u64,
    /// Most base units all records together may mint per UTC day.
    pub daily_mint_cap: u64,
    /// Recipients accepted although they are off-curve or not owned by the
    /// System Program, such as vault PDAs.
    #[max_len(MAX_VAULTS)]
    pub vaults: Vec<Pubkey>,
}

impl MintPolicy {
    /// Requires `0 < per_transfer_max <= daily_mint_cap` and at most
    /// `MAX_VAULTS` vaults, none of them the zero key.
    pub(crate) fn validate(&self) -> Result<()> {
        require!(
            self.per_transfer_max > 0
                && self.per_transfer_max <= self.daily_mint_cap
                && self.vaults.len() <= MAX_VAULTS
                && !self.vaults.contains(&Pubkey::default()),
            AktError::InvalidPolicy
        );
        Ok(())
    }
}

/// Pause guardian at `["guardian"]`. The guardian can pause minting without
/// waiting out the upgrade authority's time lock. It cannot enable minting or
/// change anything else.
#[account]
#[derive(InitSpace)]
pub struct PauseGuardian {
    /// Key allowed to call `guardian_pause`; `None` allows no one.
    pub guardian: Option<Pubkey>,
    /// Bump of `["guardian"]`.
    pub bump: u8,
}

/// Upload buffer for one ICS23 proof at `["proof", submitter, nonce]`. Only the
/// submitter writes or closes it; any mint or void may read a complete one.
#[account]
#[derive(InitSpace)]
pub struct ProofBuffer {
    /// Creator. It paid the rent and receives it back on close.
    pub submitter: Pubkey,
    /// Caller-chosen value that distinguishes the submitter's buffers.
    pub nonce: u64,
    /// Proof length declared at creation, 1 to `MAX_PROOF_BYTES`.
    pub expected_len: u32,
    /// Bytes written so far, in order.
    #[max_len(MAX_PROOF_BYTES)]
    pub data: Vec<u8>,
}

impl ProofBuffer {
    /// Whether every declared byte has been written.
    pub fn is_complete(&self) -> bool {
        !self.data.is_empty() && self.data.len() == self.expected_len as usize
    }
}

/// Terminal outcome of one x/sink record at `["transfer", id]`.
/// `mint_transfer` and `void_transfer` both create it with `init`, and nothing
/// closes it, so each record ID resolves exactly once.
#[account]
#[derive(InitSpace)]
pub struct TransferStatus {
    /// x/sink record ID.
    pub id: u64,
    /// Verified Akash height whose state proved the record or void order.
    pub proof_height: u64,
    /// Slot of resolution. Reporters read this slot's block for the event.
    pub resolved_at_slot: u64,
    pub outcome: TransferOutcome,
}

/// How a record resolved.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, InitSpace)]
pub enum TransferOutcome {
    /// `amount` base units were minted to the associated token account of
    /// `recipient`.
    Minted { amount: u64, recipient: Pubkey },
    /// Nothing was minted; Akash may refund the deposit.
    Voided { reason: VoidReason },
}

/// Why a record was voided.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, InitSpace)]
pub enum VoidReason {
    /// The proven recipient cannot receive AKT; see `mint_transfer`.
    InvalidRecipient,
    /// Akash governance wrote a void order for the record.
    Governance,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cap_rollover_overflow_and_clock_regression() {
        let mut config = BridgeConfig {
            light_client_program: Pubkey::new_unique(),
            chain_id: "local".into(),
            policy: MintPolicy {
                per_transfer_max: 100,
                daily_mint_cap: 100,
                vaults: vec![],
            },
            mint_day: 7,
            minted_today: 90,
        };
        assert_eq!(config.next_mint_total(7, 10).unwrap(), 100);
        assert_eq!(
            config.next_mint_total(7, 11).unwrap_err(),
            AktError::DailyLimit.into()
        );
        assert_eq!(config.next_mint_total(8, 100).unwrap(), 100);
        assert_eq!(
            config.next_mint_total(8, 101).unwrap_err(),
            AktError::TransferLimit.into()
        );
        assert_eq!(
            config.next_mint_total(6, 1).unwrap_err(),
            AktError::InvalidClock.into()
        );
        config.policy.per_transfer_max = u64::MAX;
        config.policy.daily_mint_cap = u64::MAX;
        config.minted_today = u64::MAX;
        assert_eq!(
            config.next_mint_total(7, 1).unwrap_err(),
            AktError::DailyLimit.into()
        );
    }

    #[test]
    fn policies_need_ordered_nonzero_limits_and_bounded_vaults() {
        let valid = MintPolicy {
            per_transfer_max: 10,
            daily_mint_cap: 10,
            vaults: vec![Pubkey::new_unique(); MAX_VAULTS],
        };
        assert!(valid.validate().is_ok());
        let invalid = [
            MintPolicy {
                per_transfer_max: 0,
                ..valid.clone()
            },
            MintPolicy {
                daily_mint_cap: 9,
                ..valid.clone()
            },
            MintPolicy {
                vaults: vec![Pubkey::new_unique(); MAX_VAULTS + 1],
                ..valid.clone()
            },
            MintPolicy {
                vaults: vec![Pubkey::default()],
                ..valid.clone()
            },
        ];
        for policy in invalid {
            assert_eq!(
                policy.validate().unwrap_err(),
                AktError::InvalidPolicy.into()
            );
        }
    }

    /// Sizes in docs/audit.md, including the 8-byte discriminator. A change
    /// would break existing accounts.
    #[test]
    fn account_sizes_match_the_deployed_layout() {
        assert_eq!(8 + LaunchConfig::INIT_SPACE, 84);
        assert_eq!(8 + BridgeConfig::INIT_SPACE, 656);
        assert_eq!(8 + PauseGuardian::INIT_SPACE, 42);
        assert_eq!(8 + ProofBuffer::INIT_SPACE, 4152);
        assert_eq!(8 + TransferStatus::INIT_SPACE, 73);
    }
}
