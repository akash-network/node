// Code generated by mockery v1.1.2. DO NOT EDIT.

package kubernetes_mocks

import (
	context "context"

	autoscalingv1 "k8s.io/api/autoscaling/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mock "github.com/stretchr/testify/mock"

	types "k8s.io/apimachinery/pkg/types"

	v1 "k8s.io/api/core/v1"

	watch "k8s.io/apimachinery/pkg/watch"
)

// ReplicationControllerInterface is an autogenerated mock type for the ReplicationControllerInterface type
type ReplicationControllerInterface struct {
	mock.Mock
}

// Create provides a mock function with given fields: ctx, replicationController, opts
func (_m *ReplicationControllerInterface) Create(ctx context.Context, replicationController *v1.ReplicationController, opts metav1.CreateOptions) (*v1.ReplicationController, error) {
	ret := _m.Called(ctx, replicationController, opts)

	var r0 *v1.ReplicationController
	if rf, ok := ret.Get(0).(func(context.Context, *v1.ReplicationController, metav1.CreateOptions) *v1.ReplicationController); ok {
		r0 = rf(ctx, replicationController, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.ReplicationController)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *v1.ReplicationController, metav1.CreateOptions) error); ok {
		r1 = rf(ctx, replicationController, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, name, opts
func (_m *ReplicationControllerInterface) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	ret := _m.Called(ctx, name, opts)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, metav1.DeleteOptions) error); ok {
		r0 = rf(ctx, name, opts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DeleteCollection provides a mock function with given fields: ctx, opts, listOpts
func (_m *ReplicationControllerInterface) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	ret := _m.Called(ctx, opts, listOpts)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, metav1.DeleteOptions, metav1.ListOptions) error); ok {
		r0 = rf(ctx, opts, listOpts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Get provides a mock function with given fields: ctx, name, opts
func (_m *ReplicationControllerInterface) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.ReplicationController, error) {
	ret := _m.Called(ctx, name, opts)

	var r0 *v1.ReplicationController
	if rf, ok := ret.Get(0).(func(context.Context, string, metav1.GetOptions) *v1.ReplicationController); ok {
		r0 = rf(ctx, name, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.ReplicationController)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, metav1.GetOptions) error); ok {
		r1 = rf(ctx, name, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetScale provides a mock function with given fields: ctx, replicationControllerName, options
func (_m *ReplicationControllerInterface) GetScale(ctx context.Context, replicationControllerName string, options metav1.GetOptions) (*autoscalingv1.Scale, error) {
	ret := _m.Called(ctx, replicationControllerName, options)

	var r0 *autoscalingv1.Scale
	if rf, ok := ret.Get(0).(func(context.Context, string, metav1.GetOptions) *autoscalingv1.Scale); ok {
		r0 = rf(ctx, replicationControllerName, options)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*autoscalingv1.Scale)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, metav1.GetOptions) error); ok {
		r1 = rf(ctx, replicationControllerName, options)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// List provides a mock function with given fields: ctx, opts
func (_m *ReplicationControllerInterface) List(ctx context.Context, opts metav1.ListOptions) (*v1.ReplicationControllerList, error) {
	ret := _m.Called(ctx, opts)

	var r0 *v1.ReplicationControllerList
	if rf, ok := ret.Get(0).(func(context.Context, metav1.ListOptions) *v1.ReplicationControllerList); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.ReplicationControllerList)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, metav1.ListOptions) error); ok {
		r1 = rf(ctx, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Patch provides a mock function with given fields: ctx, name, pt, data, opts, subresources
func (_m *ReplicationControllerInterface) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*v1.ReplicationController, error) {
	_va := make([]interface{}, len(subresources))
	for _i := range subresources {
		_va[_i] = subresources[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, name, pt, data, opts)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1.ReplicationController
	if rf, ok := ret.Get(0).(func(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) *v1.ReplicationController); ok {
		r0 = rf(ctx, name, pt, data, opts, subresources...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.ReplicationController)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) error); ok {
		r1 = rf(ctx, name, pt, data, opts, subresources...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Update provides a mock function with given fields: ctx, replicationController, opts
func (_m *ReplicationControllerInterface) Update(ctx context.Context, replicationController *v1.ReplicationController, opts metav1.UpdateOptions) (*v1.ReplicationController, error) {
	ret := _m.Called(ctx, replicationController, opts)

	var r0 *v1.ReplicationController
	if rf, ok := ret.Get(0).(func(context.Context, *v1.ReplicationController, metav1.UpdateOptions) *v1.ReplicationController); ok {
		r0 = rf(ctx, replicationController, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.ReplicationController)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *v1.ReplicationController, metav1.UpdateOptions) error); ok {
		r1 = rf(ctx, replicationController, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateScale provides a mock function with given fields: ctx, replicationControllerName, scale, opts
func (_m *ReplicationControllerInterface) UpdateScale(ctx context.Context, replicationControllerName string, scale *autoscalingv1.Scale, opts metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	ret := _m.Called(ctx, replicationControllerName, scale, opts)

	var r0 *autoscalingv1.Scale
	if rf, ok := ret.Get(0).(func(context.Context, string, *autoscalingv1.Scale, metav1.UpdateOptions) *autoscalingv1.Scale); ok {
		r0 = rf(ctx, replicationControllerName, scale, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*autoscalingv1.Scale)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, *autoscalingv1.Scale, metav1.UpdateOptions) error); ok {
		r1 = rf(ctx, replicationControllerName, scale, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateStatus provides a mock function with given fields: ctx, replicationController, opts
func (_m *ReplicationControllerInterface) UpdateStatus(ctx context.Context, replicationController *v1.ReplicationController, opts metav1.UpdateOptions) (*v1.ReplicationController, error) {
	ret := _m.Called(ctx, replicationController, opts)

	var r0 *v1.ReplicationController
	if rf, ok := ret.Get(0).(func(context.Context, *v1.ReplicationController, metav1.UpdateOptions) *v1.ReplicationController); ok {
		r0 = rf(ctx, replicationController, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.ReplicationController)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *v1.ReplicationController, metav1.UpdateOptions) error); ok {
		r1 = rf(ctx, replicationController, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Watch provides a mock function with given fields: ctx, opts
func (_m *ReplicationControllerInterface) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	ret := _m.Called(ctx, opts)

	var r0 watch.Interface
	if rf, ok := ret.Get(0).(func(context.Context, metav1.ListOptions) watch.Interface); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(watch.Interface)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, metav1.ListOptions) error); ok {
		r1 = rf(ctx, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
