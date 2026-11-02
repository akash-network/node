use anchor_lang::prelude::*;

/// AKT program errors, grouped by area.
///
/// Codes start at 7000: Anchor adds 6000 to the first variant's discriminant
/// of 1000. `mint_transfer` and `void_transfer` call the ICS07 client, whose
/// own errors start at 6000 and fail the AKT instruction with the same custom
/// code, so the two ranges must not overlap. Anchor 0.32's
/// `#[error_code(offset = ...)]` cannot be used: its IDL keeps the 6000 base.
#[error_code]
pub enum AktError {
    // Authorities.
    #[msg("ProgramData account does not belong to this program")]
    InvalidProgramData = 1000,
    #[msg("Only this program's upgrade authority may run genesis")]
    UnauthorizedInitializer,
    #[msg("Signer is not this program's upgrade authority")]
    InvalidAuthority,
    #[msg("Signer is not the configured pause guardian")]
    UnauthorizedGuardian,

    // Token metadata.
    #[msg("Metadata URI must start with https:// and be 9 to 200 printable ASCII bytes")]
    InvalidMetadataUri,
    #[msg("Existing metadata is not for the AKT mint or not controlled by the mint-authority PDA")]
    InvalidMetadata,

    // Bridge configuration.
    #[msg("Chain ID must be 1 to 64 bytes")]
    InvalidChainId,
    #[msg("Mint policy needs 0 < per-transfer max <= daily cap and at most 16 nonzero vaults")]
    InvalidPolicy,

    // Light client.
    #[msg("Light-client account has the wrong owner or address, or invalid parameters")]
    InvalidLightClient,
    #[msg("Light-client chain ID differs from the expected chain ID")]
    WrongChain,
    #[msg("Light client is frozen")]
    ClientFrozen,
    #[msg("Light client's latest consensus state is older than its trusting period")]
    ClientExpired,
    #[msg("Latest consensus timestamp is zero or later than the clock plus the allowed drift")]
    InvalidConsensusTimestamp,
    #[msg("Proof height must be between 1 and the client's latest height")]
    InvalidProofHeight,

    // Proof buffers.
    #[msg("Proof must declare 1 to 4096 bytes; chunks must be nonempty and fit that length")]
    InvalidProofLength,
    #[msg("Proof chunk offset must equal the number of bytes already written")]
    InvalidProofOffset,
    #[msg("Proof buffer holds fewer bytes than it declared")]
    IncompleteProof,

    // Transfer resolution.
    #[msg("Bridge minting is disabled")]
    BridgeDisabled,
    #[msg("Mint and void must be top-level instructions, not CPI")]
    CpiNotAllowed,
    #[msg("Only compute budget instructions may precede mint or void")]
    PrecedingInstruction,
    #[msg("Record needs version 1, nonzero ID and amount, and 0 < height <= proof height")]
    InvalidRecord,
    #[msg("Solana clock is negative or earlier than the recorded mint day")]
    InvalidClock,
    #[msg("Amount exceeds the per-transfer mint limit; retry after a policy change")]
    TransferLimit,
    #[msg("Amount would exceed the daily mint cap; retry on a later UTC day")]
    DailyLimit,
}
