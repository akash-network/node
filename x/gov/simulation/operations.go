package simulation

import (
	"github.com/cosmos/cosmos-sdk/codec"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"github.com/akash-network/node/x/gov/keeper"
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simtypes.AppParams, cdc codec.JSONCodec, k keeper.IKeeper) simulation.WeightedOperations {
	return nil
	// var (
	// 	weightMsgCreate int
	// 	weightMsgUpdate int
	// )
	//
	// appParams.GetOrGenerate(
	// 	cdc, OpWeightMsgCreate, &weightMsgCreate, nil, func(r *rand.Rand) {
	// 		weightMsgCreate = appparams.DefaultWeightMsgCreateProvider
	// 	},
	// )
	//
	// appParams.GetOrGenerate(
	// 	cdc, OpWeightMsgUpdate, &weightMsgUpdate, nil, func(r *rand.Rand) {
	// 		weightMsgUpdate = appparams.DefaultWeightMsgUpdateProvider
	// 	},
	// )
	//
	// return simulation.WeightedOperations{
	// 	simulation.NewWeightedOperation(
	// 		weightMsgCreate,
	// 		// SimulateMsgCreate(ak, bk, k),
	// 	),
	// 	simulation.NewWeightedOperation(
	// 		weightMsgUpdate,
	// 		SimulateMsgUpdate(ak, bk, k),
	// 	),
	// }
}
