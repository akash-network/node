package query

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	providersPath = "providers"
	providerPath  = "provider"
)

// ProvidersPath returns providers path for queries
func ProvidersPath() string {
	return providersPath
}

func ProviderPath(id sdk.AccAddress) string {
	return fmt.Sprintf("%s/%s", providerPath, id)
}
