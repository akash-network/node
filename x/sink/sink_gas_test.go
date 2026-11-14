package sink_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	sinktypes "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/config"
	reportertx "pkg.akt.dev/node/v2/operator/reporter/tx"
)

// simGasLimit and simFee are the gas limit and fee the reporter simulates with under its defaults: gas = auto,
// tx.max-gas and gas prices in uakt.
const simGasLimit = 20_000_000

var simFee = sdk.NewCoins(coin(1))

// gasProbe builds reporter transactions with the reporter's real builder on a 3-member, 0.75-threshold policy, as
// in the local stack, where every member must vote.
type gasProbe struct {
	h        *testSuite
	builders []*reportertx.Builder
	prices   sdk.DecCoins
}

func newGasProbe(t *testing.T) *gasProbe {
	t.Helper()
	h := setupTestSuite(t, func(p *sinktypes.Params) {
		p.MaxAmountPerTransfer = coin(100)
		p.MaxEpochRefund = coin(100)
		p.RefundsEnabled = true
		p.Reporters = p.Reporters[:3]
	})
	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)

	g := &gasProbe{h: h, prices: sdk.NewDecCoins(sdk.NewDecCoinFromDec(sdkutil.DenomUakt, sdkmath.LegacyMustNewDecFromStr("0.025")))}
	for i := 0; i < 3; i++ {
		account := h.a.Keepers.Cosmos.Acct.GetAccount(h.ctx(), sdk.AccAddress(h.reporters[i].PubKey().Address()))
		g.builders = append(g.builders, reportertx.NewBuilder(h.a.TxConfig(), h.a.ChainID(), account.GetAccountNumber(), keySigner{key: h.reporters[i]}))
	}

	return g
}

// prepared is a reporter transaction simulated on the latest committed state and signed with the limit the
// broadcaster computes, ceil(simulated × adjustment), priced as the broadcaster prices it.
type prepared struct {
	simulated uint64
	limit     uint64
	bz        []byte
}

func (g *gasProbe) prepare(t *testing.T, reporter int, adjustment float64, msg sdk.Msg) prepared {
	t.Helper()
	h, b := g.h, g.builders[reporter]
	timeout := h.now.Add(time.Minute)

	sim, err := b.SimBytes([]sdk.Msg{msg}, simGasLimit, simFee, timeout)
	require.NoError(t, err)
	info, _, err := h.a.Simulate(sim)
	require.NoError(t, err)

	adj := sdkmath.LegacyMustNewDecFromStr(strconv.FormatFloat(adjustment, 'f', -1, 64))
	limit := sdkmath.LegacyNewDecFromInt(sdkmath.NewIntFromUint64(info.GasUsed)).Mul(adj).Ceil().TruncateInt().Uint64()
	bz, _, err := b.Build(t.Context(), []sdk.Msg{msg}, limit, reportertx.FeeFor(limit, g.prices, nil), timeout)
	require.NoError(t, err)

	return prepared{simulated: info.GasUsed, limit: limit, bz: bz}
}

// quorum simulates every member's vote on the same state, then delivers them in one block in member order, so the
// last vote completes the quorum on a state its simulation did not see.
func (g *gasProbe) quorum(t *testing.T, adjustment float64, outcome sinktypes.Outcome) ([]prepared, []*abci.ExecTxResult) {
	t.Helper()
	ps := make([]prepared, 0, 3)
	txs := make([][]byte, 0, 3)
	for i := 0; i < 3; i++ {
		p := g.prepare(t, i, adjustment, g.h.reportMsg(t, i, 1, outcome))
		ps = append(ps, p)
		txs = append(txs, p.bz)
	}

	return ps, g.h.commitBlock(t, g.h.now.Add(time.Second), txs...)
}

func logRatio(t *testing.T, name string, p prepared, res *abci.ExecTxResult) {
	t.Helper()
	require.Zero(t, res.Code, "%s: %s", name, res.Log)
	t.Logf("%-34s simulated %6d delivered %6d ratio %.3f", name, p.simulated, res.GasUsed, float64(res.GasUsed)/float64(p.simulated))
}

