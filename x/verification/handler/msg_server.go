package handler

import (
	vtypes "pkg.akt.dev/go/node/verification/v1"

	"pkg.akt.dev/node/v2/x/verification/keeper"
)

type msgServer struct {
	vtypes.UnimplementedMsgServer
	keeper keeper.Keeper
}

func NewMsgServerImpl(k keeper.Keeper) vtypes.MsgServer {
	return &msgServer{keeper: k}
}
