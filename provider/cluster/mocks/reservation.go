// Code generated by mockery v2.5.1. DO NOT EDIT.

package mocks

import (
	akashtypes "github.com/ovrclk/akash/types"

	mock "github.com/stretchr/testify/mock"

	types "github.com/ovrclk/akash/x/market/types"
)

// Reservation is an autogenerated mock type for the Reservation type
type Reservation struct {
	mock.Mock
}

// Allocated provides a mock function with given fields:
func (_m *Reservation) Allocated() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// OrderID provides a mock function with given fields:
func (_m *Reservation) OrderID() types.OrderID {
	ret := _m.Called()

	var r0 types.OrderID
	if rf, ok := ret.Get(0).(func() types.OrderID); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(types.OrderID)
	}

	return r0
}

// Resources provides a mock function with given fields:
func (_m *Reservation) Resources() akashtypes.ResourceGroup {
	ret := _m.Called()

	var r0 akashtypes.ResourceGroup
	if rf, ok := ret.Get(0).(func() akashtypes.ResourceGroup); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(akashtypes.ResourceGroup)
		}
	}

	return r0
}
