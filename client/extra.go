package client

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	mquery "github.com/ovrclk/akash/x/market/query"
	"github.com/ovrclk/akash/x/market/types"
)

// TODO: implement with search parameters

func (c *qclient) ActiveLeasesForProvider(id sdk.AccAddress) (mquery.Leases, error) {
	leases, err := c.Leases(types.LeaseFilters{})
	if err != nil {
		return nil, err
	}

	var filtered mquery.Leases
	for _, lease := range leases {
		if lease.Provider.Equals(id) && lease.State == types.LeaseActive {
			filtered = append(filtered, lease)
		}
	}
	return filtered, nil
}
