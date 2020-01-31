package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	tmkv "github.com/tendermint/tendermint/libs/kv"
)

type Provider struct {
	Owner      sdk.AccAddress  `json:"owner"`
	HostURI    string          `json:"host-uri"`
	Attributes []tmkv.Pair `json:"attributes"`
}
