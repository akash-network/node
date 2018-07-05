package cluster

import (
	"bufio"
	"context"
	"io"

	"github.com/ovrclk/akash/types"
)

type Client interface {
	Deploy(types.LeaseID, *types.ManifestGroup) error
	TeardownLease(types.LeaseID) error
	TeardownNamespace(string) error

	Deployments() ([]Deployment, error)
	LeaseStatus(types.LeaseID) (*types.LeaseStatusResponse, error)
	ServiceStatus(types.LeaseID, string) (*types.ServiceStatusResponse, error)
	ServiceLogs(context.Context, types.LeaseID, int64, bool) ([]*ServiceLog, error)
}

type Deployment interface {
	LeaseID() types.LeaseID
	ManifestGroup() *types.ManifestGroup
}

type ServiceLog struct {
	Name    string
	Stream  io.ReadCloser
	Scanner *bufio.Scanner
}

type nullClient int

func NewServiceLog(name string, stream io.ReadCloser) *ServiceLog {
	return &ServiceLog{
		Name:    name,
		Stream:  stream,
		Scanner: bufio.NewScanner(stream),
	}
}

func NullClient() Client {
	return nullClient(0)
}

func (nullClient) Deploy(_ types.LeaseID, _ *types.ManifestGroup) error {
	return nil
}

func (nullClient) LeaseStatus(_ types.LeaseID) (*types.LeaseStatusResponse, error) {
	return nil, nil
}

func (nullClient) ServiceStatus(_ types.LeaseID, _ string) (*types.ServiceStatusResponse, error) {
	return nil, nil
}

func (nullClient) ServiceLogs(_ context.Context, _ types.LeaseID, _ int64, _ bool) ([]*ServiceLog, error) {
	return nil, nil
}

func (nullClient) TeardownLease(_ types.LeaseID) error {
	return nil
}

func (nullClient) TeardownNamespace(_ string) error {
	return nil
}

func (nullClient) Deployments() ([]Deployment, error) {
	return nil, nil
}
