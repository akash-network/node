package market_test

import (
	"context"
	"testing"

	"github.com/ovrclk/akash/app/market"
	"github.com/ovrclk/akash/app/market/mocks"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMarketWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	commitState, _ := testutil.NewState(t, nil)

	delegate := new(mocks.Facilitator)
	delegate.On("Run", commitState).
		Return(nil).Once()

	worker := market.NewWorker(ctx, delegate)
	testutil.SleepForThreadStart(t)
	assert.NoError(t, worker.Run(commitState))
	cancel()

	testutil.SleepForThreadStart(t)
	assert.Error(t, worker.Run(commitState))
}
