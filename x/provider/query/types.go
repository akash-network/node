package query

import "github.com/ovrclk/akash/x/provider/types"

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
