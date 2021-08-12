// Code generated by mockery 2.9.0. DO NOT EDIT.

package mocks

import (
	context "context"

	cluster "github.com/ovrclk/akash/provider/cluster"

	manifest "github.com/ovrclk/akash/provider/manifest"

	mock "github.com/stretchr/testify/mock"

	provider "github.com/ovrclk/akash/provider"

	types "github.com/ovrclk/akash/x/deployment/types"
)

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// Cluster provides a mock function with given fields:
func (_m *Client) Cluster() cluster.Client {
	ret := _m.Called()

	var r0 cluster.Client
	if rf, ok := ret.Get(0).(func() cluster.Client); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(cluster.Client)
		}
	}

	return r0
}

// Manifest provides a mock function with given fields:
func (_m *Client) Manifest() manifest.Client {
	ret := _m.Called()

	var r0 manifest.Client
	if rf, ok := ret.Get(0).(func() manifest.Client); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(manifest.Client)
		}
	}

	return r0
}

// Status provides a mock function with given fields: _a0
func (_m *Client) Status(_a0 context.Context) (*provider.Status, error) {
	ret := _m.Called(_a0)

	var r0 *provider.Status
	if rf, ok := ret.Get(0).(func(context.Context) *provider.Status); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*provider.Status)
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

// Validate provides a mock function with given fields: _a0, _a1
func (_m *Client) Validate(_a0 context.Context, _a1 types.GroupSpec) (provider.ValidateGroupSpecResult, error) {
	ret := _m.Called(_a0, _a1)

	var r0 provider.ValidateGroupSpecResult
	if rf, ok := ret.Get(0).(func(context.Context, types.GroupSpec) provider.ValidateGroupSpecResult); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Get(0).(provider.ValidateGroupSpecResult)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, types.GroupSpec) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
