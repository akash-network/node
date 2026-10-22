package app_test

import (
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

func eventAttributes(res *abci.ExecTxResult, name string) map[string]string {
	for _, event := range res.Events {
		if event.Type == name {
			attrs := map[string]string{}
			for _, attr := range event.Attributes {
				attrs[attr.Key] = attr.Value
			}
			return attrs
		}
	}
	return nil
}

func TestSinkGuardianPause(t *testing.T) {
	h := newSinkHarness(t, func(g *types.GenesisState) {
		g.Params.MaxTransfer = 100
		g.Params.DailyRefundCap = 100
		g.Params.ChallengePeriodSeconds = types.DefaultChallengePeriodSeconds
		g.Params.RefundsEnabled = true
	})
	k := h.a.Keepers.Akash.Sink
	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	server := sinkkeeper.MsgServer{Keeper: k}
	guardianKey := h.reporters[4] // funded, not a reporter
	guardian := sdk.AccAddress(guardianKey.PubKey().Address()).String()
	other := h.sender.String()
	pause := func(signer string, deposits, refunds bool) *abci.ExecTxResult {
		key := h.key
		if signer == guardian {
			key = guardianKey
		}
		return h.deliverBytes(t, h.txFrom(t, key, "", &types.MsgGuardianPause{Guardian: signer, Deposits: deposits, Refunds: refunds}))
	}

	// No guardian is configured by default, so nobody can pause.
	require.Empty(t, k.GetParams(h.ctx()).PauseGuardian)
	res := pause(guardian, true, false)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "not the sink pause guardian")
	require.True(t, k.GetParams(h.ctx()).Enabled)

	// Only governance sets the guardian, and it must be a valid account.
	p := k.GetParams(h.ctx())
	p.PauseGuardian = "akash1invalid"
	_, err := server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: gov, Params: p})
	require.ErrorContains(t, err, "pause guardian")
	p.PauseGuardian = guardian
	_, err = server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: guardian, Params: p})
	require.ErrorContains(t, err, "authority must be governance")
	_, err = server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)
	configured := k.GetParams(h.ctx())
	epoch := k.GetTotals(h.ctx()).ReporterEpoch

	// Set up a void that is ready to refund.
	res = h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.Outcome_OUTCOME_VOIDED)
	h.now = time.Unix(h.settlement(t, 1).ChallengeEnd, 0)

	// Another account cannot pause, even naming itself.
	res = pause(other, true, true)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "not the sink pause guardian")
	// Naming the guardian without its signature fails authentication.
	res = h.deliverBytes(t, h.tx(t, "", &types.MsgGuardianPause{Guardian: guardian, Deposits: true}))
	require.NotZero(t, res.Code)
	// A request with no switch fails before fees are charged.
	balance := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sdk.MustAccAddressFromBech32(guardian))
	res = pause(guardian, false, false)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "deposits, refunds or both")
	require.Equal(t, balance, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sdk.MustAccAddressFromBech32(guardian)))
	require.Equal(t, configured, k.GetParams(h.ctx()))

	// Deposits only, signed with legacy Amino JSON as a hardware wallet would.
	msg := &types.MsgGuardianPause{Guardian: guardian, Deposits: true}
	bz := h.txFromMode(t, guardianKey, signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, "", msg)
	require.Contains(t, string(h.signBytes), `"type":"sink/MsgGuardianPause"`)
	res = h.deliverBytes(t, bz)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, map[string]string{"guardian": guardian, "deposits": "true", "refunds": "false", "enabled": "false",
		"refunds_enabled": "true", "msg_index": "0"}, eventAttributes(res, "sink_guardian_pause"))
	want := configured
	want.Enabled = false
	require.Equal(t, want, k.GetParams(h.ctx())) // nothing else changes
	res = h.deliver(t, "", h.deposit(10))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "deposits disabled")

	// Refunds next; a repeated pause is a no-op, never a toggle.
	res = pause(guardian, false, true)
	require.Zero(t, res.Code, res.Log)
	want.RefundsEnabled = false
	require.Equal(t, want, k.GetParams(h.ctx()))
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "refunds disabled")
	res = pause(guardian, true, true)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, want, k.GetParams(h.ctx()))
	require.Equal(t, epoch, k.GetTotals(h.ctx()).ReporterEpoch) // the reporter set is untouched
	h.checkSettlementExport(t)

	// Only governance enables again; the guardian can pause again afterwards.
	_, err = server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: gov, Params: configured})
	require.NoError(t, err)
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	res = pause(guardian, true, true)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, want, k.GetParams(h.ctx()))

	// Governance can replace or remove the guardian.
	p = configured
	p.PauseGuardian = other
	_, err = server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)
	res = pause(guardian, true, false)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "not the sink pause guardian")
	res = pause(other, true, false)
	require.Zero(t, res.Code, res.Log)
	p.PauseGuardian = ""
	_, err = server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)
	res = pause(other, false, true)
	require.NotZero(t, res.Code)
	require.True(t, k.GetParams(h.ctx()).RefundsEnabled)
	require.ErrorContains(t, k.GuardianPause(h.ctx(), nil), "nil guardian pause")
}
