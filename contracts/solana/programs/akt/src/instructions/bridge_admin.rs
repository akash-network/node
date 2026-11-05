//! Bridge administration by the program upgrade authority: pinning the light
//! client, enabling minting and setting the mint policy.

use super::upgrade_authority::*;
use crate::{
    constants::*,
    errors::AktError,
    events::{BridgeConfigured, BridgeEnabledSet, LightClientRepinned, MintPolicySet},
    light_client,
    state::{BridgeConfig, LaunchConfig, MintPolicy},
};
use anchor_lang::prelude::*;

#[derive(Accounts)]
pub struct ConfigureBridge<'info> {
    /// Pays rent for `bridge`.
    pub admin: UpgradeAuthority<'info>,
    /// Requiring the launch record orders configuration after genesis.
    #[account(seeds = [CONFIG_SEED], bump = config.bump)]
    pub config: Account<'info, LaunchConfig>,
    /// Created here; `init` makes configuration one-time.
    #[account(init, payer = admin.authority, space = 8 + BridgeConfig::INIT_SPACE, seeds = [BRIDGE_SEED], bump)]
    pub bridge: Account<'info, BridgeConfig>,
    /// CHECK: The ICS07 deployment to pin, chosen by the upgrade authority.
    /// Must be executable; its client accounts are checked below.
    #[account(executable)]
    pub light_client_program: UncheckedAccount<'info>,
    /// CHECK: `light_client::load_client_state` checks its owner, PDA and
    /// type; `check_active` its chain ID, frozen height and trusting period.
    pub client_state: UncheckedAccount<'info>,
    /// CHECK: `light_client::check_active` checks its owner, PDA and height
    /// (the client's latest) and that it is inside the trusting period.
    pub latest_consensus: UncheckedAccount<'info>,
    pub system_program: Program<'info, System>,
}

/// Accounts of `set_bridge_enabled` and `set_mint_policy`. The first writes
/// `config` and the second `bridge`; both are writable because the two
/// instructions share this struct, which is part of the deployed interface.
/// Requiring `bridge` means minting cannot be enabled before configuration.
#[derive(Accounts)]
pub struct ManageBridge<'info> {
    pub admin: UpgradeAuthority<'info>,
    #[account(mut, seeds = [CONFIG_SEED], bump = config.bump)]
    pub config: Account<'info, LaunchConfig>,
    #[account(mut, seeds = [BRIDGE_SEED], bump)]
    pub bridge: Account<'info, BridgeConfig>,
}

#[derive(Accounts)]
pub struct RepinLightClient<'info> {
    pub admin: UpgradeAuthority<'info>,
    #[account(mut, seeds = [BRIDGE_SEED], bump)]
    pub bridge: Account<'info, BridgeConfig>,
    /// CHECK: The replacement ICS07 deployment, chosen by the upgrade
    /// authority. Must be executable; its client accounts are checked below.
    #[account(executable)]
    pub light_client_program: UncheckedAccount<'info>,
    /// CHECK: `light_client::load_client_state` checks its owner, PDA and
    /// type; `check_active` its chain ID, frozen height and trusting period.
    pub client_state: UncheckedAccount<'info>,
    /// CHECK: `light_client::check_active` checks its owner, PDA and height
    /// (the client's latest) and that it is inside the trusting period.
    pub latest_consensus: UncheckedAccount<'info>,
}

/// Pins the light-client deployment and chain ID after checking that the
/// client is active, and stores the first policy with zero usage. Minting
/// stays disabled until `set_bridge_enabled`.
pub(crate) fn configure_bridge(
    ctx: Context<ConfigureBridge>,
    chain_id: String,
    policy: MintPolicy,
) -> Result<()> {
    check_chain_id(&chain_id)?;
    policy.validate()?;
    let program = ctx.accounts.light_client_program.key();
    let client = light_client::load_client_state(&ctx.accounts.client_state, &program)?;
    light_client::check_active(&client, &ctx.accounts.latest_consensus, &program, &chain_id)?;
    ctx.accounts.bridge.set_inner(BridgeConfig {
        light_client_program: program,
        chain_id: chain_id.clone(),
        policy: policy.clone(),
        mint_day: 0,
        minted_today: 0,
    });
    emit!(BridgeConfigured {
        light_client_program: program,
        chain_id,
        policy,
    });
    Ok(())
}

/// Sets whether `mint_transfer` may run.
pub(crate) fn set_bridge_enabled(ctx: Context<ManageBridge>, enabled: bool) -> Result<()> {
    ctx.accounts.config.bridge_enabled = enabled;
    emit!(BridgeEnabledSet {
        enabled,
        authority: ctx.accounts.admin.authority.key(),
    });
    Ok(())
}

/// Replaces the policy. The day's usage carries over, so a cap lowered below
/// it blocks mints until the next UTC day.
pub(crate) fn set_mint_policy(ctx: Context<ManageBridge>, policy: MintPolicy) -> Result<()> {
    policy.validate()?;
    let previous = std::mem::replace(&mut ctx.accounts.bridge.policy, policy.clone());
    emit!(MintPolicySet { previous, policy });
    Ok(())
}

/// Pins another deployment and chain ID, with the same checks as
/// `configure_bridge`. The current pin is never read, so a broken, expired or
/// closed deployment cannot block recovery. Transfer statuses are keyed by
/// record ID alone, so no record resolves twice across clients. This adds no
/// trust: the same authority can replace the program.
pub(crate) fn repin_light_client(ctx: Context<RepinLightClient>, chain_id: String) -> Result<()> {
    check_chain_id(&chain_id)?;
    let program = ctx.accounts.light_client_program.key();
    let client = light_client::load_client_state(&ctx.accounts.client_state, &program)?;
    light_client::check_active(&client, &ctx.accounts.latest_consensus, &program, &chain_id)?;
    // Policy and the day's mint usage carry over.
    let bridge = &mut ctx.accounts.bridge;
    let previous_program = std::mem::replace(&mut bridge.light_client_program, program);
    let previous_chain_id = std::mem::replace(&mut bridge.chain_id, chain_id.clone());
    emit!(LightClientRepinned {
        previous_program,
        program,
        previous_chain_id,
        chain_id,
    });
    Ok(())
}

/// A chain ID must be nonempty and fit the space `BridgeConfig` reserves.
fn check_chain_id(chain_id: &str) -> Result<()> {
    require!(
        !chain_id.is_empty() && chain_id.len() <= MAX_CHAIN_ID_LEN,
        AktError::InvalidChainId
    );
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn chain_id_must_fit_the_bridge_account() {
        assert!(check_chain_id("akash-local-1").is_ok());
        assert!(check_chain_id(&"a".repeat(MAX_CHAIN_ID_LEN)).is_ok());
        for chain_id in [String::new(), "a".repeat(MAX_CHAIN_ID_LEN + 1)] {
            assert_eq!(
                check_chain_id(&chain_id).unwrap_err(),
                AktError::InvalidChainId.into()
            );
        }
    }
}
