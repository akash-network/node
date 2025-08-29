package handler

import (
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	types "pkg.akt.dev/go/node/escrow/v1"

	"pkg.akt.dev/node/x/escrow/keeper"
)

// NewHandler returns a handler for "deployment" type messages
func NewHandler(keeper keeper.Keeper, authzKeeper AuthzKeeper, bkeeper BankKeeper) baseapp.MsgServiceHandler {
	ms := NewServer(keeper, authzKeeper, bkeeper)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgAccountDeposit:
			res, err := ms.AccountDeposit(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)
		default:
			return nil, sdkerrors.ErrUnknownRequest
		}
	}
}
