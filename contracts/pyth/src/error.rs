use cosmwasm_std::StdError;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ContractError {
    #[error("{0}")]
    Std(#[from] StdError),

    #[error("Unauthorized")]
    Unauthorized {},

    #[error("Invalid price data: {reason}")]
    InvalidPriceData { reason: String },

    #[error("Insufficient funds: required {required}, sent {sent}")]
    InsufficientFunds { required: String, sent: String },

    #[error("Price data is stale")]
    StalePriceData {},

    #[error("Invalid exponent: expected -8, got {expo}")]
    InvalidExponent { expo: i32 },

    #[error("Price cannot be zero")]
    ZeroPrice {},

    #[error("Confidence interval too high: conf {conf} exceeds max allowed {max_allowed}")]
    HighConfidence { conf: String, max_allowed: String },

    #[error("Invalid data source: emitter_chain {emitter_chain}, emitter_address {emitter_address}")]
    InvalidDataSource {
        emitter_chain: u16,
        emitter_address: String,
    },

    #[error("VAA verification failed: {reason}")]
    VAAVerificationFailed { reason: String },
}
