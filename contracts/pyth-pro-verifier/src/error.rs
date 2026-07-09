use cosmwasm_std::StdError;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ContractError {
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
    #[error("TooManySignatures")]
    TooManySignatures,
    #[error("CannotDecodeSignature")]
    CannotDecodeSignature,
    #[error("CannotRecoverKey")]
    CannotRecoverKey,
    #[error("RouterSignatureError")]
    RouterSignatureError,
}

impl ContractError {
    pub fn std_err<T>(self) -> Result<T, StdError> {
        Err(StdError::msg(self.to_string()))
    }
}
