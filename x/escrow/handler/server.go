package handler

import (
	"github.com/ovrclk/akash/x/escrow/keeper"
	"github.com/ovrclk/akash/x/escrow/types"
)

func NewServer(k keeper.Keeper) types.MsgServer {
	return nil
}
