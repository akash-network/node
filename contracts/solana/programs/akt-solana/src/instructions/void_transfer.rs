//! `void_transfer`: resolves a record that Akash governance voided.

use super::resolution::{require_top_level_invocation, resolve};
use crate::{
    constants::*,
    errors::AktError,
    light_client::{self, MembershipAccounts},
    state::{BridgeConfig, ProofBuffer, TransferOutcome, TransferStatus, VoidReason},
    wire,
};
use anchor_lang::prelude::*;

#[derive(Accounts)]
#[instruction(id: u64)]
pub struct VoidTransfer<'info> {
    /// Pays rent for the status account.
    #[account(mut)]
    pub payer: Signer<'info>,
    /// Pinned client. `bridge_enabled` is not read: governance voids work while
    /// minting is paused.
    #[account(seeds = [BRIDGE_SEED], bump)]
    pub bridge: Account<'info, BridgeConfig>,
    /// A complete proof of the void key of `id`, from any submitter's buffer.
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
    /// Terminal status. `init` fails if the record already has one.
    #[account(init, payer = payer, space = 8 + TransferStatus::INIT_SPACE,
        seeds = [TRANSFER_SEED, &id.to_le_bytes()], bump)]
    pub status: Account<'info, TransferStatus>,
    /// CHECK: Must be `bridge.light_client_program` and executable.
    #[account(address = bridge.light_client_program, executable)]
    pub light_client_program: UncheckedAccount<'info>,
    /// CHECK: `light_client::verify_membership` checks its owner, PDA, type,
    /// chain ID, frozen height and trusting period.
    pub client_state: UncheckedAccount<'info>,
    /// CHECK: `light_client::verify_membership` checks its owner, PDA and height
    /// (the client's latest) and that it is inside the trusting period.
    pub latest_consensus: UncheckedAccount<'info>,
    /// CHECK: `light_client::verify_membership` checks its owner, PDA and height
    /// (`proof_height`); ICS07 verifies the proof against its root.
    pub proof_consensus: UncheckedAccount<'info>,
    pub system_program: Program<'info, System>,
}

/// Verifies that the governance void order at the void key of `id` holds
/// `[1, id as little-endian u64]` at `proof_height` and records
/// `Voided / Governance`.
pub(crate) fn void_transfer(ctx: Context<VoidTransfer>, id: u64, proof_height: u64) -> Result<()> {
    require_top_level_invocation()?;
    require!(id > 0, AktError::InvalidRecord);
    light_client::verify_membership(
        MembershipAccounts {
            program: &ctx.accounts.light_client_program,
            client: &ctx.accounts.client_state,
            latest: &ctx.accounts.latest_consensus,
            consensus: &ctx.accounts.proof_consensus,
        },
        &ctx.accounts.bridge,
        &ctx.accounts.proof_buffer,
        proof_height,
        wire::void_key(id),
        wire::void_value(id),
    )?;
    resolve(
        &mut ctx.accounts.status,
        id,
        proof_height,
        TransferOutcome::Voided {
            reason: VoidReason::Governance,
        },
    )
}
