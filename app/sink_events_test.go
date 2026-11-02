package app_test

import (
	"reflect"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
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

// A void, veto, governance confirmation and refund each emit their typed event
// with the values an indexer needs.
func TestSinkVoidAndRefundEvents(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)

	ctx := h.ctx()
	require.NoError(t, k.RequestVoid(ctx, gov, 1))
	require.Equal(t, []proto.Message{&types.EventVoidOrdered{ID: 1}}, sinkEvents(t, ctx.EventManager().ABCIEvents()))

	for i := 0; i < 2; i++ {
		res = h.report(t, i, 1, types.Outcome_OUTCOME_VOIDED)
		require.Zero(t, res.Code, res.Log)
		require.Equal(t, []proto.Message{
			&types.EventOutcomeReported{ID: 1, Reporter: h.reporterAddress(i), Outcome: types.Outcome_OUTCOME_VOIDED},
		}, sinkEvents(t, res.Events))
	}
	res = h.report(t, 2, 1, types.Outcome_OUTCOME_VOIDED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []proto.Message{
		&types.EventVoidConfirmed{ID: 1, ChallengeEnd: h.settlement(t, 1).ChallengeEnd},
		&types.EventOutcomeReported{ID: 1, Reporter: h.reporterAddress(2), Outcome: types.Outcome_OUTCOME_VOIDED},
	}, sinkEvents(t, res.Events))

	veto := &types.MsgVetoVoid{Signer: h.reporterAddress(3), Id: 1, Reason: "node does not show a finalized void"}
	res = h.deliverBytes(t, h.txFrom(t, h.reporters[3], "", veto))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []proto.Message{
		&types.EventVoidVetoed{ID: 1, Signer: veto.Signer, Reason: veto.Reason},
	}, sinkEvents(t, res.Events))

	evidence := observation(types.Outcome_OUTCOME_VOIDED)
	resolve := &types.MsgResolveTransfer{Authority: gov, Id: 1, Decision: types.Decision_DECISION_CONFIRM_VOID,
		Observation: &evidence, Reason: "local governance review"}
	result, err := h.a.MsgServiceRouter().Handler(resolve)(h.ctx(), resolve)
	require.NoError(t, err)
	require.Equal(t, []proto.Message{
		&types.EventTransferResolved{ID: 1, Decision: resolve.Decision, Reason: resolve.Reason},
	}, sinkEvents(t, result.Events))

	h.now = time.Unix(h.settlement(t, 1).ChallengeEnd, 0)
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []proto.Message{
		&types.EventTransferRefunded{ID: 1, Sender: h.sender.String(), Amount: sdk.NewInt64Coin(types.Denom, 60)},
	}, sinkEvents(t, res.Events))
}

// A mint quorum emits the confirmation before the report that reached it, and
// a contradicting report emits the conflict.
func TestSinkMintAndConflictEvents(t *testing.T) {
	h := reportingHarness(t)

	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	for i := 0; i < 2; i++ {
		res = h.report(t, i, 1, types.Outcome_OUTCOME_MINTED)
		require.Zero(t, res.Code, res.Log)
	}
	res = h.report(t, 2, 1, types.Outcome_OUTCOME_MINTED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []proto.Message{
		&types.EventMintConfirmed{ID: 1},
		&types.EventOutcomeReported{ID: 1, Reporter: h.reporterAddress(2), Outcome: types.Outcome_OUTCOME_MINTED},
	}, sinkEvents(t, res.Events))

	res = h.report(t, 3, 1, types.Outcome_OUTCOME_VOIDED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []proto.Message{
		&types.EventReportConflict{ID: 1, Reporter: h.reporterAddress(3)},
		&types.EventOutcomeReported{ID: 1, Reporter: h.reporterAddress(3), Outcome: types.Outcome_OUTCOME_VOIDED},
	}, sinkEvents(t, res.Events))
}
