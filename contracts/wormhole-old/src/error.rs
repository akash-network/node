use cosmwasm_std::StdError;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ContractError {
    #[error("{0}")]
    Std(#[from] StdError),

    #[error("Unauthorized")]
    Unauthorized {},

    #[error("Invalid VAA version")]
    InvalidVersion,

    #[error("Invalid VAA")]
    InvalidVAA,

    #[error("VAA has already been executed")]
    VaaAlreadyExecuted,

    #[error("Invalid guardian set index")]
    InvalidGuardianSetIndex,

    #[error("Guardian set has expired")]
    GuardianSetExpired,

    #[error("No quorum")]
    NoQuorum,

    #[error("Wrong guardian index order")]
    WrongGuardianIndexOrder,

    #[error("Cannot decode signature")]
    CannotDecodeSignature,

    #[error("Cannot recover key")]
    CannotRecoverKey,

    #[error("Too many signatures")]
    TooManySignatures,

    #[error("Guardian signature verification failed")]
    GuardianSignatureError,

    #[error("Invalid VAA action")]
    InvalidVAAAction,

    #[error("Guardian set index increase error")]
    GuardianSetIndexIncreaseError,

    #[error("Fee too low")]
    FeeTooLow,
}

impl ContractError {
    pub fn std_err<T>(self) -> Result<T, StdError> {
        Err(StdError::msg(self.to_string()))
    }
}
