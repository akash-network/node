package v1beta1

import (
	"context"

	sdktypes "github.com/cosmos/cosmos-sdk/types"

	mtypes "github.com/ovrclk/akash/x/market/types/v1beta1"
)

type HostnameServiceClient interface {
	ReserveHostnames(ctx context.Context, hostnames []string, leaseID mtypes.LeaseID) ([]string, error)
	ReleaseHostnames(leaseID mtypes.LeaseID) error
	CanReserveHostnames(hostnames []string, ownerAddr sdktypes.Address) error
	PrepareHostnamesForTransfer(ctx context.Context, hostnames []string, leaseID mtypes.LeaseID) error
}
