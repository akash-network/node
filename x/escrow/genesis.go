package escrow

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	eid "pkg.akt.dev/go/node/escrow/id/v1"
	emodule "pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"

	"pkg.akt.dev/node/x/escrow/keeper"

	types "pkg.akt.dev/go/node/escrow/v1"
)

// ValidateGenesis does validation check of the Genesis and returns an error in case of failure
func ValidateGenesis(data *types.GenesisState) error {
	amap := make(map[eid.Account]etypes.Account, len(data.Accounts))
	pmap := make(map[eid.Payment]etypes.Payment, len(data.Payments))

	for idx, account := range data.Accounts {
		if err := account.ValidateBasic(); err != nil {
			return fmt.Errorf("%w: error with account %s (idx %v)", err, account.ID, idx)
		}
		if _, found := amap[account.ID]; found {
			return fmt.Errorf("%w: duplicate account %s (idx %v)", emodule.ErrAccountExists, account.ID, idx)
		}
		amap[account.ID] = account
	}

	for idx, payment := range data.Payments {
		if err := payment.ValidateBasic(); err != nil {
			return fmt.Errorf("%w: error with payment %s (idx %v)", err, payment.ID, idx)
		}

		// make sure there's an account
		account, found := amap[payment.ID.Account()]
		if !found {
			return fmt.Errorf(
				"%w: no account %s for payment %s (idx %v)", emodule.ErrAccountNotFound, payment.ID.Account(), payment.ID, idx)
		}

		// ensure the state is in sync with payment
		switch {
		case payment.State.State == etypes.StateOpen && account.State.State != etypes.StateOpen:
			return fmt.Errorf("%w: invalid payment state for payment %s (idx %v)",
				emodule.ErrInvalidPayment, payment.ID, idx)
		case payment.State.State == etypes.StateOverdrawn && account.State.State != etypes.StateOverdrawn:
			return fmt.Errorf("%w: invalid payment statefor payment %s (idx %v)",
				emodule.ErrInvalidPayment, payment.ID, idx)
		}

		// check for duplicates
		if _, exists := pmap[payment.ID]; exists {
			return fmt.Errorf("%w, dupliate payment for %s (idx %v)", emodule.ErrPaymentExists, payment.ID, idx)
		}

		pmap[payment.ID] = payment
	}

	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data *types.GenesisState) {
	for idx := range data.Accounts {
		err := keeper.SaveAccount(ctx, data.Accounts[idx])
		if err != nil {
			panic(fmt.Sprintf("error saving account: %s", err.Error()))
		}
	}
	for idx := range data.Payments {
		err := keeper.SavePayment(ctx, data.Payments[idx])
		if err != nil {
			panic(fmt.Sprintf("error saving payment: %s", err.Error()))
		}
	}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	state := &types.GenesisState{}

	k.WithAccounts(ctx, func(obj etypes.Account) bool {
		state.Accounts = append(state.Accounts, obj)
		return false
	})

	k.WithPayments(ctx, func(obj etypes.Payment) bool {
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

// GetGenesisStateFromAppState returns x/escrow GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *types.GenesisState {
	var genesisState types.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}
