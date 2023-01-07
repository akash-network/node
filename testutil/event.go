package testutil

import (
	"testing"

	"github.com/akash-network/node/sdkutil"
	dtypes "github.com/akash-network/node/x/deployment/types/v1beta2"
	mtypes "github.com/akash-network/node/x/market/types/v1beta2"
	ptypes "github.com/akash-network/node/x/provider/types/v1beta2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
)

func ParseEvent(t testing.TB, events []abci.Event) sdkutil.Event {
	t.Helper()

	require.Equal(t, 1, len(events))

	sev := sdk.StringifyEvent(events[0])
	ev, err := sdkutil.ParseEvent(sev)

	require.NoError(t, err)

	return ev
}

func ParseDeploymentEvent(t testing.TB, events []abci.Event) sdkutil.ModuleEvent {
	t.Helper()

	uev := ParseEvent(t, events)

	iev, err := dtypes.ParseEvent(uev)
	require.NoError(t, err)

	return iev
}

func ParseMarketEvent(t testing.TB, events []abci.Event) sdkutil.ModuleEvent {
	t.Helper()

	uev := ParseEvent(t, events)

	iev, err := mtypes.ParseEvent(uev)
	require.NoError(t, err)

	return iev
}

func ParseProviderEvent(t testing.TB, events []abci.Event) sdkutil.ModuleEvent {
	t.Helper()

	uev := ParseEvent(t, events)

	iev, err := ptypes.ParseEvent(uev)
	require.NoError(t, err)

	return iev
}
