package v1beta2

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
)

func (obj *AccountID) ValidateBasic() error {
	if len(obj.Scope) == 0 {
		return errors.Wrap(ErrInvalidAccountID, "empty scope")
	}
	if len(obj.XID) == 0 {
		return errors.Wrap(ErrInvalidAccountID, "empty scope")
	}
	return nil
}

func (obj *Account) ValidateBasic() error {
	if err := obj.ID.ValidateBasic(); err != nil {
		return errors.Wrapf(ErrInvalidAccount, "invalid account: id - %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(obj.Owner); err != nil {
		return errors.Wrapf(ErrInvalidAccount, "invalid account: owner - %s", err)
	}
	if obj.State == AccountStateInvalid {
		return errors.Wrapf(ErrInvalidAccount, "invalid account: state - %s", obj.State)
	}
	if _, err := sdk.AccAddressFromBech32(obj.Depositor); err != nil {
		return errors.Wrapf(ErrInvalidAccount, "invalid account: depositor - %s", err)
	}
	return nil
}

func (obj *FractionalPayment) ValidateBasic() error {
	if err := obj.AccountID.ValidateBasic(); err != nil {
		return errors.Wrapf(ErrInvalidPayment, "invalid account id: %s", err)
	}
	if len(obj.PaymentID) == 0 {
		return errors.Wrap(ErrInvalidPayment, "empty payment id")
	}
	if obj.Rate.IsZero() {
		return errors.Wrap(ErrInvalidPayment, "payment rate zero")
	}
	if obj.State == PaymentStateInvalid {
		return errors.Wrap(ErrInvalidPayment, "invalid state")
	}
	return nil
}

// TotalBalance is the sum of Balance and Funds
func (obj *Account) TotalBalance() sdk.DecCoin {
	return obj.Balance.Add(obj.Funds)
}
