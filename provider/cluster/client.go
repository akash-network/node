package cluster

import "github.com/ovrclk/akash/types"

type Client interface {
	Deploy(types.LeaseID, *types.ManifestGroup) error
	Teardown(types.LeaseID) error

	Deployments() ([]Deployment, error)
}

type Deployment interface {
	LeaseID() types.LeaseID
	ManifestGroup() *types.ManifestGroup
}

func NullClient() Client {
	return nullClient(0)
}

type nullClient int

func (nullClient) Deploy(_ types.LeaseID, _ *types.ManifestGroup) error {
	return nil
}

func (nullClient) Teardown(_ types.LeaseID) error {
	return nil
}

func (nullClient) Deployments() ([]Deployment, error) {
	return nil, nil
}
