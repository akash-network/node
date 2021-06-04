package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
)

const DefaultMsgType string = "/akash.deployment.v1beta1.MsgCreateDeployment"

var (
	_ authz.Authorization = &EscrowAuthorization{}
)

// NewEscrowAuthorization creates a new EscrowAuthorization object.
func NewEscrowAuthorization(credits sdk.Coin) *EscrowAuthorization {
	return &EscrowAuthorization{
		Credits: credits,
	}
}

// MsgTypeURL implements Authorization.MsgTypeURL.
func (a EscrowAuthorization) MsgTypeURL() string {
	return DefaultMsgType
}

// Accept implements Authorization.Accept.
func (a EscrowAuthorization) Accept(ctx sdk.Context, msg sdk.Msg) (authz.AcceptResponse, error) {
	return authz.AcceptResponse{Accept: false}, nil
}

// ValidateBasic implements Authorization.ValidateBasic.
func (a EscrowAuthorization) ValidateBasic() error {
	if !a.Credits.IsPositive() {
		return sdkerrors.ErrInvalidCoins.Wrapf("credits cannot be negative")
	}
	return nil
}
