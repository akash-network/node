//! The upgrade-authority check shared by every admin instruction.

use crate::errors::AktError;
use anchor_lang::prelude::*;

/// Proves the signer is this program's current upgrade authority. `program`
/// must be this program (Anchor checks its address and executable flag),
/// `program_data` its ProgramData account (Anchor checks the loader owns it),
/// and the upgrade authority recorded there must be `authority`. If the
/// program is made immutable, every admin instruction fails, including
/// `repin_light_client`.
///
/// `authority` is writable because it pays rent in `configure_bridge`,
/// `set_pause_guardian` and `set_token_metadata`. The other admin
/// instructions share this struct, which is part of the deployed interface.
#[derive(Accounts)]
pub struct UpgradeAuthority<'info> {
    #[account(constraint = program.programdata_address()? == Some(program_data.key()) @ AktError::InvalidProgramData)]
    pub program: Program<'info, crate::program::AktSolana>,
    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ AktError::InvalidAuthority)]
    pub program_data: Account<'info, ProgramData>,
    #[account(mut)]
    pub authority: Signer<'info>,
}
