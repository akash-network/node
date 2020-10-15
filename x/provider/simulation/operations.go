package simulation

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp/helpers"
	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	"github.com/pkg/errors"

	appparams "github.com/ovrclk/akash/app/params"
	"github.com/ovrclk/akash/x/provider/config"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

// Simulation operation weights constants
const (
	OpWeightMsgCreate = "op_weight_msg_create"
	OpWeightMsgUpdate = "op_weight_msg_update"
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simtypes.AppParams, cdc codec.JSONMarshaler, ak govtypes.AccountKeeper,
	bk bankkeeper.Keeper, k keeper.Keeper) simulation.WeightedOperations {
	var (
		weightMsgCreate int
		weightMsgUpdate int
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCreate, &weightMsgCreate, nil, func(r *rand.Rand) {
			weightMsgCreate = appparams.DefaultWeightMsgCreateProvider
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgUpdate, &weightMsgUpdate, nil, func(r *rand.Rand) {
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
func SimulateMsgCreate(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accounts)

		// ensure the provider doesn't exist already
		_, found := k.Get(ctx, simAccount.Address)
		if found {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCreateProvider, "provider already exists"), nil, nil
		}

		cfg, readError := config.ReadConfigPath("../x/provider/testdata/provider.yaml")
		if readError != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCreateProvider, "unable to read config file"), nil, readError
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCreateProvider, "unable to generate fees"), nil, err
		}

		msg := types.NewMsgCreateProvider(simAccount.Address, cfg.Host, cfg.GetAttributes())

		txGen := simappparams.MakeEncodingConfig().TxConfig
		tx, err := helpers.GenTx(
			txGen,
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)

		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		}

		_, _, err = app.Deliver(tx)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to deliver mock tx"), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgUpdate generates a MsgUpdateProvider with random values
// nolint:funlen
func SimulateMsgUpdate(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var providers []types.Provider

		k.WithProviders(ctx, func(provider types.Provider) bool {
			providers = append(providers, provider)

			return false
		})

		if len(providers) == 0 {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateProvider, "no providers found"), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(providers))
		provider := providers[i]

		owner, convertErr := sdk.AccAddressFromBech32(provider.Owner)
		if convertErr != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateProvider, "error while converting address"),
				nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, owner)
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateProvider, "provider not found"),
				nil, errors.Errorf("provider with %s not found", provider.Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateProvider, "unable to generate fees"), nil, err
		}

		msg := &types.MsgUpdateProvider{
			Owner:   simAccount.Address.String(),
			HostURI: provider.HostURI,
		}

		txGen := simappparams.MakeEncodingConfig().TxConfig
		tx, err := helpers.GenTx(
			txGen,
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)

		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		}

		_, _, err = app.Deliver(tx)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to deliver mock tx"), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}
