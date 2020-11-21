package types_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	akashtypes "github.com/ovrclk/akash/types"
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/require"
)

func TestZeroValueGroupSpec(t *testing.T) {
	did := testutil.DeploymentID(t)

	dgroup := testutil.DeploymentGroup(t, did, uint32(6))
	gspec := dgroup.GroupSpec

	t.Run("assert nominal test success", func(t *testing.T) {
		err := gspec.ValidateBasic()
		require.NoError(t, err)
	})

	gspec.OrderBidDuration = int64(0)
	t.Run("assert error for zero value bid duration", func(t *testing.T) {
		err := gspec.ValidateBasic()
		require.Error(t, err)
	})
}

func TestZeroValueGroupSpecs(t *testing.T) {
	did := testutil.DeploymentID(t)
	dgroups := testutil.DeploymentGroups(t, did, uint32(6))
	gspecs := make([]types.GroupSpec, 0)
	for _, d := range dgroups {
		gspecs = append(gspecs, d.GroupSpec)
	}

	t.Run("assert nominal test success", func(t *testing.T) {
		err := types.ValidateDeploymentGroups(gspecs)
		require.NoError(t, err)
	})

	gspecZeroed := make([]types.GroupSpec, len(gspecs))
	for _, g := range gspecs {
		g.OrderBidDuration = int64(0)
		gspecZeroed = append(gspecZeroed, g)
	}
	t.Run("assert error for zero value bid duration", func(t *testing.T) {
		err := types.ValidateDeploymentGroups(gspecZeroed)
		require.Error(t, err)
	})
}

func TestEmptyGroupSpecIsInvalid(t *testing.T) {
	err := types.ValidateDeploymentGroups(make([]types.GroupSpec, 0))
	require.Equal(t, types.ErrInvalidGroups, err)
}

func validSimpleGroupSpec() types.GroupSpec {
	resources := make([]types.Resource, 1)
	resources[0] = types.Resource{
		Resources: akashtypes.ResourceUnits{
			CPU: &akashtypes.CPU{
				Units: akashtypes.ResourceValue{
					Val: sdk.NewInt(10),
				},
				Attributes: nil,
			},
			Memory: &akashtypes.Memory{
				Quantity: akashtypes.ResourceValue{
					Val: sdk.NewInt(1024),
				},
				Attributes: nil,
			},
			Storage: &akashtypes.Storage{
				Quantity: akashtypes.ResourceValue{
					Val: sdk.NewInt(1025),
				},
				Attributes: nil,
			},
			Endpoints: nil,
		},
		Count: 1,
		Price: sdk.Coin{
			Denom:  testutil.CoinDenom,
			Amount: sdk.NewInt(1),
		},
	}
	return types.GroupSpec{
		Name:             "testGroup",
		Requirements:     nil,
		Resources:        resources,
		OrderBidDuration: 3,
	}
}

func validSimpleGroupSpecs() []types.GroupSpec {
	result := make([]types.GroupSpec, 1)
	result[0] = validSimpleGroupSpec()

	return result
}

func TestSimpleGroupSpecIsValid(t *testing.T) {
	groups := validSimpleGroupSpecs()
	err := types.ValidateDeploymentGroups(groups)
	require.NoError(t, err)
}

func TestDuplicateSimpleGroupSpecIsInvalid(t *testing.T) {
	groups := validSimpleGroupSpecs()
	groupsDuplicate := make([]types.GroupSpec, 2)
	groupsDuplicate[0] = groups[0]
	groupsDuplicate[1] = groups[0]
	err := types.ValidateDeploymentGroups(groupsDuplicate)
	require.Error(t, err) // TODO - specific error
	require.Regexp(t, "^.*duplicate.*$", err)
}

func TestGroupWithZeroCount(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Count = 0
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*invalid unit count.*$", err)
}

func TestGroupWithZeroCPU(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Resources.CPU.Units.Val = sdk.NewInt(0)
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*invalid unit CPU.*$", err)
}

func TestGroupWithZeroMemory(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Resources.Memory.Quantity.Val = sdk.NewInt(0)
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*invalid unit memory.*$", err)
}

func TestGroupWithZeroStorage(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Resources.Storage.Quantity.Val = sdk.NewInt(0)
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*invalid unit storage.*$", err)
}

func TestGroupWithNilCPU(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Resources.CPU = nil
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*invalid unit CPU.*$", err)
}

func TestGroupWithNilMemory(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Resources.Memory = nil
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*invalid unit memory.*$", err)
}

func TestGroupWithNilStorage(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Resources.Storage = nil
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*invalid unit storage.*$", err)
}

func TestGroupWithZeroOrderBid(t *testing.T) {
	group := validSimpleGroupSpec()
	group.OrderBidDuration = 0
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*order bid duration must be greater than zero.*$", err)
}

func TestGroupWithInvalidPrice(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Price = sdk.Coin{}
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*invalid price object.*$", err)
}

func TestGroupWithNegativePrice(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Price.Amount = sdk.NewInt(-1)
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*invalid price object.*$", err)
}

func TestGroupWithInvalidDenom(t *testing.T) {
	group := validSimpleGroupSpec()
	group.Resources[0].Price.Denom = "goldenTicket"
	err := group.ValidateBasic()
	require.Error(t, err)
	require.Regexp(t, "^.*denomination must be.*$", err)
}
