//! One module per instruction group. Each defines its `Accounts` structs and
//! handlers; `lib.rs` holds only the entrypoints.
//!
//! `#[program]` expects the client modules Anchor generates for each
//! `Accounts` struct at the crate root, hence the glob re-exports. Handlers are
//! `pub(crate)` so they stay out of the crate's public interface and do not
//! collide with the instruction functions `#[program]` exports.

pub mod bridge_admin;
pub mod genesis;
pub mod mint_transfer;
pub mod pause_guardian;
pub mod proof_buffer;
mod resolution;
pub mod token_metadata;
pub mod upgrade_authority;
pub mod void_transfer;

pub use bridge_admin::*;
pub use genesis::*;
pub use mint_transfer::*;
pub use pause_guardian::*;
pub use proof_buffer::*;
pub use token_metadata::*;
pub use upgrade_authority::*;
pub use void_transfer::*;
