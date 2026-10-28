//! AKT on Solana.
//!
//! Genesis creates the AKT mint with zero supply. Every AKT on Solana is then
//! minted by `mint_transfer`, for an Akash x/sink transfer record whose
//! membership in Akash state the pinned ICS07 Tendermint light client
//! verifies. Each record ID resolves once, as minted or voided.
//! `docs/audit.md` describes the trust model, accounts and invariants.

use anchor_lang::prelude::*;

declare_id!("FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5");

pub mod constants;
pub mod errors;
pub mod events;
pub mod instructions;
pub mod light_client;
pub mod state;
pub mod wire;

pub use constants::*;
pub use errors::*;
pub use events::*;
pub use instructions::*;
pub use state::*;
pub use wire::TransferRecord;

/// Entrypoints in lifecycle order. Handlers live in `instructions`.
#[program]
pub mod akt_solana {
    use super::*;

    /// Genesis: creates the launch record and the AKT mint. Nothing is
    /// minted; supply starts at zero and bridge minting starts disabled.
    ///
    /// Caller: the program's upgrade authority, once. It pays rent.
    /// Event: `LaunchInitialized`.
    /// Errors: `InvalidProgramData`, `UnauthorizedInitializer`. A second call
    /// fails because `config` exists.
    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        instructions::genesis::initialize(ctx)
    }

    /// Creates or updates the AKT mint's Metaplex metadata. Supply and token
    /// authorities do not change.
    ///
    /// Caller: the upgrade authority. It pays rent on creation.
    /// Event: `TokenMetadataSet`.
    /// Errors: `InvalidAuthority`, `InvalidMetadataUri`, `InvalidMetadata`.
    pub fn set_token_metadata(ctx: Context<SetTokenMetadata>, uri: String) -> Result<()> {
        instructions::token_metadata::set_token_metadata(ctx, uri)
    }

    /// Pins the light-client deployment and Akash chain ID and sets the first
    /// mint policy. One-time; minting stays disabled.
    ///
    /// Caller: the upgrade authority. It pays rent.
    /// Event: `BridgeConfigured`.
    /// Errors: `InvalidChainId`, `InvalidPolicy`, `InvalidLightClient`,
    /// `WrongChain`, `ClientFrozen`, `ClientExpired`,
    /// `InvalidConsensusTimestamp`.
    pub fn configure_bridge(
        ctx: Context<ConfigureBridge>,
        chain_id: String,
        policy: MintPolicy,
    ) -> Result<()> {
        instructions::bridge_admin::configure_bridge(ctx, chain_id, policy)
    }

    /// Enables or disables `mint_transfer`. `void_transfer` is never gated.
    ///
    /// Caller: the upgrade authority, after `configure_bridge`.
    /// Event: `BridgeEnabledSet`.
    /// Errors: `InvalidAuthority`.
    pub fn set_bridge_enabled(ctx: Context<ManageBridge>, enabled: bool) -> Result<()> {
        instructions::bridge_admin::set_bridge_enabled(ctx, enabled)
    }

    /// Replaces the per-transfer limit, daily cap and vault allowlist. The
    /// day's minted total carries over.
    ///
    /// Caller: the upgrade authority.
    /// Event: `MintPolicySet`.
    /// Errors: `InvalidAuthority`, `InvalidPolicy`.
    pub fn set_mint_policy(ctx: Context<ManageBridge>, policy: MintPolicy) -> Result<()> {
        instructions::bridge_admin::set_mint_policy(ctx, policy)
    }

    /// Names, rotates or clears (`None`) the pause guardian.
    ///
    /// Caller: the upgrade authority. It pays rent on the first call.
    /// Event: `PauseGuardianSet`.
    /// Errors: `InvalidAuthority`.
    pub fn set_pause_guardian(
        ctx: Context<SetPauseGuardian>,
        guardian: Option<Pubkey>,
    ) -> Result<()> {
        instructions::pause_guardian::set_pause_guardian(ctx, guardian)
    }

    /// Disables `mint_transfer`. The guardian cannot enable minting or change
    /// any other setting.
    ///
    /// Caller: the pause guardian.
    /// Event: `GuardianPaused`.
    /// Errors: `UnauthorizedGuardian`.
    pub fn guardian_pause(ctx: Context<GuardianPause>) -> Result<()> {
        instructions::pause_guardian::guardian_pause(ctx)
    }

    /// Pins another light-client deployment and chain ID, with the checks of
    /// `configure_bridge`. The current pin is not read, so a broken, expired
    /// or closed deployment cannot block recovery. Policy and usage carry
    /// over.
    ///
    /// Caller: the upgrade authority, at any time.
    /// Event: `LightClientRepinned`.
    /// Errors: those of `configure_bridge` except `InvalidPolicy`.
    pub fn repin_light_client(ctx: Context<RepinLightClient>, chain_id: String) -> Result<()> {
        instructions::bridge_admin::repin_light_client(ctx, chain_id)
    }

    /// Creates the caller's proof buffer `["proof", submitter, nonce]` for a
    /// proof of `expected_len` bytes.
    ///
    /// Caller: anyone. It pays rent and owns the buffer.
    /// Errors: `InvalidProofLength`.
    pub fn create_proof_buffer(
        ctx: Context<CreateProofBuffer>,
        nonce: u64,
        expected_len: u32,
    ) -> Result<()> {
        instructions::proof_buffer::create_proof_buffer(ctx, nonce, expected_len)
    }

    /// Appends `chunk` at `offset` to the caller's proof buffer.
    ///
    /// Caller: the buffer's submitter.
    /// Errors: `InvalidProofOffset`, `InvalidProofLength`.
    pub fn append_proof(ctx: Context<WriteProofBuffer>, offset: u32, chunk: Vec<u8>) -> Result<()> {
        instructions::proof_buffer::append_proof(ctx, offset, chunk)
    }

    /// Closes the caller's proof buffer and returns its rent.
    ///
    /// Caller: the buffer's submitter.
    pub fn close_proof_buffer(ctx: Context<CloseProofBuffer>) -> Result<()> {
        instructions::proof_buffer::close_proof_buffer(ctx)
    }

    /// Resolves an x/sink record: verifies through the pinned client that
    /// the transfer key of `record.id` holds `record` at `proof_height`, then
    /// mints `record.amount` to the recipient's associated token account, or
    /// voids the record if the recipient cannot hold AKT.
    ///
    /// Caller: anyone, in a top-level instruction preceded only by
    /// ComputeBudget instructions, while minting is enabled. It pays rent for
    /// the status and the token account.
    /// Event: `TransferResolved`.
    /// Errors: `BridgeDisabled`, `CpiNotAllowed`, `PrecedingInstruction`,
    /// `InvalidRecord`, the light-client and proof errors, `InvalidClock`,
    /// `TransferLimit`, `DailyLimit`, and ICS07 errors from verification. A
    /// resolved record fails because its status exists.
    pub fn mint_transfer(
        ctx: Context<MintTransfer>,
        record: TransferRecord,
        proof_height: u64,
    ) -> Result<()> {
        instructions::mint_transfer::mint_transfer(ctx, record, proof_height)
    }

    /// Resolves an x/sink record as voided by Akash governance: verifies that
    /// the void key of `id` holds `[1, id]` at `proof_height`. Works while
    /// minting is disabled.
    ///
    /// Caller: anyone, in a top-level instruction preceded only by
    /// ComputeBudget instructions. It pays rent for the status.
    /// Event: `TransferResolved`.
    /// Errors: `CpiNotAllowed`, `PrecedingInstruction`, `InvalidRecord`, the
    /// light-client and proof errors, and ICS07 errors from verification. A
    /// resolved record fails because its status exists.
    pub fn void_transfer(ctx: Context<VoidTransfer>, id: u64, proof_height: u64) -> Result<()> {
        instructions::void_transfer::void_transfer(ctx, id, proof_height)
    }
}
