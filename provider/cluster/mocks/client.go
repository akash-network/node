// Code generated by mockery v2.5.1. DO NOT EDIT.

package mocks

import (
	context "context"

	cluster "github.com/ovrclk/akash/provider/cluster/types"

	io "io"

	manifest "github.com/ovrclk/akash/manifest"

	mock "github.com/stretchr/testify/mock"

	remotecommand "k8s.io/client-go/tools/remotecommand"

	v1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"

	v1beta2 "github.com/ovrclk/akash/x/market/types/v1beta2"
)

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// AllHostnames provides a mock function with given fields: _a0
func (_m *Client) AllHostnames(_a0 context.Context) ([]cluster.ActiveHostname, error) {
	ret := _m.Called(_a0)

	var r0 []cluster.ActiveHostname
	if rf, ok := ret.Get(0).(func(context.Context) []cluster.ActiveHostname); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]cluster.ActiveHostname)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ConnectHostnameToDeployment provides a mock function with given fields: ctx, directive
func (_m *Client) ConnectHostnameToDeployment(ctx context.Context, directive cluster.ConnectHostnameToDeploymentDirective) error {
	ret := _m.Called(ctx, directive)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, cluster.ConnectHostnameToDeploymentDirective) error); ok {
		r0 = rf(ctx, directive)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DeclareHostname provides a mock function with given fields: ctx, lID, host, serviceName, externalPort
func (_m *Client) DeclareHostname(ctx context.Context, lID v1beta2.LeaseID, host string, serviceName string, externalPort uint32) error {
	ret := _m.Called(ctx, lID, host, serviceName, externalPort)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID, string, string, uint32) error); ok {
		r0 = rf(ctx, lID, host, serviceName, externalPort)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Deploy provides a mock function with given fields: ctx, lID, mgroup
func (_m *Client) Deploy(ctx context.Context, lID v1beta2.LeaseID, mgroup *manifest.Group) error {
	ret := _m.Called(ctx, lID, mgroup)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID, *manifest.Group) error); ok {
		r0 = rf(ctx, lID, mgroup)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Deployments provides a mock function with given fields: _a0
func (_m *Client) Deployments(_a0 context.Context) ([]cluster.Deployment, error) {
	ret := _m.Called(_a0)

	var r0 []cluster.Deployment
	if rf, ok := ret.Get(0).(func(context.Context) []cluster.Deployment); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]cluster.Deployment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Exec provides a mock function with given fields: ctx, lID, service, podIndex, cmd, stdin, stdout, stderr, tty, tsq
func (_m *Client) Exec(ctx context.Context, lID v1beta2.LeaseID, service string, podIndex uint, cmd []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, tty bool, tsq remotecommand.TerminalSizeQueue) (cluster.ExecResult, error) {
	ret := _m.Called(ctx, lID, service, podIndex, cmd, stdin, stdout, stderr, tty, tsq)

	var r0 cluster.ExecResult
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID, string, uint, []string, io.Reader, io.Writer, io.Writer, bool, remotecommand.TerminalSizeQueue) cluster.ExecResult); ok {
		r0 = rf(ctx, lID, service, podIndex, cmd, stdin, stdout, stderr, tty, tsq)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(cluster.ExecResult)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, v1beta2.LeaseID, string, uint, []string, io.Reader, io.Writer, io.Writer, bool, remotecommand.TerminalSizeQueue) error); ok {
		r1 = rf(ctx, lID, service, podIndex, cmd, stdin, stdout, stderr, tty, tsq)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetHostnameDeploymentConnections provides a mock function with given fields: ctx
func (_m *Client) GetHostnameDeploymentConnections(ctx context.Context) ([]cluster.LeaseIDHostnameConnection, error) {
	ret := _m.Called(ctx)

	var r0 []cluster.LeaseIDHostnameConnection
	if rf, ok := ret.Get(0).(func(context.Context) []cluster.LeaseIDHostnameConnection); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]cluster.LeaseIDHostnameConnection)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetManifestGroup provides a mock function with given fields: _a0, _a1
