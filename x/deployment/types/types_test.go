package types_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/testutil"
	atypes "github.com/ovrclk/akash/x/audit/types"

	"github.com/ovrclk/akash/x/deployment/types"
)

type gStateTest struct {
	state                types.Group_State
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
			state:                types.Group_State(99),
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
				Requirements:     testutil.PlacementRequirements(t),
				Resources:        testutil.Resources(t),
				OrderBidDuration: int64(0),
			},
			expErr: types.ErrInvalidGroups,
		},
		{
			desc: "bid duration exceeds limit",
			gspec: types.GroupSpec{
				Name:             testutil.Name(t, "groupspec"),
				Requirements:     testutil.PlacementRequirements(t),
				Resources:        testutil.Resources(t),
				OrderBidDuration: types.MaxBiddingDuration * int64(2),
			},
			expErr: types.ErrInvalidGroups,
		},
		{
			desc: "groupspec requires name",
			gspec: types.GroupSpec{
				Name:             "",
				Requirements:     testutil.PlacementRequirements(t),
				Resources:        testutil.Resources(t),
				OrderBidDuration: types.DefaultOrderBiddingDuration,
			},
			expErr: types.ErrInvalidGroups,
		},
		{
			desc: "groupspec valid",
			gspec: types.GroupSpec{
				Name:             "hihi",
				Requirements:     testutil.PlacementRequirements(t),
				Resources:        testutil.Resources(t),
				OrderBidDuration: types.DefaultOrderBiddingDuration,
			},
			expErr: nil,
		},
	}

	for _, test := range tests {
		err := test.gspec.ValidateBasic()
		if test.expErr != nil {
			assert.Error(t, err)
			continue
		}
		assert.Equal(t, test.expErr, err, test.desc)
	}
}

func TestGroupPlacementRequirementsNoSigners(t *testing.T) {
	group := types.GroupSpec{
		Name:             "spec",
		Requirements:     testutil.PlacementRequirements(t),
		Resources:        testutil.Resources(t),
		OrderBidDuration: types.DefaultOrderBiddingDuration,
	}

	providerAttr := []atypes.Provider{
		{
			Owner:      "test",
			Attributes: group.Requirements.Attributes,
		},
	}

	require.True(t, group.MatchRequirements(providerAttr))
}

func TestGroupPlacementRequirementsSignerAllOf(t *testing.T) {
	group := types.GroupSpec{
		Name:             "spec",
		Requirements:     testutil.PlacementRequirements(t),
		Resources:        testutil.Resources(t),
		OrderBidDuration: types.DefaultOrderBiddingDuration,
	}

	group.Requirements.SignedBy.AllOf = append(group.Requirements.SignedBy.AllOf, "validator1")
	group.Requirements.SignedBy.AllOf = append(group.Requirements.SignedBy.AllOf, "validator2")

	providerAttr := []atypes.Provider{
		{
			Owner:      "test",
			Attributes: group.Requirements.Attributes,
		},
	}

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Validator:  "validator2",
		Attributes: group.Requirements.Attributes,
	})

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Validator:  "validator1",
		Attributes: group.Requirements.Attributes,
	})

	require.True(t, group.MatchRequirements(providerAttr))
}

func TestGroupPlacementRequirementsSignerAnyOf(t *testing.T) {
	group := types.GroupSpec{
		Name:             "spec",
		Requirements:     testutil.PlacementRequirements(t),
		Resources:        testutil.Resources(t),
		OrderBidDuration: types.DefaultOrderBiddingDuration,
	}

	group.Requirements.SignedBy.AllOf = append(group.Requirements.SignedBy.AllOf, "validator1")

	providerAttr := []atypes.Provider{
		{
			Owner:      "test",
			Attributes: group.Requirements.Attributes,
		},
	}

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Validator:  "validator2",
		Attributes: group.Requirements.Attributes,
	})

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Validator:  "validator1",
		Attributes: group.Requirements.Attributes,
	})

	require.True(t, group.MatchRequirements(providerAttr))
}

func TestGroupPlacementRequirementsSignerAllOfAnyOf(t *testing.T) {
	group := types.GroupSpec{
		Name:             "spec",
		Requirements:     testutil.PlacementRequirements(t),
		Resources:        testutil.Resources(t),
		OrderBidDuration: types.DefaultOrderBiddingDuration,
	}

	group.Requirements.SignedBy.AllOf = append(group.Requirements.SignedBy.AllOf, "validator1")
	group.Requirements.SignedBy.AllOf = append(group.Requirements.SignedBy.AllOf, "validator2")

	group.Requirements.SignedBy.AnyOf = append(group.Requirements.SignedBy.AnyOf, "validator3")
	group.Requirements.SignedBy.AnyOf = append(group.Requirements.SignedBy.AnyOf, "validator4")

	providerAttr := []atypes.Provider{
		{
			Owner:      "test",
			Attributes: group.Requirements.Attributes,
		},
		{
			Owner:      "test",
			Validator:  "validator3",
			Attributes: group.Requirements.Attributes,
		},
		{
			Owner:      "test",
			Validator:  "validator4",
			Attributes: group.Requirements.Attributes,
		},
	}

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Validator:  "validator2",
		Attributes: group.Requirements.Attributes,
	})

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Validator:  "validator1",
		Attributes: group.Requirements.Attributes,
	})

	require.True(t, group.MatchRequirements(providerAttr))
}
