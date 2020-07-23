package types_test

import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/pkg/errors"

	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

func TestOrderValidateCanMatch(t *testing.T) {
	baseHeight := int64(10000)
	boostHeight := int64(50)

	type testCase struct {
		desc   string
		order  types.Order
		height int64
		expErr error
	}
	tests := []testCase{
		{
			desc: "zero values",
			order: types.Order{
				OrderID: testutil.OrderID(t),
				StartAt: baseHeight,
				Spec:    testutil.GroupSpec(t),
			},
			height: baseHeight,
			expErr: types.ErrOrderClosed,
		},
		{
			desc: "zero value CloseAt",
			order: types.Order{
				OrderID: testutil.OrderID(t),
				State:   types.OrderOpen,
				StartAt: baseHeight,
				// CloseAt is 0
				Spec: testutil.GroupSpec(t),
			},
			height: baseHeight + dtypes.DefaultOrderBiddingDuration + boostHeight,
			expErr: types.ErrOrderDurationExceeded,
		},
		{
			desc: "open order returns no error",
			order: types.Order{
				OrderID: testutil.OrderID(t),
				State:   types.OrderOpen,
				StartAt: baseHeight,
				CloseAt: baseHeight + boostHeight,
				Spec:    testutil.GroupSpec(t),
			},
			height: baseHeight + boostHeight - int64(10),
			expErr: nil,
		},
		{
			desc: "block height beyond CloseAt",
			order: types.Order{
				OrderID: testutil.OrderID(t),
				State:   types.OrderOpen,
				StartAt: baseHeight,
				CloseAt: baseHeight + boostHeight,
				Spec:    testutil.GroupSpec(t),
			},
			height: baseHeight + boostHeight + boostHeight,
			expErr: types.ErrOrderDurationExceeded,
		},
		{
			desc: "state errors makes sense",
			order: types.Order{
				OrderID: testutil.OrderID(t),
				State:   types.OrderClosed,
				StartAt: baseHeight,
				CloseAt: baseHeight + boostHeight,
				Spec:    testutil.GroupSpec(t),
			},
			height: int64(120),
			expErr: types.ErrOrderClosed,
		},
		{
			desc: "assert order matched error",
			order: types.Order{
				OrderID: testutil.OrderID(t),
				State:   types.OrderMatched,
				StartAt: baseHeight,
				CloseAt: baseHeight + boostHeight,
				Spec:    testutil.GroupSpec(t),
			},
			height: int64(80),
			expErr: types.ErrOrderMatched,
		},
		{
			desc: "height less than allowed start",
			order: types.Order{
				OrderID: testutil.OrderID(t),
				State:   types.OrderOpen,
				StartAt: baseHeight,
				CloseAt: baseHeight + boostHeight,
				Spec:    testutil.GroupSpec(t),
			},
			height: int64(90),
			expErr: types.ErrOrderTooEarly,
		},
	}
	for _, test := range tests {
		err := test.order.ValidateCanMatch(test.height)
		if !errors.Is(err, test.expErr) { // cannot use assert.Equal due to lack of support for errorstack
			t.Error(test.desc, ": ", err)
		} else if test.expErr != nil && err == nil {
			t.Error("expected error but none returned")
		}
	}
}