func (_m *Client) GetManifestGroup(_a0 context.Context, _a1 v1beta2.LeaseID) (bool, v1.ManifestGroup, error) {
	ret := _m.Called(_a0, _a1)

	var r0 bool
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID) bool); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 v1.ManifestGroup
	if rf, ok := ret.Get(1).(func(context.Context, v1beta2.LeaseID) v1.ManifestGroup); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Get(1).(v1.ManifestGroup)
	}

	var r2 error
	if rf, ok := ret.Get(2).(func(context.Context, v1beta2.LeaseID) error); ok {
		r2 = rf(_a0, _a1)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// Inventory provides a mock function with given fields: _a0
func (_m *Client) Inventory(_a0 context.Context) (cluster.Inventory, error) {
	ret := _m.Called(_a0)

	var r0 cluster.Inventory
	if rf, ok := ret.Get(0).(func(context.Context) cluster.Inventory); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(cluster.Inventory)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LeaseEvents provides a mock function with given fields: _a0, _a1, _a2, _a3
func (_m *Client) LeaseEvents(_a0 context.Context, _a1 v1beta2.LeaseID, _a2 string, _a3 bool) (cluster.EventsWatcher, error) {
	ret := _m.Called(_a0, _a1, _a2, _a3)

	var r0 cluster.EventsWatcher
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID, string, bool) cluster.EventsWatcher); ok {
		r0 = rf(_a0, _a1, _a2, _a3)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(cluster.EventsWatcher)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, v1beta2.LeaseID, string, bool) error); ok {
		r1 = rf(_a0, _a1, _a2, _a3)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LeaseLogs provides a mock function with given fields: _a0, _a1, _a2, _a3, _a4
func (_m *Client) LeaseLogs(_a0 context.Context, _a1 v1beta2.LeaseID, _a2 string, _a3 bool, _a4 *int64) ([]*cluster.ServiceLog, error) {
	ret := _m.Called(_a0, _a1, _a2, _a3, _a4)

	var r0 []*cluster.ServiceLog
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID, string, bool, *int64) []*cluster.ServiceLog); ok {
		r0 = rf(_a0, _a1, _a2, _a3, _a4)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*cluster.ServiceLog)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, v1beta2.LeaseID, string, bool, *int64) error); ok {
		r1 = rf(_a0, _a1, _a2, _a3, _a4)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LeaseStatus provides a mock function with given fields: _a0, _a1
func (_m *Client) LeaseStatus(_a0 context.Context, _a1 v1beta2.LeaseID) (*cluster.LeaseStatus, error) {
	ret := _m.Called(_a0, _a1)

	var r0 *cluster.LeaseStatus
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID) *cluster.LeaseStatus); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*cluster.LeaseStatus)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, v1beta2.LeaseID) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ObserveHostnameState provides a mock function with given fields: ctx
func (_m *Client) ObserveHostnameState(ctx context.Context) (<-chan cluster.HostnameResourceEvent, error) {
	ret := _m.Called(ctx)

	var r0 <-chan cluster.HostnameResourceEvent
	if rf, ok := ret.Get(0).(func(context.Context) <-chan cluster.HostnameResourceEvent); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan cluster.HostnameResourceEvent)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PurgeDeclaredHostname provides a mock function with given fields: ctx, lID, hostname
func (_m *Client) PurgeDeclaredHostname(ctx context.Context, lID v1beta2.LeaseID, hostname string) error {
	ret := _m.Called(ctx, lID, hostname)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID, string) error); ok {
		r0 = rf(ctx, lID, hostname)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// PurgeDeclaredHostnames provides a mock function with given fields: ctx, lID
func (_m *Client) PurgeDeclaredHostnames(ctx context.Context, lID v1beta2.LeaseID) error {
	ret := _m.Called(ctx, lID)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID) error); ok {
		r0 = rf(ctx, lID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RemoveHostnameFromDeployment provides a mock function with given fields: ctx, hostname, leaseID, allowMissing
func (_m *Client) RemoveHostnameFromDeployment(ctx context.Context, hostname string, leaseID v1beta2.LeaseID, allowMissing bool) error {
	ret := _m.Called(ctx, hostname, leaseID, allowMissing)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, v1beta2.LeaseID, bool) error); ok {
		r0 = rf(ctx, hostname, leaseID, allowMissing)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ServiceStatus provides a mock function with given fields: _a0, _a1, _a2
func (_m *Client) ServiceStatus(_a0 context.Context, _a1 v1beta2.LeaseID, _a2 string) (*cluster.ServiceStatus, error) {
	ret := _m.Called(_a0, _a1, _a2)

	var r0 *cluster.ServiceStatus
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID, string) *cluster.ServiceStatus); ok {
		r0 = rf(_a0, _a1, _a2)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*cluster.ServiceStatus)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, v1beta2.LeaseID, string) error); ok {
		r1 = rf(_a0, _a1, _a2)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// TeardownLease provides a mock function with given fields: _a0, _a1
func (_m *Client) TeardownLease(_a0 context.Context, _a1 v1beta2.LeaseID) error {
	ret := _m.Called(_a0, _a1)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, v1beta2.LeaseID) error); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
