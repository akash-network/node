package testutil

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/require"
)

func ParseEvent(t testing.TB, events sdk.Events) sdkutil.Event {
	t.Helper()

	require.Equal(t, 1, len(events))

	sev := sdk.StringifyEvent(events.ToABCIEvents()[0])
	ev, err := sdkutil.ParseEvent(sev)

	require.NoError(t, err)

	return ev
}

func ParseDeploymentEvent(t testing.TB, events sdk.Events) sdkutil.ModuleEvent {
	t.Helper()

	uev := ParseEvent(t, events)

	iev, err := dtypes.ParseEvent(uev)
	require.NoError(t, err)

	return iev
}
