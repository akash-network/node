//! `initialize`: genesis.

use crate::{constants::*, errors::AktError, events::LaunchInitialized, state::LaunchConfig};
use anchor_lang::prelude::*;
use anchor_spl::token::{Mint, Token};

#[derive(Accounts)]
pub struct Initialize<'info> {
    /// This program. Anchor checks its address and executable flag.
    #[account(
        constraint = program.programdata_address()? == Some(program_data.key())
            @ AktError::InvalidProgramData
    )]
    pub program: Program<'info, crate::program::AktSolana>,
    /// This program's ProgramData. Anchor checks the loader owns it; its
    /// recorded upgrade authority must sign.
    #[account(
        constraint = program_data.upgrade_authority_address == Some(authority.key())
            @ AktError::UnauthorizedInitializer
    )]
    pub program_data: Account<'info, ProgramData>,
    /// Pays rent for every account created here.
    #[account(mut)]
    pub authority: Signer<'info>,
    /// Created here; `init` fails if genesis already ran.
    #[account(
        init,
        payer = authority,
        space = 8 + LaunchConfig::INIT_SPACE,
        seeds = [CONFIG_SEED],
        bump
    )]
    pub config: Account<'info, LaunchConfig>,
    /// Legacy SPL mint with six decimals, the mint-authority PDA as mint
    /// authority and no freeze authority. Its supply starts at zero.
    #[account(
        init,
        payer = authority,
        seeds = [MINT_SEED],
        bump,
        mint::decimals = AKT_DECIMALS,
        mint::authority = mint_authority,
        mint::token_program = token_program
    )]
    pub mint: Account<'info, Mint>,
    /// CHECK: The `["mint_authority"]` PDA. It holds no data and only signs
    /// CPIs from this program.
    #[account(seeds = [MINT_AUTHORITY_SEED], bump)]
    pub mint_authority: UncheckedAccount<'info>,
    pub token_program: Program<'info, Token>,
    pub system_program: Program<'info, System>,
}

/// Records the launch with minting disabled. The account constraints created
/// the mint with zero supply; only `mint_transfer` ever mints.
pub(crate) fn initialize(ctx: Context<Initialize>) -> Result<()> {
    ctx.accounts.config.set_inner(LaunchConfig {
        version: LAUNCH_CONFIG_VERSION,
        initializer: ctx.accounts.authority.key(),
        mint: ctx.accounts.mint.key(),
        bridge_enabled: false,
        initialized_at_slot: Clock::get()?.slot,
        bump: ctx.bumps.config,
        mint_authority_bump: ctx.bumps.mint_authority,
    });

    emit!(LaunchInitialized {
        mint: ctx.accounts.mint.key(),
        initializer: ctx.accounts.authority.key(),
    });
    Ok(())
}
