package types_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/types"
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
			},
			err: types.ErrInvalidVersion,
		},
		{
			msg: &types.MsgUpdateDeployment{
				ID:      testutil.DeploymentID(t),
				Version: testutil.DeploymentVersion(t),
				Groups: []types.GroupSpec{
					testutil.GroupSpec(t),
				},
			},
			err: nil,
		},
		{
			msg: &types.MsgUpdateDeployment{
				ID:      testutil.DeploymentID(t),
				Version: []byte(""),
				Groups: []types.GroupSpec{
					testutil.GroupSpec(t),
				},
			},
			err: types.ErrEmptyVersion,
		},
		{
			msg: &types.MsgUpdateDeployment{
				ID:      testutil.DeploymentID(t),
				Version: []byte("invalidversion"),
				Groups: []types.GroupSpec{
					testutil.GroupSpec(t),
				},
			},
			err: types.ErrInvalidVersion,
		},
	}

	for _, test := range tests {
		require.Equal(t, test.err, test.msg.ValidateBasic())
	}
}
