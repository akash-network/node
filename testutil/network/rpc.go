package network

import (
	"context"

	"github.com/cometbft/cometbft/rpc/client/local"

	aclient "pkg.akt.dev/go/node/client"
)

// LocalRPCClient wraps local.Local and implements the RPCClient interface
// required by chain-sdk's aclient.DiscoverClient.
// The local.Local client only implements client.CometRPC but not the Akash() method
// needed by DiscoverClient to detect the API version.
type LocalRPCClient struct {
	*local.Local
}

// NewLocalRPCClient creates a new LocalRPCClient wrapping the local client
func NewLocalRPCClient(lc *local.Local) *LocalRPCClient {
	return &LocalRPCClient{Local: lc}
}

// Akash implements the RPCClient interface required by chain-sdk.
// Returns client info with the current API version.
func (c *LocalRPCClient) Akash(_ context.Context) (*aclient.Akash, error) {
	return &aclient.Akash{
		ClientInfo: aclient.ClientInfo{
			ApiVersion: "v1beta3",
		},
	}, nil
}
