package testutil

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/stretchr/testify/require"
)

func ParseEvent(t testing.TB, events sdk.Events, expectedLen int) sdkutil.Event {
	t.Helper()

	require.Equal(t, expectedLen, len(events))

	sev := sdk.StringifyEvent(events.ToABCIEvents()[expectedLen-1])
	ev, err := sdkutil.ParseEvent(sev)

	require.NoError(t, err)

	return ev
}

func ParseDeploymentEvent(t testing.TB, events sdk.Events) sdkutil.ModuleEvent {
	t.Helper()

	uev := ParseEvent(t, events, 1)

	iev, err := dtypes.ParseEvent(uev)
	require.NoError(t, err)

	return iev
}

func ParseMarketEvent(t testing.TB, events sdk.Events, expectedLen int) sdkutil.ModuleEvent {
	t.Helper()

	uev := ParseEvent(t, events, expectedLen)

	iev, err := mtypes.ParseEvent(uev)
	require.NoError(t, err)

	return iev
}
