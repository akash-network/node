package app_test

import (
	"reflect"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

// sinkEvents decodes the typed sink events in order, skipping the events of
// other modules, such as bank's coin_spent.
func sinkEvents(t *testing.T, events []abci.Event) []proto.Message {
	t.Helper()
	pkg := reflect.TypeOf(types.Params{}).PkgPath()
	var decoded []proto.Message
	for _, event := range events {
		if proto.MessageType(event.Type) == nil {
			continue
		}

		msg, err := sdk.ParseTypedEvent(event)
		require.NoError(t, err)

		if reflect.TypeOf(msg).Elem().PkgPath() == pkg {
			decoded = append(decoded, msg)
		}
	}

	return decoded
}

// reporterAddress is the account of reporter i.
func (h *sinkHarness) reporterAddress(i int) string {
	return sdk.AccAddress(h.reporters[i].PubKey().Address()).String()
}

func TestSinkVoidAndRefundEvents(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	events := sinkEvents(t, res.Events)
	require.Contains(t, events, proto.Message(&types.EventTotalsUpdated{Totals: h.totals()}))
	require.NoError(t, h.order(h.id(1)))
	h.quorum(t, 1, types.OutcomeVoided)
	ctx := h.ctx().WithEventManager(sdk.NewEventManager())
	k := h.a.Keepers.Akash.Sink
	result, err := h.a.MsgServiceRouter().Handler(&types.MsgVetoVoid{})(ctx, &types.MsgVetoVoid{Signer: h.reporterAddress(0), ID: h.id(1), Reason: "review"})
	require.NoError(t, err)
	require.Equal(t, []proto.Message{&types.EventVoidVetoed{ID: h.id(1), Signer: h.reporterAddress(0), Reason: "review"}}, sinkEvents(t, result.Events))
	require.NoError(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
	h.now = *h.settlement(t, 1).ChallengeEnd
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []proto.Message{&types.EventTotalsUpdated{Totals: h.totals()}, &types.EventTransferRefunded{ID: h.id(1), Sender: h.sender.String(), Amount: coin(60)}}, sinkEvents(t, res.Events))
	require.NoError(t, k.CheckBalance(h.ctx()))
}

func TestSinkMintAndConflictEvents(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)

	for i := 0; i < 3; i++ {
		res = h.report(t, i, 1, types.OutcomeMinted)
		require.Zero(t, res.Code, res.Log)
	}

	require.Contains(t, sinkEvents(t, res.Events), proto.Message(&types.EventMintConfirmed{ID: h.id(1)}))
	res = h.report(t, 0, 1, types.OutcomeVoided)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []proto.Message{&types.EventReportConflict{ID: h.id(1), Reporter: h.reporterAddress(0)}, &types.EventOutcomeReported{ID: h.id(1), Reporter: h.reporterAddress(0), Outcome: types.OutcomeVoided}}, sinkEvents(t, res.Events))
}
