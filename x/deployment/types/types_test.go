package types_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/testutil"
	akashtypes "github.com/ovrclk/akash/types"
	atypes "github.com/ovrclk/akash/x/audit/types"

	"github.com/ovrclk/akash/x/deployment/types"
)

type gStateTest struct {
	state               types.Group_State
	expValidateClosable error
}

func TestGroupState(t *testing.T) {
	tests := []gStateTest{
		{
			state: types.GroupOpen,
		},
		{
			state: types.GroupOpen,
		},
		{
			state: types.GroupInsufficientFunds,
		},
		{
			state:               types.GroupClosed,
			expValidateClosable: types.ErrGroupClosed,
		},
		{
			state: types.Group_State(99),
		},
	}

	for _, test := range tests {
		group := types.Group{
			GroupID: testutil.GroupID(t),
			State:   test.state,
		}

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
			desc: "groupspec requires name",
			gspec: types.GroupSpec{
				Name:         "",
				Requirements: testutil.PlacementRequirements(t),
				Resources:    testutil.Resources(t),
			},
			expErr: types.ErrInvalidGroups,
		},
		{
			desc: "groupspec valid",
			gspec: types.GroupSpec{
				Name:         "hihi",
				Requirements: testutil.PlacementRequirements(t),
				Resources:    testutil.Resources(t),
			},
			expErr: nil,
		},
	}

	for _, test := range tests {
		err := test.gspec.ValidateBasic()
		if test.expErr != nil {
			assert.Error(t, err, test.desc)
			continue
		}
		assert.Equal(t, test.expErr, err, test.desc)
	}
}

func TestGroupPlacementRequirementsNoSigners(t *testing.T) {
	group := types.GroupSpec{
		Name:         "spec",
		Requirements: testutil.PlacementRequirements(t),
		Resources:    testutil.Resources(t),
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
		Name:         "spec",
		Requirements: testutil.PlacementRequirements(t),
		Resources:    testutil.Resources(t),
	}

	group.Requirements.SignedBy.AllOf = append(group.Requirements.SignedBy.AllOf, "auditor1")
	group.Requirements.SignedBy.AllOf = append(group.Requirements.SignedBy.AllOf, "auditor2")

	providerAttr := []atypes.Provider{
		{
			Owner:      "test",
			Attributes: group.Requirements.Attributes,
		},
	}

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Auditor:    "auditor2",
		Attributes: group.Requirements.Attributes,
	})

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Auditor:    "auditor1",
		Attributes: group.Requirements.Attributes,
	})

	require.True(t, group.MatchRequirements(providerAttr))
}

func TestGroupPlacementRequirementsSignerAnyOf(t *testing.T) {
	group := types.GroupSpec{
		Name:         "spec",
		Requirements: testutil.PlacementRequirements(t),
		Resources:    testutil.Resources(t),
	}

	group.Requirements.SignedBy.AnyOf = append(group.Requirements.SignedBy.AnyOf, "auditor1")

	providerAttr := []atypes.Provider{
		{
			Owner:      "test",
			Attributes: group.Requirements.Attributes,
		},
	}

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Auditor:    "auditor2",
		Attributes: group.Requirements.Attributes,
	})

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Auditor:    "auditor1",
		Attributes: group.Requirements.Attributes,
	})

	require.True(t, group.MatchRequirements(providerAttr))
}

func TestGroupPlacementRequirementsSignerAllOfAnyOf(t *testing.T) {
	group := types.GroupSpec{
		Name:         "spec",
		Requirements: testutil.PlacementRequirements(t),
		Resources:    testutil.Resources(t),
	}

	group.Requirements.SignedBy.AllOf = append(group.Requirements.SignedBy.AllOf, "auditor1")
	group.Requirements.SignedBy.AllOf = append(group.Requirements.SignedBy.AllOf, "auditor2")

	group.Requirements.SignedBy.AnyOf = append(group.Requirements.SignedBy.AnyOf, "auditor3")
	group.Requirements.SignedBy.AnyOf = append(group.Requirements.SignedBy.AnyOf, "auditor4")

	providerAttr := []atypes.Provider{
		{
			Owner:      "test",
			Attributes: group.Requirements.Attributes,
		},
		{
			Owner:      "test",
			Auditor:    "auditor3",
			Attributes: group.Requirements.Attributes,
		},
		{
			Owner:      "test",
			Auditor:    "auditor4",
			Attributes: group.Requirements.Attributes,
		},
	}

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Auditor:    "auditor2",
		Attributes: group.Requirements.Attributes,
	})

	require.False(t, group.MatchRequirements(providerAttr))

	providerAttr = append(providerAttr, atypes.Provider{
		Owner:      "test",
		Auditor:    "auditor1",
		Attributes: group.Requirements.Attributes,
	})

	require.True(t, group.MatchRequirements(providerAttr))
}

func TestGroupSpec_MatchResourcesAttributes(t *testing.T) {
	group := types.GroupSpec{
		Name:         "spec",
		Requirements: testutil.PlacementRequirements(t),
		Resources:    testutil.Resources(t),
	}

	group.Resources[0].Resources.Storage[0].Attributes = akashtypes.Attributes{
		{
			Key:   "persistent",
			Value: "true",
		},
		{
			Key:   "class",
			Value: "default",
		},
	}

	provAttributes := akashtypes.Attributes{
		{
			Key:   "capabilities/storage/1/class",
			Value: "default",
		},
		{
			Key:   "capabilities/storage/1/persistent",
			Value: "true",
		},
	}

	prov2Attributes := akashtypes.Attributes{
		{
			Key:   "capabilities/storage/1/class",
			Value: "default",
		},
	}

	prov3Attributes := akashtypes.Attributes{
		{
			Key:   "capabilities/storage/1/class",
			Value: "beta2",
		},
	}

	require.True(t, group.MatchResourcesRequirements(provAttributes))
	require.False(t, group.MatchResourcesRequirements(prov2Attributes))
	require.False(t, group.MatchResourcesRequirements(prov3Attributes))
}
