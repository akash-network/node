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

    #[error("Invalid exponent: expected -8, got {expo}")]
    InvalidExponent { expo: i32 },

    #[error("Price cannot be zero")]
    ZeroPrice {},

    #[error("InvalidConfig")]
    InvalidConfig,

    #[error("InvalidAddressLength")]
    InvalidAddressLength,

    #[error("InvalidVAA")]
    InvalidVAA,

    #[error("InvalidVersion")]
    InvalidVersion,

    #[error("InvalidRouterSetIndex")]
    InvalidRouterSetIndex,

    #[error("InvalidEmitter")]
    InvalidEmitter,

    #[error("NoQuorum")]
    NoQuorum,

    #[error("WrongRouterIndexOrder")]
    WrongRouterIndexOrder,

    #[error("InvalidRouterIndex")]
    InvalidRouterIndex,

    #[error("TooManySignatures")]
    TooManySignatures,

    #[error("CannotDecodeSignature")]
    CannotDecodeSignature,

    #[error("CannotRecoverKey")]
    CannotRecoverKey,

    #[error("RouterSignatureError")]
    RouterSignatureError,
}
