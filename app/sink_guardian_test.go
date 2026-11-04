package app_test

import (
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestSinkGuardianPause(t *testing.T) {
	h := reportingHarness(t)
	guardian := h.reporterAddress(4)
	pause := func(deposits, refunds bool) {
		res := h.deliverBytes(t, h.txFromMode(t, h.reporters[4], signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, "", &types.MsgGuardianPause{
			Guardian: guardian,
			Deposits: deposits,
			Refunds:  refunds,
		}))
		require.Zero(t, res.Code, res.Log)
	}

	res := h.deliverBytes(t, h.txFrom(t, h.reporters[4], "", &types.MsgGuardianPause{
		Guardian: guardian,
		Deposits: true,
	}))
	require.NotZero(t, res.Code)
	p := h.params()
	p.PauseGuardian = guardian
	require.NoError(t, h.update(p))
	version := h.policy().Reporters[0].Version

	res = h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.OutcomeVoided)
	h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	res = h.deliver(t, "", &types.MsgGuardianPause{
		Guardian: guardian,
		Deposits: true,
	})
	require.NotZero(t, res.Code)
	res = h.deliverBytes(t, h.txFrom(t, h.reporters[4], "", &types.MsgGuardianPause{Guardian: guardian}))
	require.NotZero(t, res.Code)
	pause(true, false)
	require.False(t, h.params().Enabled)
	require.True(t, h.params().RefundsEnabled)
	require.Contains(t, string(h.signBytes), `"type":"sink/MsgGuardianPause"`)
	res = h.deliver(t, "", h.deposit(10))
	require.NotZero(t, res.Code)
	pause(false, true)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	pause(true, true)

	require.Equal(t, version, h.policy().Reporters[0].Version)
	require.NoError(t, h.update(p))
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	p.PauseGuardian = ""
	require.NoError(t, h.update(p))
	res = h.deliverBytes(t, h.txFrom(t, h.reporters[4], "", &types.MsgGuardianPause{
		Guardian: guardian,
		Deposits: true,
	}))
	require.NotZero(t, res.Code)
	h.checkSettlementExport(t)
}
