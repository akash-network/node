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
		dtypes.EventDeploymentCreate{ID: testutil.DeploymentID(t)},
		dtypes.EventDeploymentUpdate{ID: testutil.DeploymentID(t)},
		dtypes.EventDeploymentClose{ID: testutil.DeploymentID(t)},
		dtypes.EventGroupClose{ID: testutil.GroupID(t)},

		// x/market events
		mtypes.EventOrderCreated{ID: testutil.OrderID(t)},
		mtypes.EventOrderClosed{ID: testutil.OrderID(t)},
		mtypes.EventBidCreated{ID: testutil.BidID(t), Price: testutil.Coin(t)},
		mtypes.EventBidClosed{ID: testutil.BidID(t), Price: testutil.Coin(t)},
		mtypes.EventLeaseCreated{ID: testutil.LeaseID(t), Price: testutil.Coin(t)},
		mtypes.EventLeaseClosed{ID: testutil.LeaseID(t), Price: testutil.Coin(t)},

		// x/provider events
		ptypes.EventProviderCreate{Owner: testutil.AccAddress(t)},
		ptypes.EventProviderUpdate{Owner: testutil.AccAddress(t)},
		ptypes.EventProviderDelete{Owner: testutil.AccAddress(t)},
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
