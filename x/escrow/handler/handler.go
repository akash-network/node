package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/escrow/keeper"
)

func NewHandler(keeper keeper.Keeper) sdk.Handler {
	return nil
}
