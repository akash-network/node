//! The pause guardian: a key that can disable minting without the upgrade
//! authority's time lock, and nothing else.

use super::upgrade_authority::*;
use crate::{
    constants::*,
    errors::AktError,
    events::{GuardianPaused, PauseGuardianSet},
    state::{LaunchConfig, PauseGuardian},
};
use anchor_lang::prelude::*;

#[derive(Accounts)]
pub struct SetPauseGuardian<'info> {
    /// Pays rent when the guardian account is created.
    pub admin: UpgradeAuthority<'info>,
    /// `init_if_needed`: the first call creates the account and later calls
    /// overwrite it. Reinitialization is harmless here: only the upgrade
    /// authority reaches the handler, the handler rewrites every field, the
    /// address is the canonical PDA and nothing closes the account.
    #[account(init_if_needed, payer = admin.authority, space = 8 + PauseGuardian::INIT_SPACE,
        seeds = [GUARDIAN_SEED], bump)]
    pub pause_guardian: Account<'info, PauseGuardian>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct GuardianPause<'info> {
    pub guardian: Signer<'info>,
    /// Must name `guardian`. Before the first `set_pause_guardian` the account
    /// does not exist and every call fails.
    #[account(seeds = [GUARDIAN_SEED], bump = pause_guardian.bump,
        constraint = pause_guardian.guardian == Some(guardian.key()) @ AktError::UnauthorizedGuardian)]
    pub pause_guardian: Account<'info, PauseGuardian>,
    #[account(mut, seeds = [CONFIG_SEED], bump = config.bump)]
    pub config: Account<'info, LaunchConfig>,
}

/// Names, rotates or clears (`None`) the guardian.
pub(crate) fn set_pause_guardian(
    ctx: Context<SetPauseGuardian>,
    guardian: Option<Pubkey>,
) -> Result<()> {
    // A newly created account reads as `None`.
    let previous = ctx.accounts.pause_guardian.guardian;
    ctx.accounts.pause_guardian.set_inner(PauseGuardian {
        guardian,
        bump: ctx.bumps.pause_guardian,
    });
    emit!(PauseGuardianSet { previous, guardian });
    Ok(())
}

/// Disables `mint_transfer`. Only the upgrade authority can enable it again.
pub(crate) fn guardian_pause(ctx: Context<GuardianPause>) -> Result<()> {
    ctx.accounts.config.bridge_enabled = false;
    emit!(GuardianPaused {
        guardian: ctx.accounts.guardian.key(),
    });
    Ok(())
}
