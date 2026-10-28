//! Anchor bindings for the pinned ICS07 Tendermint light client.
//!
//! `declare_program!` reads `idls/ics07_tendermint.json` at the repository
//! root, the interface `npm run setup:light-client` checks against the patched
//! upstream build. AKT uses only `cpi::verify_membership` and the
//! `ClientState` and `ConsensusStateStore` account types. The bindings select
//! no deployment: AKT passes the pinned program account to every CPI.
//!
//! The bindings live in their own crate because `declare_program!` also emits
//! the client's `#[error_code]` enum. Anchor 0.32 builds a program's IDL from
//! the output of the program crate's unit tests and keeps whichever error
//! table it reads last, so inside the program crate the client's errors could
//! replace AKT's. A dependency's tests are never part of that build.

use anchor_lang::prelude::*;

declare_program!(ics07_tendermint);
