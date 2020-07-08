package events

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/testutil"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
	"github.com/stretchr/testify/assert"
)

func Test_processEvent(t *testing.T) {
	tests := []sdkutil.ModuleEvent{
		// x/deployment events
		dtypes.NewEventDeploymentCreated(testutil.DeploymentID(t)),
		dtypes.NewEventDeploymentUpdated(testutil.DeploymentID(t)),
		dtypes.NewEventDeploymentClosed(testutil.DeploymentID(t)),
		dtypes.NewEventGroupClosed(testutil.GroupID(t)),

		// x/market events
		mtypes.NewEventOrderCreated(testutil.OrderID(t)),
		mtypes.NewEventOrderClosed(testutil.OrderID(t)),
		mtypes.NewEventBidCreated(testutil.BidID(t), testutil.Coin(t)),
		mtypes.NewEventBidClosed(testutil.BidID(t), testutil.Coin(t)),
		mtypes.NewEventLeaseCreated(testutil.LeaseID(t), testutil.Coin(t)),
		mtypes.NewEventLeaseClosed(testutil.LeaseID(t), testutil.Coin(t)),

		// x/provider events
		ptypes.NewEventProviderCreated(testutil.AccAddress(t)),
		ptypes.NewEventProviderUpdated(testutil.AccAddress(t)),
		ptypes.NewEventProviderDeleted(testutil.AccAddress(t)),
	}

	for _, test := range tests {
		sdkevs := sdk.Events{
			test.ToSDKEvent(),
		}.ToABCIEvents()

		sdkev := sdkevs[0]

		ev, ok := processEvent(sdkev)
		assert.True(t, ok, test)
		assert.Equal(t, test, ev, test)
	}
}
