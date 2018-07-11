package node

import (
	amino "github.com/tendermint/go-amino"
	tmtypes "github.com/tendermint/tendermint/types"
)

var cdc = amino.NewCodec()

func init() {
	tmtypes.RegisterBlockAmino(cdc)
}
