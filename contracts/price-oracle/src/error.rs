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

    #[error("Price data is stale: current time {current_time}, publish time {publish_time}")]
    StalePriceData {
        current_time: i64,
        publish_time: i64,
    },

    #[error("Invalid exponent: expected -8, got {expo}")]
    InvalidExponent { expo: i32 },

    #[error("Price cannot be zero")]
    ZeroPrice {},

    #[error("Confidence interval too high: conf {conf} exceeds threshold")]
    HighConfidence { conf: String },
}
