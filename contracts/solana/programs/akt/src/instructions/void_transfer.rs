//! `void_transfer`: resolves a record that Akash governance voided.

use super::resolution::{require_top_level_invocation, resolve};
use crate::{
    constants::*,
    light_client::{self, MembershipAccounts, ProvenValue},
    state::{BridgeConfig, ProofBuffer, TransferOutcome, TransferStatus, VoidReason},
    wire::{self, TransferId},
};
use anchor_lang::prelude::*;

#[derive(Accounts)]
#[instruction(id: TransferId)]
pub struct VoidTransfer<'info> {
    /// Pays rent for the status account.
    #[account(mut)]
    pub payer: Signer<'info>,
    /// Pinned client. `bridge_enabled` is not read: governance voids work while
    /// minting is paused.
    #[account(seeds = [BRIDGE_SEED], bump)]
    pub bridge: Account<'info, BridgeConfig>,
    /// A complete proof of the settlement key of `id`, from any submitter's buffer.
    #[account(seeds = [PROOF_SEED, proof_buffer.submitter.as_ref(), proof_buffer.nonce.as_ref()], bump)]
    pub proof_buffer: Box<Account<'info, ProofBuffer>>,
    /// Terminal status. `init` fails if the record already has one.
    #[account(init, payer = payer, space = 8 + TransferStatus::INIT_SPACE,
        seeds = [TRANSFER_SEED, &wire::owner_seed(&id.owner), &id.height.to_le_bytes(), &id.sequence.to_le_bytes()], bump)]
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
    /// (`proof_height`); the pinned library verifies against its root.
    pub proof_consensus: UncheckedAccount<'info>,
    pub system_program: Program<'info, System>,
}

/// Verifies the whole native settlement and its permanent governance receipt.
pub(crate) fn void_transfer(
    ctx: Context<VoidTransfer>,
    id: TransferId,
    settlement_status: u8,
    proof_height: u64,
) -> Result<()> {
    require_top_level_invocation()?;
    id.validate_proof_height(proof_height)?;
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
        wire::settlement_key(&id, settlement_status)?,
        ProvenValue::AuthorizedVoid {
            deposit_height: id.height,
        },
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
