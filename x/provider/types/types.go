package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/libs/common"
)

type Provider struct {
	Owner      sdk.AccAddress  `json:"owner"`
	HostURI    string          `json:"host-uri"`
	Attributes []common.KVPair `json:"attributes"`
}
