// Code generated by mockery v1.1.2. DO NOT EDIT.

package kubernetes_mocks

import (
	mock "github.com/stretchr/testify/mock"
	v1alpha1 "k8s.io/client-go/kubernetes/typed/settings/v1alpha1"
)

// PodPresetsGetter is an autogenerated mock type for the PodPresetsGetter type
type PodPresetsGetter struct {
	mock.Mock
}

// PodPresets provides a mock function with given fields: namespace
func (_m *PodPresetsGetter) PodPresets(namespace string) v1alpha1.PodPresetInterface {
	ret := _m.Called(namespace)

	var r0 v1alpha1.PodPresetInterface
	if rf, ok := ret.Get(0).(func(string) v1alpha1.PodPresetInterface); ok {
		r0 = rf(namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1alpha1.PodPresetInterface)
		}
	}

	return r0
}
