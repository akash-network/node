// Code generated by mockery v2.5.1. DO NOT EDIT.

package kubernetes_mocks

import (
	mock "github.com/stretchr/testify/mock"
	v1beta1 "k8s.io/client-go/kubernetes/typed/authorization/v1beta1"
)

// SelfSubjectRulesReviewsGetter is an autogenerated mock type for the SelfSubjectRulesReviewsGetter type
type SelfSubjectRulesReviewsGetter struct {
	mock.Mock
}

// SelfSubjectRulesReviews provides a mock function with given fields:
func (_m *SelfSubjectRulesReviewsGetter) SelfSubjectRulesReviews() v1beta1.SelfSubjectRulesReviewInterface {
	ret := _m.Called()

	var r0 v1beta1.SelfSubjectRulesReviewInterface
	if rf, ok := ret.Get(0).(func() v1beta1.SelfSubjectRulesReviewInterface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(v1beta1.SelfSubjectRulesReviewInterface)
		}
	}

	return r0
}
