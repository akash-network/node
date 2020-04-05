package query

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	providersPath = "providers"
	providerPath  = "provider"
)

// getProvidersPath returns providers path for queries
func getProvidersPath() string {
	return providersPath
}

func getProviderPath(id sdk.AccAddress) string {
	return fmt.Sprintf("%s/%s", providerPath, id)
}
