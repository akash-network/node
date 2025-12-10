package simulation

import (
	"math/rand"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	types "pkg.akt.dev/go/node/take/v1"
)

// Simulation operation weights constants
const (
	DefaultWeightMsgUpdateParams int = 100

	OpWeightMsgUpdateParams = "op_weight_msg_update_params" //nolint:gosec
)

// ProposalMsgs defines the module weighted proposals' contents
func ProposalMsgs() []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{
		simulation.NewWeightedProposalMsg(
			OpWeightMsgUpdateParams,
			DefaultWeightMsgUpdateParams,
			SimulateMsgUpdateParams,
		),
	}
}

func SimulateMsgUpdateParams(r *rand.Rand, _ sdk.Context, _ []simtypes.Account) sdk.Msg {
	// use the default gov module account address as authority
	var authority sdk.AccAddress = address.Module("gov")

	params := types.DefaultParams()

	params.DenomTakeRates = types.DenomTakeRates{
		{
			Denom: "uakt",
			Rate:  uint32(simtypes.RandIntBetween(r, 0, 100)), // nolint gosec
		},
		{
			Denom: "uact",
			Rate:  uint32(simtypes.RandIntBetween(r, 0, 100)), // nolint gosec
		},
	}

	return &types.MsgUpdateParams{
		Authority: authority.String(),
		Params:    params,
	}
}
