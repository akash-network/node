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
	registry *aclient.VersionRegistry
}

// NewLocalRPCClient creates a new LocalRPCClient wrapping the local client
// with a local registry instance to avoid mutating global discovery state.
func NewLocalRPCClient(lc *local.Local, registry *aclient.VersionRegistry) *LocalRPCClient {
	return &LocalRPCClient{Local: lc, registry: registry}
}

// Akash implements the RPCClient interface required by chain-sdk.
// Returns version discovery info from the local registry.
func (c *LocalRPCClient) Akash(_ context.Context) (*aclient.Akash, error) {
	return c.registry.ToAkash(), nil
}
