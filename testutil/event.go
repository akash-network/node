package testutil

// import (
// 	"testing"
//
// 	abci "github.com/cometbft/cometbft/abci/types"
// 	sdk "github.com/cosmos/cosmos-sdk/types"
// 	"github.com/stretchr/testify/require"
//
// 	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
// 	mtypes "pkg.akt.dev/go/node/market/v1beta5"
// 	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
// 	"pkg.akt.dev/go/sdkutil"
// )
//
// func ParseEvent(t testing.TB, events []abci.Event) sdkutil.Event {
// 	t.Helper()
//
// 	require.Equal(t, 1, len(events))
//
// 	sev := sdk.StringifyEvent(events[0])
// 	ev, err := sdkutil.ParseEvent(sev)
//
// 	require.NoError(t, err)
//
// 	return ev
// }
//
// func ParseDeploymentEvent(t testing.TB, events []abci.Event) sdkutil.ModuleEvent {
// 	t.Helper()
//
// 	uev := ParseEvent(t, events)
//
// 	iev, err := dtypes.ParseEvent(uev)
// 	require.NoError(t, err)
//
// 	return iev
// }
//
// func ParseMarketEvent(t testing.TB, events []abci.Event) sdkutil.ModuleEvent {
// 	t.Helper()
//
// 	uev := ParseEvent(t, events)
//
// 	iev, err := mtypes.ParseEvent(uev)
// 	require.NoError(t, err)
//
// 	return iev
// }
//
// func ParseProviderEvent(t testing.TB, events []abci.Event) sdkutil.ModuleEvent {
// 	t.Helper()
//
// 	uev := ParseEvent(t, events)
//
// 	iev, err := ptypes.ParseEvent(uev)
// 	require.NoError(t, err)
//
// 	return iev
// }
