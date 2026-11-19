package tx

import (
	"context"
	"math"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// capBuildChain records build's work without emulating native ante. Native coverage is in x/sink.
type capBuildChain struct {
	Chain
	state      *akash.State
	sim        func([]byte) (uint64, error)
	broadcasts [][]byte
}

func (c *capBuildChain) Latest(context.Context) (*akash.State, error)           { return c.state, nil }
func (c *capBuildChain) Simulate(_ context.Context, raw []byte) (uint64, error) { return c.sim(raw) }
func (c *capBuildChain) BroadcastSync(_ context.Context, raw []byte) (akash.BroadcastResult, error) {
	c.broadcasts = append(c.broadcasts, raw)
	return akash.BroadcastResult{}, nil
}

func TestManualGasKnownCaps(t *testing.T) {
	cases := []struct {
		name      string
		n         int
		gas       uint64
		maxGas    uint64
		blockGas  int64
		maxFee    sdk.Coin
		fixed     sdk.Coins
		simulated []int
		sent      int
		failed    int
		deferred  int
	}{
		{
			name:      "ordinary manual",
			n:         2,
			gas:       100_000,
			maxGas:    400_000,
			blockGas:  500_000,
			simulated: []int{2},
			sent:      2,
		},
		{
			name:      "block cap",
			n:         5,
			gas:       100_000,
			maxGas:    500_000,
			blockGas:  250_000,
			simulated: []int{2},
			sent:      2,
			deferred:  3,
		},
		{
			name:      "configured cap",
			n:         5,
			gas:       100_000,
			maxGas:    250_000,
			blockGas:  500_000,
			simulated: []int{2},
			sent:      2,
			deferred:  3,
		},
		{
			name:      "unlimited block",
			n:         2,
			gas:       100_000,
			maxGas:    400_000,
			blockGas:  -1,
			simulated: []int{2},
			sent:      2,
		},
		{
			name:      "priced fee cap",
			n:         5,
			gas:       100_000,
			maxGas:    1_000_000,
			maxFee:    sdk.NewInt64Coin("uakt", 5000),
			simulated: []int{2},
			sent:      2,
			deferred:  3,
		},
		{
			name:     "single block over cap",
			n:        1,
			gas:      200_000,
			maxGas:   500_000,
			blockGas: 150_000,
			failed:   1,
		},
		{
			name:     "single config over cap",
			n:        1,
			gas:      200_000,
			maxGas:   150_000,
			blockGas: 500_000,
			failed:   1,
		},
		{
			name:     "fixed fee over cap",
			n:        2,
			gas:      100_000,
			maxGas:   500_000,
			maxFee:   sdk.NewInt64Coin("uakt", 5),
			fixed:    sdk.NewCoins(sdk.NewInt64Coin("uakt", 6)),
			failed:   1,
			deferred: 1,
		},
		{
			name:     "auto fixed fee over cap",
			n:        2,
			maxGas:   500_000,
			maxFee:   sdk.NewInt64Coin("uakt", 5),
			fixed:    sdk.NewCoins(sdk.NewInt64Coin("uakt", 6)),
			failed:   1,
			deferred: 1,
		},
		{
			name:      "auto retains simulation shape",
			n:         2,
			maxGas:    500_000,
			blockGas:  100_000,
			simulated: []int{2},
			sent:      2,
		},
		{
			name:      "multiplication overflow",
			n:         2,
			gas:       math.MaxUint64/2 + 1,
			maxGas:    math.MaxUint64,
			simulated: []int{1},
			sent:      1,
			deferred:  1,
		},
		{
			name:      "multiplication boundary",
			n:         2,
			gas:       math.MaxUint64 / 2,
			maxGas:    math.MaxUint64,
			simulated: []int{2},
			sent:      2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &recordingSigner{Signer: reporterSigner(t)}
			st := &akash.State{
				Height: 42,
				Time:   start,
				Policy: sinkv1.ReporterPolicy{
					Members: []string{s.Address().String()},
				},
			}
			batch := make([]policy.Planned, tc.n)
			for i := range batch {
				msg := voteMsg(s).(*sinkv1.MsgReportOutcome)
				msg.ID.Sequence = uint64(i + 1)
				id, err := sinkid.FromProto(msg.ID)
				require.NoError(t, err)
				batch[i] = plannedAt(st, id, msg)
			}
			opts := Options{
				Reporter:      s.Address().String(),
				GasPerMessage: tc.gas,
				GasPrices:     testPrices,
				Fees:          tc.fixed,
				MaxGas:        tc.maxGas,
				BlockMaxGas:   tc.blockGas,
				MaxFee:        tc.maxFee,
			}
			var simulated []int
			chain := &capBuildChain{state: st}
			chain.sim = func(raw []byte) (uint64, error) {
				_, body, auth := decodeRaw(t, raw)
				simulated = append(simulated, len(body.Messages))
				if tc.gas == 0 {
					require.Zero(t, auth.Fee.GasLimit)
					require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 1)), auth.Fee.Amount)
				} else {
					require.Equal(t, tc.gas*uint64(len(body.Messages)), auth.Fee.GasLimit)
				}
				return 1, nil
			}
			b := New(opts, chain, NewBuilder(testTxConfig(), testChainID, testAccountNumber, s), nil)
			b.stop, b.work = t.Context(), t.Context()
			logger := zerolog.Nop()
			b.log = &logger
			result := b.build(buildJob{
				msgs:      batch,
				timeout:   start.Add(time.Minute),
				chainTime: start,
			})
			require.Equal(t, tc.simulated, simulated)
			require.Len(t, result.failed, tc.failed)
			require.Len(t, result.deferred, tc.deferred)
			counts := map[policy.Key]int{}
			for _, f := range result.failed {
				counts[f.planned.Key]++
			}
			for _, d := range result.deferred {
				require.Equal(t, reasonCap, d.reason)
				counts[d.planned.Key]++
			}
			require.Empty(t, result.released)
			if tc.sent == 0 {
				require.Nil(t, result.sent)
				require.Empty(t, chain.broadcasts)
				require.Empty(t, s.got)
			} else {
				require.NotNil(t, result.sent)
				require.Len(t, result.sent.msgs, tc.sent)
				require.Len(t, chain.broadcasts, 1)
				require.Len(t, s.got, 1)
				for _, p := range result.sent.msgs {
					counts[p.Key]++
				}
			}
			require.Len(t, counts, len(batch))
			for _, p := range batch {
				require.Equal(t, 1, counts[p.Key], "every input is sent, failed or retained exactly once")
			}
		})
	}
}
