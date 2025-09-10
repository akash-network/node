package handler

import (
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/market/v1beta5"
)

// NewHandler returns a handler for "market" type messages
func NewHandler(keepers Keepers) baseapp.MsgServiceHandler {
	ms := NewServer(keepers)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgCreateBid:
			res, err := ms.CreateBid(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)
		case *types.MsgCloseBid:
			res, err := ms.CloseBid(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)
		case *types.MsgWithdrawLease:
			res, err := ms.WithdrawLease(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)
		case *types.MsgCreateLease:
			res, err := ms.CreateLease(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)
		case *types.MsgCloseLease:
			res, err := ms.CloseLease(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)
		default:
			return nil, sdkerrors.ErrUnknownRequest
		}
	}
}
