package types_test

import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/assert"
)

type gStateTest struct {
	state                types.GroupState
	expValidateOrderable error
	expValidateClosable  error
}

func TestGroupState(t *testing.T) {
	tests := []gStateTest{
		{
			state: types.GroupOpen,
		},
		{
			state:                types.GroupOrdered,
			expValidateOrderable: types.ErrGroupNotOpen,
		},
		{
			state:                types.GroupMatched,
			expValidateOrderable: types.ErrGroupNotOpen,
		},
		{
			state:                types.GroupInsufficientFunds,
			expValidateOrderable: types.ErrGroupNotOpen,
		},
		{
			state:                types.GroupClosed,
			expValidateClosable:  types.ErrGroupClosed,
			expValidateOrderable: types.ErrGroupNotOpen,
		},
		{
			state:                types.GroupState(99),
			expValidateOrderable: types.ErrGroupNotOpen,
		},
	}

	for i, test := range tests {
		t.Logf("------test-%d: %#v", i, test)
		group := types.Group{
			GroupID: testutil.GroupID(t),
			State:   test.state,
		}

		assert.Equal(t, group.ValidateOrderable(), test.expValidateOrderable, group.State)

		assert.Equal(t, group.ValidateClosable(), test.expValidateClosable, group.State)
	}
}
