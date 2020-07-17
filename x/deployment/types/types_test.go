package types_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/types"
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

	for _, test := range tests {
		group := types.Group{
			GroupID: testutil.GroupID(t),
			State:   test.state,
		}

		assert.Equal(t, group.ValidateOrderable(), test.expValidateOrderable, group.State)

		assert.Equal(t, group.ValidateClosable(), test.expValidateClosable, group.State)
	}
}

func TestDeploymentVersionAttributeLifecycle(t *testing.T) {
	d := testutil.Deployment(t)

	t.Run("deployment created", func(t *testing.T) {
		edc := types.NewEventDeploymentCreated(d.ID(), d.Version)
		sdkEvent := edc.ToSDKEvent()
		strEvent := sdk.StringifyEvent(abci.Event(sdkEvent))

		ev, err := sdkutil.ParseEvent(strEvent)
		require.NoError(t, err)

		versionString, err := types.ParseEVDeploymentVersion(ev.Attributes)
		require.NoError(t, err)
		assert.Equal(t, d.Version, versionString)
	})

	t.Run("deployment updated", func(t *testing.T) {
		edu := types.NewEventDeploymentUpdated(d.ID(), d.Version)

		sdkEvent := edu.ToSDKEvent()
		strEvent := sdk.StringifyEvent(abci.Event(sdkEvent))

		ev, err := sdkutil.ParseEvent(strEvent)
		require.NoError(t, err)

		versionString, err := types.ParseEVDeploymentVersion(ev.Attributes)
		require.NoError(t, err)
		assert.Equal(t, d.Version, versionString)
	})

	t.Run("deployment closed error", func(t *testing.T) {
		edc := types.NewEventDeploymentClosed(d.ID())

		sdkEvent := edc.ToSDKEvent()
		strEvent := sdk.StringifyEvent(abci.Event(sdkEvent))

		ev, err := sdkutil.ParseEvent(strEvent)
		require.NoError(t, err)

		versionString, err := types.ParseEVDeploymentVersion(ev.Attributes)
		require.Error(t, err)
		assert.NotEqual(t, d.Version, versionString)
	})
}

func TestGroupSpecValidation(t *testing.T) {
	tests := []struct {
		desc   string
		gspec  types.GroupSpec
		expErr error
	}{
		{
			desc: "zero value bid duration error",
			gspec: types.GroupSpec{
				Name:             testutil.Name(t, "groupspec"),
				Requirements:     testutil.Attributes(t),
				Resources:        testutil.Resources(t),
				OrderBidDuration: int64(0),
			},
			expErr: types.ErrInvalidGroups,
		},
		{
			desc: "bid duration exceeds limit",
			gspec: types.GroupSpec{
				Name:             testutil.Name(t, "groupspec"),
				Requirements:     testutil.Attributes(t),
				Resources:        testutil.Resources(t),
				OrderBidDuration: types.MaxBiddingDuration * int64(2),
			},
			expErr: types.ErrInvalidGroups,
		},
		{
			desc: "groupspec requires name",
			gspec: types.GroupSpec{
				Name:             "",
				Requirements:     testutil.Attributes(t),
				Resources:        testutil.Resources(t),
				OrderBidDuration: types.DefaultOrderBiddingDuration,
			},
			expErr: types.ErrInvalidGroups,
		},
		{
			desc: "groupspec valid",
			gspec: types.GroupSpec{
				Name:             "hihi",
				Requirements:     testutil.Attributes(t),
				Resources:        testutil.Resources(t),
				OrderBidDuration: types.DefaultOrderBiddingDuration,
			},
			expErr: nil,
		},
	}

	for _, test := range tests {
		err := test.gspec.ValidateBasic()
		assert.Equal(t, test.expErr, err, test.desc)
	}
}
