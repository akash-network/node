package simulation

import (
	"math/rand"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	types "pkg.akt.dev/go/node/deployment/v1beta4"
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

	coins := simtypes.RandSubsetCoins(r, sdk.Coins{
		sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D84", int64(simtypes.RandIntBetween(r, 500000, 50000000))),
		sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D85", int64(simtypes.RandIntBetween(r, 500000, 50000000))),
		sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D86", int64(simtypes.RandIntBetween(r, 500000, 50000000))),
		sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D87", int64(simtypes.RandIntBetween(r, 500000, 50000000))),
		sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D88", int64(simtypes.RandIntBetween(r, 500000, 50000000))),
		sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D89", int64(simtypes.RandIntBetween(r, 500000, 50000000))),
		sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D8A", int64(simtypes.RandIntBetween(r, 500000, 50000000))),
		sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D8B", int64(simtypes.RandIntBetween(r, 500000, 50000000))),
	})

	// uakt must always be present
	coins = append(coins, sdk.NewInt64Coin("uakt", int64(simtypes.RandIntBetween(r, 500000, 50000000))))

	params := types.DefaultParams()
	params.MinDeposits = coins

	return &types.MsgUpdateParams{
		Authority: authority.String(),
		Params:    params,
	}
}
