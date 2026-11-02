//! Events, emitted with `emit!` as `Program data:` log lines.
//!
//! Anchor's TypeScript `EventParser` ignores events logged inside a CPI.
//! `mint_transfer` and `void_transfer` reject CPI for that reason. Admin
//! instructions do not: a multisig upgrade authority calls them through CPI,
//! so monitors must attribute their events to the AKT invocation frame.

use crate::state::{MintPolicy, TransferOutcome};
use anchor_lang::prelude::*;

/// `initialize` created the launch record and the AKT mint with zero supply.
#[event]
pub struct LaunchInitialized {
    pub mint: Pubkey,
    /// The upgrade authority that ran genesis.
    pub initializer: Pubkey,
}

/// `set_token_metadata` created or updated the mint's Metaplex metadata.
#[event]
pub struct TokenMetadataSet {
    pub mint: Pubkey,
    pub metadata: Pubkey,
    pub uri: String,
}

/// `configure_bridge` pinned the light client and set the first mint policy.
/// Minting stays disabled.
#[event]
pub struct BridgeConfigured {
    pub light_client_program: Pubkey,
    pub chain_id: String,
    pub policy: MintPolicy,
}

/// The upgrade authority set `LaunchConfig::bridge_enabled`.
#[event]
pub struct BridgeEnabledSet {
    pub enabled: bool,
    /// The signing upgrade authority.
    pub authority: Pubkey,
}

/// `set_mint_policy` replaced the policy. The day's usage carries over.
#[event]
pub struct MintPolicySet {
    pub previous: MintPolicy,
    pub policy: MintPolicy,
}

/// `set_pause_guardian` named, rotated or cleared (`None`) the guardian.
#[event]
pub struct PauseGuardianSet {
    pub previous: Option<Pubkey>,
    pub guardian: Option<Pubkey>,
}

/// The pause guardian disabled bridge minting.
#[event]
pub struct GuardianPaused {
    pub guardian: Pubkey,
}

/// `repin_light_client` moved the pin. Policy and usage carry over.
#[event]
pub struct LightClientRepinned {
    pub previous_program: Pubkey,
    pub program: Pubkey,
    pub previous_chain_id: String,
    pub chain_id: String,
}

/// A record reached its terminal status, stored at `["transfer", id]`.
#[event]
pub struct TransferResolved {
    pub id: u64,
    pub proof_height: u64,
    pub outcome: TransferOutcome,
}
