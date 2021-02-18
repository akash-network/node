// Code generated by mockery v2.5.1. DO NOT EDIT.

package kubernetes_mocks

import (
	mock "github.com/stretchr/testify/mock"
	v1beta1 "k8s.io/client-go/kubernetes/typed/authentication/v1beta1"
)

// TokenReviewsGetter is an autogenerated mock type for the TokenReviewsGetter type
type TokenReviewsGetter struct {
	mock.Mock
}

// TokenReviews provides a mock function with given fields:
func (_m *TokenReviewsGetter) TokenReviews() v1beta1.TokenReviewInterface {
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
