package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Provider stores owner and host details
type Provider struct {
	Owner      sdk.AccAddress  `json:"owner"`
	HostURI    string          `json:"host-uri"`
	Attributes []sdk.Attribute `json:"attributes"`
}
