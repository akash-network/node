package v1beta2_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/testutil"
	types "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	"github.com/stretchr/testify/require"
)

type testMsg struct {
	msg sdk.Msg
	err error
}

func TestVersionValidation(t *testing.T) {
	tests := []testMsg{
		{
			msg: &types.MsgCreateDeployment{
				ID:      testutil.DeploymentID(t),
				Version: testutil.DeploymentVersion(t),
				Groups: []types.GroupSpec{
					testutil.GroupSpec(t),
				},
				Depositor: testutil.AccAddress(t).String(),
			},
			err: nil,
		},
		{
			msg: &types.MsgCreateDeployment{
				ID:      testutil.DeploymentID(t),
				Version: []byte(""),
				Groups: []types.GroupSpec{
					testutil.GroupSpec(t),
				},
				Depositor: testutil.AccAddress(t).String(),
			},
			err: types.ErrEmptyVersion,
		},
		{
			msg: &types.MsgCreateDeployment{
				ID:      testutil.DeploymentID(t),
				Version: []byte("invalidversion"),
				Groups: []types.GroupSpec{
					testutil.GroupSpec(t),
				},
				Depositor: testutil.AccAddress(t).String(),
			},
			err: types.ErrInvalidVersion,
		},
		{
			msg: &types.MsgUpdateDeployment{
				ID:      testutil.DeploymentID(t),
				Version: testutil.DeploymentVersion(t),
			},
			err: nil,
		},
		{
			msg: &types.MsgUpdateDeployment{
				ID:      testutil.DeploymentID(t),
				Version: []byte(""),
			},
			err: types.ErrEmptyVersion,
		},
		{
			msg: &types.MsgUpdateDeployment{
				ID:      testutil.DeploymentID(t),
				Version: []byte("invalidversion"),
			},
			err: types.ErrInvalidVersion,
		},
	}

	for _, test := range tests {
		require.Equal(t, test.err, test.msg.ValidateBasic())
	}
}
