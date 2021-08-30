package cluster

import (
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"context"
)

type HostnameServiceClient interface {
	ReserveHostnames(ctx context.Context, hostnames []string, leaseID mtypes.LeaseID) ([]string, error)
	ReleaseHostnames(leaseID mtypes.LeaseID) error
	CanReserveHostnames(hostnames []string, ownerAddr sdktypes.Address) error
	PrepareHostnamesForTransfer(ctx context.Context, hostnames []string, leaseID mtypes.LeaseID) error
}

