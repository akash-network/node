package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/provider/types"
)

type (
	// Provider type
	Provider types.Provider
	// Providers - Slice of Provider Struct
	Providers []Provider
)

func (obj Provider) String() string {
	return "TODO see deployment/query/types.go"
}

func (obj Providers) String() string {
	return "TODO see deployment/query/types.go"
}

func (p *Provider) Address() sdk.AccAddress {
	return p.Owner
}
