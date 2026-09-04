use cosmwasm_std::StdError;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ContractError {
    #[error("{0}")]
    Std(#[from] StdError),

    #[error("Unauthorized")]
    Unauthorized {},

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

    #[error("InvalidGovernanceTarget")]
    InvalidGovernanceTarget,

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

    #[error("InvalidRouterSetUpdate")]
    InvalidRouterSetUpdate,

    #[error("InvalidVAAAction")]
    InvalidVAAAction,

    #[error("RouterSetIndexIncreaseError")]
    RouterSetIndexIncreaseError,

    #[error("RouterSetAlreadyExists")]
    RouterSetAlreadyExists,
}
