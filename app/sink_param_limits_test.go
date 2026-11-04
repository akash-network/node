package app_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	sinktypes "pkg.akt.dev/go/node/sink/v1"
)

func TestSinkIndependentSendingCaps(t *testing.T) {
	for name, tc := range map[string]struct {
		account int64
		total   int64
		code    uint32
	}{
		"account below transfer maximum":   {5, 100, sinktypes.ErrAccountDailyAmountExceeded.ABCICode()},
		"aggregate with unlimited account": {0, 5, sinktypes.ErrSendCapReached.ABCICode()},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSinkHarness(t, func(g *sinktypes.GenesisState) {
				g.Params.MaxAmountPerTransfer = coin(10)
				g.Params.MaxEpochAmountPerAccount = coin(tc.account)
				g.Params.MaxTransferAmountPerEpoch = coin(tc.total)
				g.Params.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.6")
			})
			before := h.totals()
			res := h.deliver(t, "", h.deposit(6))
			require.Equal(t, tc.code, res.Code, res.Log)
			require.Equal(t, before, h.totals())
			require.True(t, sdkmath.LegacyMustNewDecFromStr("0.6").Equal(h.policy().Reporters[0].Threshold))

			res = h.deliver(t, "", h.deposit(5))
			require.Zero(t, res.Code, res.Log)
			require.Equal(t, uint64(1), h.id(1).Sequence)
			require.Equal(t, coin(5), h.usage(h.sender).EpochSent)
			require.Equal(t, coin(5), h.state().SendUsage.Amount)

			before = h.totals()
			res = h.deliver(t, "", h.deposit(1))
			require.Equal(t, tc.code, res.Code, res.Log)
			require.Equal(t, before, h.totals())
			require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
		})
	}
}
