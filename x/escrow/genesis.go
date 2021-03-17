package escrow

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/ovrclk/akash/x/escrow/keeper"

	"github.com/ovrclk/akash/x/escrow/types"
)

// ValidateGenesis does validation check of the Genesis and returns error incase of failure
func ValidateGenesis(data *types.GenesisState) error {

	amap := make(map[types.AccountID]types.Account, len(data.Accounts))
	pmap := make(map[types.AccountID][]types.FractionalPayment, len(data.Payments))

	for idx, account := range data.Accounts {
		if err := account.ValidateBasic(); err != nil {
			return errors.Wrapf(err, "error with account %s (idx %v):", account.ID, idx)
		}
		if _, found := amap[account.ID]; found {
			return errors.Wrapf(types.ErrAccountExists, "duplicate account %s (idx %v)", account.ID, idx)
		}
		amap[account.ID] = account
	}

	for idx, payment := range data.Payments {
		if err := payment.ValidateBasic(); err != nil {
			return errors.Wrapf(err, "error with payment %s %s (idx %v):", payment.AccountID, payment.PaymentID, idx)
		}

		// make sure there's an account
		account, found := amap[payment.AccountID]
		if !found {
			return errors.Wrapf(
				types.ErrAccountNotFound, "no account for payment %s %s (idx %v)", payment.AccountID, payment.PaymentID, idx)
		}

		// ensure state is in sync with payment
		switch {
		case payment.State == types.PaymentOpen && account.State != types.AccountOpen:
			return errors.Wrapf(types.ErrInvalidPayment, "invalid payment statefor payment %s %s (idx %v)",
				payment.AccountID, payment.PaymentID, idx)
		case payment.State == types.PaymentOverdrawn && account.State != types.AccountOverdrawn:
			return errors.Wrapf(types.ErrInvalidPayment, "invalid payment statefor payment %s %s (idx %v)",
				payment.AccountID, payment.PaymentID, idx)
		}

		// check for duplicates
		for _, p2 := range pmap[payment.AccountID] {
			if p2.PaymentID == payment.PaymentID {
				return errors.Wrapf(
					types.ErrPaymentExists, "dupliate payment for %s %s (idx %v)", payment.AccountID, payment.PaymentID, idx)
			}
		}

		pmap[payment.AccountID] = append(pmap[payment.AccountID], payment)
	}

	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data *types.GenesisState) []abci.ValidatorUpdate {
	for idx := range data.Accounts {
		keeper.SaveAccount(ctx, data.Accounts[idx])
	}
	for idx := range data.Payments {
		keeper.SavePayment(ctx, data.Payments[idx])
	}
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	state := &types.GenesisState{}
	k.WithAccounts(ctx, func(obj types.Account) bool {
		state.Accounts = append(state.Accounts, obj)
		return false
	})
	k.WithPayments(ctx, func(obj types.FractionalPayment) bool {
		state.Payments = append(state.Payments, obj)
		return false
	})
	return state
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{}
}
