use anchor_lang::prelude::*;

/// AKT errors retain their 7000 base and existing discriminants.
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
    #[msg(
        "Proof must declare 1 to 16384 bytes; chunks must be 1 to 500 bytes and fit that length"
    )]
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
    #[msg("Record needs a 1-to-255-byte owner, positive sequence and amount, and 0 < height <= MaxInt64 with height < proof height")]
    InvalidRecord,
    #[msg("Solana clock is negative or earlier than the recorded mint day")]
    InvalidClock,
    #[msg("Amount exceeds the per-transfer mint limit; retry after a policy change")]
    TransferLimit,
    #[msg("Amount would exceed the daily mint cap; retry on a later UTC day")]
    DailyLimit,
    #[msg("Proof buffer nonce must be the full SHA-256 of the proven key")]
    InvalidProofNonce,
    #[msg("Canonical settlement status must be 1 through 5")]
    InvalidSettlementStatus,
    #[msg("Settlement must contain a unique, bounded permanent void authorization")]
    InvalidVoidAuthorization,
    #[msg("Full IAVL and multistore membership proof does not verify")]
    InvalidMembershipProof,
    #[msg("Grow the proof buffer before appending beyond its allocation")]
    ProofBufferTooSmall,
}