// TestReporterGasQuorumVote checks that the default gas-adjustment covers a vote simulated before the other
// members' votes landed in the same block, including the vote that completes the quorum (spec §6.4).
func TestReporterGasQuorumVote(t *testing.T) {
	adjustment := config.Default().GasAdjustment

	for _, outcome := range []sinktypes.Outcome{sinktypes.OutcomeMinted, sinktypes.OutcomeVoided} {
		t.Run(outcome.String(), func(t *testing.T) {
			g := newGasProbe(t)
			ps, res := g.quorum(t, adjustment, outcome)
			for i, r := range res {
				require.Zero(t, r.Code, "vote %d: %s", i, r.Log)
				require.LessOrEqual(t, r.GasUsed, int64(ps[i].limit)) //nolint:gosec // gas limit
			}

			want := sinktypes.StatusMinted
			if outcome == sinktypes.OutcomeVoided {
				want = sinktypes.StatusRefundPending
			}
			require.Equal(t, want, g.h.transfer(t, 1).Status)
		})
	}
}

// TestReporterGasMeasure logs delivered/simulated gas for the reporter's transactions. It only measures.
func TestReporterGasMeasure(t *testing.T) {
	if os.Getenv("SINK_GAS_MEASURE") == "" {
		t.Skip("set SINK_GAS_MEASURE=1 to measure")
	}
	// A generous limit, so every transaction runs to completion and reports the gas it needs.
	const adj = 5.0

	t.Run("lone vote", func(t *testing.T) {
		g := newGasProbe(t)
		p := g.prepare(t, 0, adj, g.h.reportMsg(t, 0, 1, sinktypes.OutcomeMinted))
		logRatio(t, "1 lone vote", p, g.h.deliverBytes(t, p.bz))
	})

	t.Run("votes in one block", func(t *testing.T) {
		for _, outcome := range []sinktypes.Outcome{sinktypes.OutcomeMinted, sinktypes.OutcomeVoided} {
			g := newGasProbe(t)
			ps, res := g.quorum(t, adj, outcome)
			for i := range ps {
				logRatio(t, fmt.Sprintf("2/3 %s vote %d of block", outcome, i+1), ps[i], res[i])
			}
		}
	})

	t.Run("votes in consecutive blocks", func(t *testing.T) {
		g := newGasProbe(t)
		ps := make([]prepared, 0, 3)
		for i := 0; i < 3; i++ {
			ps = append(ps, g.prepare(t, i, adj, g.h.reportMsg(t, i, 1, sinktypes.OutcomeMinted)))
		}
		for i := range ps {
			logRatio(t, fmt.Sprintf("2/3 vote %d, one per block", i+1), ps[i], g.h.deliverBytes(t, ps[i].bz))
		}
	})

	t.Run("veto and refund", func(t *testing.T) {
		g := newGasProbe(t)
		g.quorum(t, adj, sinktypes.OutcomeVoided)
		veto := g.prepare(t, 1, adj, &sinktypes.MsgVetoVoid{Signer: g.h.reporterAddress(1), ID: g.h.id(1), Reason: "No Solana status at finalized slot 1234"})
		logRatio(t, "4 veto", veto, g.h.deliverBytes(t, veto.bz))

		g = newGasProbe(t)
		g.quorum(t, adj, sinktypes.OutcomeVoided)
		g.h.commitBlock(t, time.Unix(g.h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC())
		refund := g.prepare(t, 0, adj, &sinktypes.MsgFinalizeRefund{Executor: g.h.reporterAddress(0), ID: g.h.id(1)})
		logRatio(t, "4 refund", refund, g.h.deliverBytes(t, refund.bz))
		require.Equal(t, sinktypes.StatusRefunded, g.h.transfer(t, 1).Status)
	})
}
