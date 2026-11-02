//! PDA seeds, token parameters and limits.

use anchor_lang::prelude::*;

// Seeds of this program's PDAs. Each is part of an on-chain address, so
// changing one orphans the existing account.

/// `LaunchConfig`, created once by `initialize`.
pub const CONFIG_SEED: &[u8] = b"config";
/// The AKT mint, created once by `initialize`.
pub const MINT_SEED: &[u8] = b"mint";
/// Mint authority and metadata update authority. It holds no data and only
/// signs CPIs from this program.
pub const MINT_AUTHORITY_SEED: &[u8] = b"mint_authority";
/// `BridgeConfig`, created once by `configure_bridge`.
pub const BRIDGE_SEED: &[u8] = b"bridge";
/// `PauseGuardian`, created by the first `set_pause_guardian`.
pub const GUARDIAN_SEED: &[u8] = b"guardian";
/// `TransferStatus`, followed by the record ID as a little-endian u64.
pub const TRANSFER_SEED: &[u8] = b"transfer";
/// `ProofBuffer`, followed by the submitter's key and a little-endian u64
/// nonce.
pub const PROOF_SEED: &[u8] = b"proof";

// Seeds of other programs' PDAs, checked before the accounts are used.

/// ICS07 client state, under the light-client program.
pub const ICS07_CLIENT_SEED: &[u8] = b"client";
/// ICS07 consensus state, followed by the height as a little-endian u64.
pub const ICS07_CONSENSUS_STATE_SEED: &[u8] = b"consensus_state";
/// Metaplex metadata: `["metadata", metadata program, mint]`.
pub const METADATA_SEED: &[u8] = b"metadata";

/// AKT keeps Akash's six decimals, so one base unit is one uakt.
pub const AKT_DECIMALS: u8 = 6;
/// `LaunchConfig::version` written by `initialize`. Version 2 dropped the
/// genesis treasury field.
pub const LAUNCH_CONFIG_VERSION: u8 = 2;
/// Metaplex display name.
pub const TOKEN_NAME: &str = "Akash Network";
/// Metaplex symbol.
pub const TOKEN_SYMBOL: &str = "AKT";
/// Required metadata URI scheme.
pub const METADATA_URI_SCHEME: &str = "https://";
/// Longest metadata URI `set_token_metadata` accepts, in bytes.
pub const MAX_METADATA_URI_LEN: usize = 200;

/// Longest Akash chain ID, in bytes. `BridgeConfig` reserves this much space.
pub const MAX_CHAIN_ID_LEN: usize = 64;
/// Most vault addresses a mint policy may allowlist.
pub const MAX_VAULTS: usize = 16;
/// Largest ICS23 proof a proof buffer holds, in bytes.
pub const MAX_PROOF_BYTES: usize = 4096;
/// Length of a mint-cap day. Days start at multiples of this many seconds of
/// Solana clock time, that is at 00:00 UTC.
pub const SECONDS_PER_DAY: u64 = 86_400;

/// ComputeBudget program. Its instructions are the only ones a transaction may
/// place before `mint_transfer` or `void_transfer`.
pub const COMPUTE_BUDGET_PROGRAM_ID: Pubkey =
    pubkey!("ComputeBudget111111111111111111111111111111");
