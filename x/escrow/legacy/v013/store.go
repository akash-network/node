package v013

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/escrow/types"
)

// MigrateStore performs in-place store migrations from v0.12 to v0.13. The
// migration includes:
//
// - Updating escrow accounts to correctly initialize new fields
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey, cdc codec.BinaryCodec) error {
	store := ctx.KVStore(storeKey)
	migrateEscrowAccounts(store, cdc)

	return nil
}

// migrateEscrowAccounts migrates escrow accounts to correctly initialize the following new fields:
// 	- Account.Depositor as Account.Owner
// 	- Account.Funds as a Coin of zero value
func migrateEscrowAccounts(store sdk.KVStore, cdc codec.BinaryCodec) {
	accountStore := prefix.NewStore(store, types.AccountKeyPrefix())

	accountIter := accountStore.Iterator(nil, nil)
	defer accountIter.Close()

	for ; accountIter.Valid(); accountIter.Next() {
		var account types.Account
		cdc.MustUnmarshal(accountIter.Value(), &account)
		account.Depositor = account.Owner
		account.Funds = sdk.NewCoin(account.Balance.Denom, sdk.ZeroInt())
		accountStore.Set(accountIter.Key(), cdc.MustMarshal(&account))
	}
}
