// Code generated by mockery v2.5.1. DO NOT EDIT.

package kubernetes_mocks

import (
	context "context"

	appsv1beta2 "k8s.io/api/apps/v1beta2"

	mock "github.com/stretchr/testify/mock"

	types "k8s.io/apimachinery/pkg/types"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1beta2 "k8s.io/client-go/applyconfigurations/apps/v1beta2"

	watch "k8s.io/apimachinery/pkg/watch"
)

// DeploymentInterface is an autogenerated mock type for the DeploymentInterface type
type DeploymentInterface struct {
	mock.Mock
}

// Apply provides a mock function with given fields: ctx, deployment, opts
func (_m *DeploymentInterface) Apply(ctx context.Context, deployment *v1beta2.DeploymentApplyConfiguration, opts v1.ApplyOptions) (*appsv1beta2.Deployment, error) {
	ret := _m.Called(ctx, deployment, opts)

	var r0 *appsv1beta2.Deployment
	if rf, ok := ret.Get(0).(func(context.Context, *v1beta2.DeploymentApplyConfiguration, v1.ApplyOptions) *appsv1beta2.Deployment); ok {
		r0 = rf(ctx, deployment, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*appsv1beta2.Deployment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *v1beta2.DeploymentApplyConfiguration, v1.ApplyOptions) error); ok {
		r1 = rf(ctx, deployment, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ApplyStatus provides a mock function with given fields: ctx, deployment, opts
func (_m *DeploymentInterface) ApplyStatus(ctx context.Context, deployment *v1beta2.DeploymentApplyConfiguration, opts v1.ApplyOptions) (*appsv1beta2.Deployment, error) {
	ret := _m.Called(ctx, deployment, opts)

	var r0 *appsv1beta2.Deployment
	if rf, ok := ret.Get(0).(func(context.Context, *v1beta2.DeploymentApplyConfiguration, v1.ApplyOptions) *appsv1beta2.Deployment); ok {
		r0 = rf(ctx, deployment, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*appsv1beta2.Deployment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *v1beta2.DeploymentApplyConfiguration, v1.ApplyOptions) error); ok {
		r1 = rf(ctx, deployment, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Create provides a mock function with given fields: ctx, deployment, opts
func (_m *DeploymentInterface) Create(ctx context.Context, deployment *appsv1beta2.Deployment, opts v1.CreateOptions) (*appsv1beta2.Deployment, error) {
	ret := _m.Called(ctx, deployment, opts)

	var r0 *appsv1beta2.Deployment
	if rf, ok := ret.Get(0).(func(context.Context, *appsv1beta2.Deployment, v1.CreateOptions) *appsv1beta2.Deployment); ok {
		r0 = rf(ctx, deployment, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*appsv1beta2.Deployment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *appsv1beta2.Deployment, v1.CreateOptions) error); ok {
		r1 = rf(ctx, deployment, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, name, opts
func (_m *DeploymentInterface) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
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
func (_m *DeploymentInterface) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
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
func (_m *DeploymentInterface) Get(ctx context.Context, name string, opts v1.GetOptions) (*appsv1beta2.Deployment, error) {
	ret := _m.Called(ctx, name, opts)

	var r0 *appsv1beta2.Deployment
	if rf, ok := ret.Get(0).(func(context.Context, string, v1.GetOptions) *appsv1beta2.Deployment); ok {
		r0 = rf(ctx, name, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*appsv1beta2.Deployment)
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
func (_m *DeploymentInterface) List(ctx context.Context, opts v1.ListOptions) (*appsv1beta2.DeploymentList, error) {
	ret := _m.Called(ctx, opts)

	var r0 *appsv1beta2.DeploymentList
	if rf, ok := ret.Get(0).(func(context.Context, v1.ListOptions) *appsv1beta2.DeploymentList); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*appsv1beta2.DeploymentList)
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
func (_m *DeploymentInterface) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (*appsv1beta2.Deployment, error) {
	_va := make([]interface{}, len(subresources))
	for _i := range subresources {
		_va[_i] = subresources[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, name, pt, data, opts)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *appsv1beta2.Deployment
	if rf, ok := ret.Get(0).(func(context.Context, string, types.PatchType, []byte, v1.PatchOptions, ...string) *appsv1beta2.Deployment); ok {
		r0 = rf(ctx, name, pt, data, opts, subresources...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*appsv1beta2.Deployment)
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

// Update provides a mock function with given fields: ctx, deployment, opts
func (_m *DeploymentInterface) Update(ctx context.Context, deployment *appsv1beta2.Deployment, opts v1.UpdateOptions) (*appsv1beta2.Deployment, error) {
	ret := _m.Called(ctx, deployment, opts)

	var r0 *appsv1beta2.Deployment
	if rf, ok := ret.Get(0).(func(context.Context, *appsv1beta2.Deployment, v1.UpdateOptions) *appsv1beta2.Deployment); ok {
		r0 = rf(ctx, deployment, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*appsv1beta2.Deployment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *appsv1beta2.Deployment, v1.UpdateOptions) error); ok {
		r1 = rf(ctx, deployment, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateStatus provides a mock function with given fields: ctx, deployment, opts
func (_m *DeploymentInterface) UpdateStatus(ctx context.Context, deployment *appsv1beta2.Deployment, opts v1.UpdateOptions) (*appsv1beta2.Deployment, error) {
	ret := _m.Called(ctx, deployment, opts)

	var r0 *appsv1beta2.Deployment
	if rf, ok := ret.Get(0).(func(context.Context, *appsv1beta2.Deployment, v1.UpdateOptions) *appsv1beta2.Deployment); ok {
		r0 = rf(ctx, deployment, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*appsv1beta2.Deployment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *appsv1beta2.Deployment, v1.UpdateOptions) error); ok {
		r1 = rf(ctx, deployment, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Watch provides a mock function with given fields: ctx, opts
func (_m *DeploymentInterface) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
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
