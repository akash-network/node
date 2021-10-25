package v015

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v015 "github.com/ovrclk/akash/util/legacy/v015"
	"github.com/ovrclk/akash/x/escrow/types/v1beta1"
	types "github.com/ovrclk/akash/x/escrow/types/v1beta2"
)

// MigrateStore performs in-place store migrations from v0.14 to v0.15. The
// migration includes:
//
// - Migrating Account proto from v1beta1 to v1beta2
// - Migrating Payment proto from v1beta1 to v1beta2
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey, cdc codec.BinaryCodec) error {
	store := ctx.KVStore(storeKey)
	v015.MigrateValue(store, cdc, types.AccountKeyPrefix(), migrateAccount)
	v015.MigrateValue(store, cdc, types.PaymentKeyPrefix(), migratePayment)

	return nil
}

func migrateAccount(oldValueBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObj v1beta1.Account
	cdc.MustUnmarshal(oldValueBz, &oldObj)
	return &types.Account{
		ID: types.AccountID{
			Scope: oldObj.ID.Scope,
			XID:   oldObj.ID.XID,
		},
		Owner:       oldObj.Owner,
		State:       types.Account_State(oldObj.State),
		Balance:     sdk.NewDecCoinFromCoin(oldObj.Balance),
		Transferred: sdk.NewDecCoinFromCoin(oldObj.Transferred),
		SettledAt:   oldObj.SettledAt,
		// Correctly initialize the new fields
		// - Account.Depositor as Account.Owner
		// - Account.Funds as a DecCoin of zero value
		Depositor: oldObj.Owner,
		Funds:     sdk.NewDecCoin(oldObj.Balance.Denom, sdk.ZeroInt()),
	}
}

func migratePayment(oldValueBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObject v1beta1.Payment
	cdc.MustUnmarshal(oldValueBz, &oldObject)

	return &types.FractionalPayment{
		AccountID: types.AccountID{
			Scope: oldObject.AccountID.Scope,
			XID:   oldObject.AccountID.XID,
		},
		PaymentID: oldObject.PaymentID,
		Owner:     oldObject.Owner,
		State:     types.FractionalPayment_State(oldObject.State),
		Rate:      sdk.NewDecCoinFromCoin(oldObject.Rate),
		Balance:   sdk.NewDecCoinFromCoin(oldObject.Balance),
		Withdrawn: oldObject.Withdrawn,
	}
}
