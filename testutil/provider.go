package testutil

import (
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

func Provider(tenant base.Bytes, nonce uint64) *types.Provider {

	address := state.ProviderAddress(tenant, nonce)

	providerattribute := &types.ProviderAttribute{
		Name:  "region",
		Value: "us-west",
	}

	attributes := []types.ProviderAttribute{*providerattribute}

	provider := &types.Provider{
		Address:    address,
		Attributes: attributes,
		Owner:      tenant,
	}

	return provider
}
