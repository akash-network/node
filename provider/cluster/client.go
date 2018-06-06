package cluster

import "github.com/ovrclk/akash/types"

type Client interface {
	Deploy(types.OrderID, *types.ManifestGroup) error
	Teardown(types.OrderID) error
}

func NullClient() Client {
	return nullClient(0)
}

type nullClient int

func (nullClient) Deploy(_ types.OrderID, _ *types.ManifestGroup) error {
	return nil
}

func (nullClient) Teardown(_ types.OrderID) error {
	return nil
}
