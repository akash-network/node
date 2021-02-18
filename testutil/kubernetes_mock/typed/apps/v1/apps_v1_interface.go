// Code generated by mockery v2.5.1. DO NOT EDIT.

package kubernetes_mocks

import (
	mock "github.com/stretchr/testify/mock"
	rest "k8s.io/client-go/rest"

	v1 "k8s.io/client-go/kubernetes/typed/apps/v1"
)

// AppsV1Interface is an autogenerated mock type for the AppsV1Interface type
type AppsV1Interface struct {
	mock.Mock
}

// ControllerRevisions provides a mock function with given fields: namespace
func (_m *AppsV1Interface) ControllerRevisions(namespace string) v1.ControllerRevisionInterface {
	ret := _m.Called(namespace)

	var r0 v1.ControllerRevisionInterface
	if rf, ok := ret.Get(0).(func(string) v1.ControllerRevisionInterface); ok {
		r0 = rf(namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1.ControllerRevisionInterface)
		}
	}

	return r0
}

// DaemonSets provides a mock function with given fields: namespace
func (_m *AppsV1Interface) DaemonSets(namespace string) v1.DaemonSetInterface {
	ret := _m.Called(namespace)

	var r0 v1.DaemonSetInterface
	if rf, ok := ret.Get(0).(func(string) v1.DaemonSetInterface); ok {
		r0 = rf(namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1.DaemonSetInterface)
		}
	}

	return r0
}

// Deployments provides a mock function with given fields: namespace
func (_m *AppsV1Interface) Deployments(namespace string) v1.DeploymentInterface {
	ret := _m.Called(namespace)

	var r0 v1.DeploymentInterface
	if rf, ok := ret.Get(0).(func(string) v1.DeploymentInterface); ok {
		r0 = rf(namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1.DeploymentInterface)
		}
	}

	return r0
}

// RESTClient provides a mock function with given fields:
func (_m *AppsV1Interface) RESTClient() rest.Interface {
	ret := _m.Called()

	var r0 rest.Interface
	if rf, ok := ret.Get(0).(func() rest.Interface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(rest.Interface)
		}
	}

	return r0
}

// ReplicaSets provides a mock function with given fields: namespace
func (_m *AppsV1Interface) ReplicaSets(namespace string) v1.ReplicaSetInterface {
	ret := _m.Called(namespace)

	var r0 v1.ReplicaSetInterface
	if rf, ok := ret.Get(0).(func(string) v1.ReplicaSetInterface); ok {
		r0 = rf(namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1.ReplicaSetInterface)
		}
	}

	return r0
}

// StatefulSets provides a mock function with given fields: namespace
func (_m *AppsV1Interface) StatefulSets(namespace string) v1.StatefulSetInterface {
	ret := _m.Called(namespace)

	var r0 v1.StatefulSetInterface
	if rf, ok := ret.Get(0).(func(string) v1.StatefulSetInterface); ok {
		r0 = rf(namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1.StatefulSetInterface)
		}
	}

	return r0
}
