package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/ovrclk/akash/x/provider/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

func createTestCodec() *codec.Codec {
	cdc := codec.New()
	auth.RegisterCodec(cdc)
	bank.RegisterCodec(cdc)
	sdk.RegisterCodec(cdc)
	types.RegisterCodec(cdc)
	codec.RegisterCrypto(cdc)

	return cdc
}

// // SetupTestInput will setup test inputs and return context and keeper
// func SetupTestInput() (sdk.Context, auth.AccountKeeper, params.Keeper, bank.BaseKeeper, Keeper) {
// 	db := dbm.NewMemDB()

// 	keyAcc := sdk.NewKVStoreKey(auth.StoreKey)
// 	keyParams := sdk.NewKVStoreKey(params.StoreKey)
// 	keyBank := sdk.NewKVStoreKey(bank.ModuleName)
// 	keyProvider := sdk.NewKVStoreKey(types.StoreKey)
// 	tkeyParams := sdk.NewTransientStoreKey(params.TStoreKey)

// 	ms := store.NewCommitMultiStore(db)
// 	ms.MountStoreWithDB(keyAcc, sdk.StoreTypeIAVL, db)
// 	ms.MountStoreWithDB(keyParams, sdk.StoreTypeIAVL, db)
// 	ms.MountStoreWithDB(keyBank, sdk.StoreTypeIAVL, db)
// 	ms.MountStoreWithDB(keyProvider, sdk.StoreTypeIAVL, db)
// 	ms.MountStoreWithDB(tkeyParams, sdk.StoreTypeTransient, db)

// 	ms.LoadLatestVersion()

// 	ctx := sdk.NewContext(ms, abci.Header{Time: time.Unix(0, 0)}, false, log.NewNopLogger())
// 	cdc := createTestCodec()
// 	appCodec, _ := app.MakeCodecs()

// 	blacklistedAddrs := make(map[string]bool)
// 	macPerms := make(map[string][]string)

// 	paramsKeeper := params.NewKeeper(appCodec, keyParams, tkeyParams)
// 	authKeeper := auth.NewAccountKeeper(appCodec, keyAcc, paramsKeeper.Subspace(auth.DefaultParamspace),
// 		auth.ProtoBaseAccount, macPerms)
// 	bankKeeper := bank.NewBaseKeeper(appCodec, keyBank, authKeeper,
// 		paramsKeeper.Subspace(bank.DefaultParamspace), blacklistedAddrs)
// 	bankKeeper.SetSendEnabled(ctx, true)

// 	providerKeeper := NewKeeper(cdc, keyProvider)

// 	return ctx, authKeeper, paramsKeeper, bankKeeper, providerKeeper

// }

var (
	ownerPub  = ed25519.GenPrivKey().PubKey()
	ownerAddr = sdk.AccAddress(ownerPub.Address())
	addr2Pub  = ed25519.GenPrivKey().PubKey()
	addr2     = sdk.AccAddress(addr2Pub.Address())
)
