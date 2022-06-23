package waiter

import (
	"context"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
	"time"
)

func TestWaiterNoInput(t *testing.T) {
	logger := testutil.Logger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// no objects passed
	waiter := NewOperatorWaiter(ctx, logger)
	require.NotNil(t, waiter)
	require.NoError(t, waiter.WaitForAll(ctx))
}

func TestWaiterContextCancelled(t *testing.T) {
	logger := testutil.Logger(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// no objects passed
	waiter := NewOperatorWaiter(ctx, logger)
	require.NotNil(t, waiter)
	require.ErrorIs(t, waiter.WaitForAll(ctx), context.Canceled)
}

type fakeWaiter struct {
	failure error
}

func (fw fakeWaiter) Check(ctx context.Context) error {
	return fw.failure
}

func (fw fakeWaiter) String() string {
	return "fakeWaiter"
}

func TestWaiterInputReady(t *testing.T) {
	waitable := fakeWaiter{failure: nil}
	logger := testutil.Logger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	waiter := NewOperatorWaiter(ctx, logger, waitable)
	require.NotNil(t, waiter)
	require.NoError(t, waiter.WaitForAll(ctx))
}

func TestWaiterInputFailed(t *testing.T) {
	waitable := fakeWaiter{failure: io.EOF}
	logger := testutil.Logger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	waiter := NewOperatorWaiter(ctx, logger, waitable)
	require.NotNil(t, waiter)
	require.ErrorIs(t, waiter.WaitForAll(ctx), context.DeadlineExceeded)
}
