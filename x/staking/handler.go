package provider

// import (
// 	sdk "github.com/cosmos/cosmos-sdk/types"
// 	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
// 	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
// 	types "pkg.akt.dev/go/node/staking/v1beta3"
//
// 	"pkg.akt.dev/node/x/staking/keeper"
// )
//
// // NewUpdateParamsProposalHandler
// func NewUpdateParamsProposalHandler(k keeper.Keeper) govtypes.Handler {
// 	return func(ctx sdk.Context, content govtypes.Content) error {
// 		switch c := content.(type) {
// 		case *types.MsgUpdateParams:
// 			return k.ClientUpdateProposal(ctx, c)
//
// 		default:
// 			return sdkerrors.ErrUnknownRequest.Wrapf("unrecognized ibc proposal content type: %T", c)
// 		}
// 	}
// }
//
