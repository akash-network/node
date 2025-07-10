package simulation

import (
	"fmt"
	"math/rand"

	cerrors "cosmossdk.io/errors"
	"pkg.akt.dev/go/sdkutil"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	types "pkg.akt.dev/go/node/provider/v1beta4"

	appparams "pkg.akt.dev/node/app/params"
	testsim "pkg.akt.dev/node/testutil/sim"
	"pkg.akt.dev/node/x/provider/config"
	"pkg.akt.dev/node/x/provider/keeper"
)

// Simulation operation weights constants
const (
	OpWeightMsgCreate = "op_weight_msg_create" // nolint gosec
	OpWeightMsgUpdate = "op_weight_msg_update" // nolint gosec
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simtypes.AppParams,
	_ codec.JSONCodec,
	ak govtypes.AccountKeeper,
	bk bankkeeper.Keeper,
	k keeper.IKeeper,
) simulation.WeightedOperations {
	var (
		weightMsgCreate int
		weightMsgUpdate int
	)

	appParams.GetOrGenerate(
		OpWeightMsgCreate, &weightMsgCreate, nil, func(_ *rand.Rand) {
			weightMsgCreate = appparams.DefaultWeightMsgCreateProvider
		},
	)

	appParams.GetOrGenerate(
		OpWeightMsgUpdate, &weightMsgUpdate, nil, func(_ *rand.Rand) {
			weightMsgUpdate = appparams.DefaultWeightMsgUpdateProvider
		},
	)

	return simulation.WeightedOperations{
		simulation.NewWeightedOperation(
			weightMsgCreate,
			SimulateMsgCreate(ak, bk, k),
		),
		simulation.NewWeightedOperation(
			weightMsgUpdate,
			SimulateMsgUpdate(ak, bk, k),
		),
	}
}

// SimulateMsgCreate generates a MsgCreateProvider with random values
// nolint:funlen
func SimulateMsgCreate(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.IKeeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accounts)

		// ensure the provider doesn't exist already
		_, found := k.Get(ctx, simAccount.Address)
		if found {
			return simtypes.NoOpMsg(types.ModuleName, (&types.MsgCreateProvider{}).Type(), "provider already exists"), nil, nil
		}

		cfg, readError := config.ReadConfigPath("../x/provider/testdata/provider.yaml")
		if readError != nil {
			return simtypes.NoOpMsg(types.ModuleName, (&types.MsgCreateProvider{}).Type(), "unable to read config file"), nil, readError
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		msg := types.NewMsgCreateProvider(simAccount.Address, cfg.Host, cfg.GetAttributes())

		return deliverMockTx(r, app, ctx, msg, account, spendable, chainID, simAccount.PrivKey)
	}
}

// SimulateMsgUpdate generates a MsgUpdateProvider with random values
// nolint:funlen
func SimulateMsgUpdate(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.IKeeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var providers []types.Provider

		k.WithProviders(ctx, func(provider types.Provider) bool {
			providers = append(providers, provider)

			return false
		})

		if len(providers) == 0 {
			return simtypes.NoOpMsg(types.ModuleName, (&types.MsgUpdateProvider{}).Type(), "no providers found"), nil, nil
		}

		// Get random deployment
		provider := providers[testsim.RandIdx(r, len(providers)-1)]

		owner, convertErr := sdk.AccAddressFromBech32(provider.Owner)
		if convertErr != nil {
			return simtypes.NoOpMsg(types.ModuleName, (&types.MsgUpdateProvider{}).Type(), "error while converting address"),
				nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, owner)
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, (&types.MsgUpdateProvider{}).Type(), "provider not found"),
				nil, fmt.Errorf("provider with %s not found", provider.Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		msg := &types.MsgUpdateProvider{
			Owner:   simAccount.Address.String(),
			HostURI: provider.HostURI,
		}

		return deliverMockTx(r, app, ctx, msg, account, spendable, chainID, simAccount.PrivKey)
	}
}

type typer interface {
	Type() string
}

func deliverMockTx(
	r *rand.Rand,
	app *baseapp.BaseApp,
	sdkctx sdk.Context,
	msg sdk.Msg,
	acc sdk.AccountI,
	spendable sdk.Coins,
	chainID string,
	privKey cryptotypes.PrivKey,
) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
	mtype, valid := msg.(typer)
	if !valid {
		return simtypes.NoOpMsg(types.ModuleName, "", "unable to determine message type. Does not implement Type interface"), nil, cerrors.ErrPanic
	}

	fees, err := simtypes.RandomFees(r, sdkctx, spendable)
	if err != nil {
		return simtypes.NoOpMsg(types.ModuleName, mtype.Type(), "unable to generate fees"), nil, err
	}

	txGen := sdkutil.MakeEncodingConfig().TxConfig
	tx, err := simtestutil.GenSignedMockTx(
		r,
		txGen,
		[]sdk.Msg{msg},
		fees,
		simtestutil.DefaultGenTxGas,
		chainID,
		[]uint64{acc.GetAccountNumber()},
		[]uint64{acc.GetSequence()},
		privKey,
	)

	if err != nil {
		return simtypes.NoOpMsg(types.ModuleName, mtype.Type(), "unable to generate mock tx"), nil, err
	}

	_, _, err = app.SimDeliver(txGen.TxEncoder(), tx)
	if err != nil {
		return simtypes.NoOpMsg(types.ModuleName, mtype.Type(), "unable to deliver mock tx"), nil, err
	}

	return simtypes.NewOperationMsg(msg, true, mtype.Type()), nil, nil
}
