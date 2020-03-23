package keeper

import (
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/ovrclk/akash/x/market/types"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"
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

// SetupTestInput will setup test inputs and return context and keeper
func SetupTestInput() (sdk.Context, auth.AccountKeeper, params.Keeper, bank.BaseKeeper, Keeper) {
	db := dbm.NewMemDB()

	keyAcc := sdk.NewKVStoreKey(auth.StoreKey)
	keyParams := sdk.NewKVStoreKey(params.StoreKey)
	keyBank := sdk.NewKVStoreKey(bank.ModuleName)
	keyMarket := sdk.NewKVStoreKey(types.StoreKey)
	tkeyParams := sdk.NewTransientStoreKey(params.TStoreKey)

	ms := store.NewCommitMultiStore(db)
	ms.MountStoreWithDB(keyAcc, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyParams, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyBank, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyMarket, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(tkeyParams, sdk.StoreTypeTransient, db)

	ms.LoadLatestVersion()

	ctx := sdk.NewContext(ms, abci.Header{Time: time.Unix(0, 0)}, false, log.NewNopLogger())
	cdc := createTestCodec()

	blacklistedAddrs := make(map[string]bool)

	paramsKeeper := params.NewKeeper(params.ModuleCdc, keyParams, tkeyParams)
	authKeeper := auth.NewAccountKeeper(cdc, keyAcc, paramsKeeper.Subspace(auth.DefaultParamspace), auth.ProtoBaseAccount)
	bankKeeper := bank.NewBaseKeeper(authKeeper, paramsKeeper.Subspace(bank.DefaultParamspace), blacklistedAddrs)
	bankKeeper.SetSendEnabled(ctx, true)

	marketKeeper := NewKeeper(cdc, keyMarket)

	return ctx, authKeeper, paramsKeeper, bankKeeper, marketKeeper

}

var (
	ownerPub     = ed25519.GenPrivKey().PubKey()
	ownerAddr    = sdk.AccAddress(ownerPub.Address())
	providerPub  = ed25519.GenPrivKey().PubKey()
	providerAddr = sdk.AccAddress(providerPub.Address())
	addr2Pub     = ed25519.GenPrivKey().PubKey()
	addr2        = sdk.AccAddress(addr2Pub.Address())
)
