// Code generated by mockery v1.0.0
package mocks

import cluster "github.com/ovrclk/akash/provider/cluster"

import mock "github.com/stretchr/testify/mock"
import types "github.com/ovrclk/akash/types"

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// Deploy provides a mock function with given fields: _a0, _a1
func (_m *Client) Deploy(_a0 types.LeaseID, _a1 *types.ManifestGroup) error {
	ret := _m.Called(_a0, _a1)

	var r0 error
	if rf, ok := ret.Get(0).(func(types.LeaseID, *types.ManifestGroup) error); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Deployments provides a mock function with given fields:
func (_m *Client) Deployments() ([]cluster.Deployment, error) {
	ret := _m.Called()

	var r0 []cluster.Deployment
	if rf, ok := ret.Get(0).(func() []cluster.Deployment); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]cluster.Deployment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LeaseStatus provides a mock function with given fields: _a0
func (_m *Client) LeaseStatus(_a0 types.LeaseID) (*types.LeaseStatusResponse, error) {
	ret := _m.Called(_a0)

	var r0 *types.LeaseStatusResponse
	if rf, ok := ret.Get(0).(func(types.LeaseID) *types.LeaseStatusResponse); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*types.LeaseStatusResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(types.LeaseID) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ServiceLogs provides a mock function with given fields: _a0, _a1
func (_m *Client) ServiceLogs(_a0 types.LeaseID, _a1 int64) ([]*cluster.ServiceLog, error) {
	ret := _m.Called(_a0, _a1)

	var r0 []*cluster.ServiceLog
	if rf, ok := ret.Get(0).(func(types.LeaseID, int64) []*cluster.ServiceLog); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*cluster.ServiceLog)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(types.LeaseID, int64) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ServiceStatus provides a mock function with given fields: _a0, _a1
func (_m *Client) ServiceStatus(_a0 types.LeaseID, _a1 string) (*types.ServiceStatusResponse, error) {
	ret := _m.Called(_a0, _a1)

	var r0 *types.ServiceStatusResponse
	if rf, ok := ret.Get(0).(func(types.LeaseID, string) *types.ServiceStatusResponse); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*types.ServiceStatusResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(types.LeaseID, string) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// TeardownLease provides a mock function with given fields: _a0
func (_m *Client) TeardownLease(_a0 types.LeaseID) error {
	ret := _m.Called(_a0)

	var r0 error
	if rf, ok := ret.Get(0).(func(types.LeaseID) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// TeardownNamespace provides a mock function with given fields: _a0
func (_m *Client) TeardownNamespace(_a0 string) error {
	ret := _m.Called(_a0)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
