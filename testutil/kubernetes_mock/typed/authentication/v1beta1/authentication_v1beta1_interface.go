// Code generated by mockery v1.1.2. DO NOT EDIT.

package kubernetes_mocks

import (
	mock "github.com/stretchr/testify/mock"
	rest "k8s.io/client-go/rest"

	v1beta1 "k8s.io/client-go/kubernetes/typed/authentication/v1beta1"
)

// AuthenticationV1beta1Interface is an autogenerated mock type for the AuthenticationV1beta1Interface type
type AuthenticationV1beta1Interface struct {
	mock.Mock
}

// RESTClient provides a mock function with given fields:
func (_m *AuthenticationV1beta1Interface) RESTClient() rest.Interface {
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

// TokenReviews provides a mock function with given fields:
func (_m *AuthenticationV1beta1Interface) TokenReviews() v1beta1.TokenReviewInterface {
	ret := _m.Called()

	var r0 v1beta1.TokenReviewInterface
	if rf, ok := ret.Get(0).(func() v1beta1.TokenReviewInterface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1beta1.TokenReviewInterface)
		}
	}

	return r0
}
