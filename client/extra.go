package client

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// TODO: implement with search parameters

func (c *qclient) ActiveLeasesForProvider(id sdk.AccAddress) (mtypes.Leases, error) {
	params := &mtypes.QueryLeasesRequest{
		Filters: mtypes.LeaseFilters{
			Provider: id.String(),
			State:    mtypes.LeaseActive.String(),
		},
		Pagination: &sdkquery.PageRequest{
			Limit: 10000,
		},
	}

	res, err := c.Leases(context.Background(), params)
	if err != nil {
		return nil, err
	}

	return res.Leases, nil
}
