// Code generated by mockery v1.1.2. DO NOT EDIT.

package kubernetes_mocks

import (
	mock "github.com/stretchr/testify/mock"
	v1beta1 "k8s.io/client-go/kubernetes/typed/apps/v1beta1"
)

// ControllerRevisionsGetter is an autogenerated mock type for the ControllerRevisionsGetter type
type ControllerRevisionsGetter struct {
	mock.Mock
}

// ControllerRevisions provides a mock function with given fields: namespace
func (_m *ControllerRevisionsGetter) ControllerRevisions(namespace string) v1beta1.ControllerRevisionInterface {
	ret := _m.Called(namespace)

	var r0 v1beta1.ControllerRevisionInterface
	if rf, ok := ret.Get(0).(func(string) v1beta1.ControllerRevisionInterface); ok {
		r0 = rf(namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1beta1.ControllerRevisionInterface)
		}
	}

	return r0
}
