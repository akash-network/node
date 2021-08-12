// Code generated by mockery v2.5.1. DO NOT EDIT.

package kubernetes_mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	storagev1beta1 "k8s.io/api/storage/v1beta1"

	types "k8s.io/apimachinery/pkg/types"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1beta1 "k8s.io/client-go/applyconfigurations/storage/v1beta1"

	watch "k8s.io/apimachinery/pkg/watch"
)

// CSINodeInterface is an autogenerated mock type for the CSINodeInterface type
type CSINodeInterface struct {
	mock.Mock
}

// Apply provides a mock function with given fields: ctx, cSINode, opts
func (_m *CSINodeInterface) Apply(ctx context.Context, cSINode *v1beta1.CSINodeApplyConfiguration, opts v1.ApplyOptions) (*storagev1beta1.CSINode, error) {
	ret := _m.Called(ctx, cSINode, opts)

	var r0 *storagev1beta1.CSINode
	if rf, ok := ret.Get(0).(func(context.Context, *v1beta1.CSINodeApplyConfiguration, v1.ApplyOptions) *storagev1beta1.CSINode); ok {
		r0 = rf(ctx, cSINode, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storagev1beta1.CSINode)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *v1beta1.CSINodeApplyConfiguration, v1.ApplyOptions) error); ok {
		r1 = rf(ctx, cSINode, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Create provides a mock function with given fields: ctx, cSINode, opts
func (_m *CSINodeInterface) Create(ctx context.Context, cSINode *storagev1beta1.CSINode, opts v1.CreateOptions) (*storagev1beta1.CSINode, error) {
	ret := _m.Called(ctx, cSINode, opts)

	var r0 *storagev1beta1.CSINode
	if rf, ok := ret.Get(0).(func(context.Context, *storagev1beta1.CSINode, v1.CreateOptions) *storagev1beta1.CSINode); ok {
		r0 = rf(ctx, cSINode, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storagev1beta1.CSINode)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *storagev1beta1.CSINode, v1.CreateOptions) error); ok {
		r1 = rf(ctx, cSINode, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, name, opts
func (_m *CSINodeInterface) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	ret := _m.Called(ctx, name, opts)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, v1.DeleteOptions) error); ok {
		r0 = rf(ctx, name, opts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DeleteCollection provides a mock function with given fields: ctx, opts, listOpts
func (_m *CSINodeInterface) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	ret := _m.Called(ctx, opts, listOpts)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, v1.DeleteOptions, v1.ListOptions) error); ok {
		r0 = rf(ctx, opts, listOpts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Get provides a mock function with given fields: ctx, name, opts
func (_m *CSINodeInterface) Get(ctx context.Context, name string, opts v1.GetOptions) (*storagev1beta1.CSINode, error) {
	ret := _m.Called(ctx, name, opts)

	var r0 *storagev1beta1.CSINode
	if rf, ok := ret.Get(0).(func(context.Context, string, v1.GetOptions) *storagev1beta1.CSINode); ok {
		r0 = rf(ctx, name, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storagev1beta1.CSINode)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, v1.GetOptions) error); ok {
		r1 = rf(ctx, name, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// List provides a mock function with given fields: ctx, opts
func (_m *CSINodeInterface) List(ctx context.Context, opts v1.ListOptions) (*storagev1beta1.CSINodeList, error) {
	ret := _m.Called(ctx, opts)

	var r0 *storagev1beta1.CSINodeList
	if rf, ok := ret.Get(0).(func(context.Context, v1.ListOptions) *storagev1beta1.CSINodeList); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storagev1beta1.CSINodeList)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, v1.ListOptions) error); ok {
		r1 = rf(ctx, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Patch provides a mock function with given fields: ctx, name, pt, data, opts, subresources
func (_m *CSINodeInterface) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (*storagev1beta1.CSINode, error) {
	_va := make([]interface{}, len(subresources))
	for _i := range subresources {
		_va[_i] = subresources[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, name, pt, data, opts)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *storagev1beta1.CSINode
	if rf, ok := ret.Get(0).(func(context.Context, string, types.PatchType, []byte, v1.PatchOptions, ...string) *storagev1beta1.CSINode); ok {
		r0 = rf(ctx, name, pt, data, opts, subresources...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storagev1beta1.CSINode)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, types.PatchType, []byte, v1.PatchOptions, ...string) error); ok {
		r1 = rf(ctx, name, pt, data, opts, subresources...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Update provides a mock function with given fields: ctx, cSINode, opts
func (_m *CSINodeInterface) Update(ctx context.Context, cSINode *storagev1beta1.CSINode, opts v1.UpdateOptions) (*storagev1beta1.CSINode, error) {
	ret := _m.Called(ctx, cSINode, opts)

	var r0 *storagev1beta1.CSINode
	if rf, ok := ret.Get(0).(func(context.Context, *storagev1beta1.CSINode, v1.UpdateOptions) *storagev1beta1.CSINode); ok {
		r0 = rf(ctx, cSINode, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storagev1beta1.CSINode)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *storagev1beta1.CSINode, v1.UpdateOptions) error); ok {
		r1 = rf(ctx, cSINode, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Watch provides a mock function with given fields: ctx, opts
func (_m *CSINodeInterface) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	ret := _m.Called(ctx, opts)

	var r0 watch.Interface
	if rf, ok := ret.Get(0).(func(context.Context, v1.ListOptions) watch.Interface); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(watch.Interface)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, v1.ListOptions) error); ok {
		r1 = rf(ctx, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
