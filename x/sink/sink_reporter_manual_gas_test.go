package sink_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/stretchr/testify/require"

	sinktypes "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	reportertx "pkg.akt.dev/node/v2/operator/reporter/tx"
)

// nativeReporterChain uses the real SDK service and BaseApp. It supplies no RPC server or consensus network.
// The mutex serializes access to the test app, including commits made by BroadcastSync.
type nativeReporterChain struct {
	mu        sync.Mutex
	h         *testSuite
	service   txtypes.ServiceServer
	policy    sinktypes.ReporterPolicy
	sims      []int
	simErrors []error
	delivered []int
	fees      sdk.Coins
	receipts  map[string]*akash.TxResult
	simGas    []uint64
	simFees   []sdk.Coins
	signed    [][]byte
}

func (c *nativeReporterChain) Latest(context.Context) (*akash.State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &akash.State{
		Height: c.h.height,
		Time:   c.h.now,
		Policy: c.policy,
	}, nil
}

func (c *nativeReporterChain) Simulate(ctx context.Context, raw []byte) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tx, err := c.h.a.TxConfig().TxDecoder()(raw)
	if err != nil {
		return 0, err
	}
	c.sims = append(c.sims, len(tx.GetMsgs()))
	c.simGas = append(c.simGas, tx.(sdk.FeeTx).GetGas())
	c.simFees = append(c.simFees, tx.(sdk.FeeTx).GetFee())
	res, err := c.service.Simulate(ctx, &txtypes.SimulateRequest{TxBytes: raw})
	c.simErrors = append(c.simErrors, err)
	if err != nil {
		return 0, err
	}
	return res.GasInfo.GasUsed, nil
}

func (c *nativeReporterChain) BroadcastSync(_ context.Context, raw []byte) (akash.BroadcastResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.signed = append(c.signed, append([]byte(nil), raw...))
	hash := sha256.Sum256(raw)
	check, err := c.h.a.CheckTx(&abci.RequestCheckTx{Tx: raw})
	if err != nil {
		return akash.BroadcastResult{}, err
	}
	result := akash.BroadcastResult{
		Code:      check.Code,
		Codespace: check.Codespace,
		Log:       check.Log,
		Hash:      hash[:],
	}
	if check.Code != 0 {
		return result, nil
	}
	c.h.height++
	c.h.now = c.h.now.Add(time.Second)
	block, err := c.h.a.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: c.h.height,
		Time:   c.h.now,
		Txs:    [][]byte{raw},
	})
	if err != nil {
		return result, err
	}
	if _, err := c.h.a.Commit(); err != nil {
		return result, err
	}
	delivered := block.TxResults[0]
	c.receipts[string(hash[:])] = &akash.TxResult{
		Height:    c.h.height,
		Code:      delivered.Code,
		Codespace: delivered.Codespace,
		Log:       delivered.Log,
	}
	tx, err := c.h.a.TxConfig().TxDecoder()(raw)
	if err != nil {
		return result, err
	}
	if delivered.Code == 0 {
		c.delivered = append(c.delivered, len(tx.GetMsgs()))
	}
	c.fees = c.fees.Add(tx.(sdk.FeeTx).GetFee()...)
	return result, nil
}

func (c *nativeReporterChain) Tx(_ context.Context, hash []byte) (*akash.TxResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.receipts[string(hash)], nil
}

func (*nativeReporterChain) TxIndexEnabled() bool { return true }

func (c *nativeReporterChain) nextWindow(t *testing.T) (int64, time.Time) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.h.commitBlock(t, reportertx.WindowStart(reportertx.WindowOf(c.h.now)+1).Add(30*time.Second))
	return c.h.height, c.h.now
}

type nativeCountingSigner struct {
	keySigner
	calls atomic.Int64
}

func (s *nativeCountingSigner) Sign(ctx context.Context, raw []byte) ([]byte, error) {
	s.calls.Add(1)
	return s.keySigner.Sign(ctx, raw)
}

func nativeSameSlotPair(t *testing.T, h *testSuite) []policy.Planned {
	t.Helper()
	slots := map[int]policy.Planned{}
	for i := 1; i <= reportertx.VoteSlots+1; i++ {
		result := h.deliver(t, "", h.deposit(1))
		require.Zero(t, result.Code, result.Log)
		msg := h.reportMsg(t, 0, i, sinktypes.OutcomeMinted)
		id, err := sinkid.FromProto(msg.ID)
		require.NoError(t, err)
		p := policy.Planned{
			ID:  id,
			Msg: msg,
			Key: policy.KeyOf(id, msg),
		}
		slot := reportertx.SlotOf(id, p.Key)
		if previous, ok := slots[slot]; ok {
			return []policy.Planned{previous, p}
		}
		slots[slot] = p
	}
	t.Fatal("no colliding pair")
	return nil
}

func TestReporterNativeManualBatchCap(t *testing.T) {
	cases := []struct {
		name           string
		gas            uint64
		maxGas         uint64
		blockGas       int64
		maxFee         sdk.Coin
		fees           sdk.Coins
		single         bool
		indexedFailure bool
		split          bool
		wantSim        []int
		wantDelivered  []int
		wantFailures   int
	}{
		{
			name:          "manual block cap",
			gas:           200_000,
			maxGas:        20_000_000,
			blockGas:      300_000,
			split:         true,
			wantSim:       []int{1},
			wantDelivered: []int{1},
		},
		{
			name:          "ordinary manual",
			gas:           200_000,
			maxGas:        600_000,
			blockGas:      500_000,
			wantSim:       []int{2},
			wantDelivered: []int{2},
		},
		{
			name:          "auto block cap",
			maxGas:        20_000_000,
			blockGas:      300_000,
			wantSim:       []int{2},
			wantDelivered: []int{2},
		},
		{
			name:          "single manual",
			gas:           200_000,
			maxGas:        20_000_000,
			blockGas:      300_000,
			single:        true,
			wantSim:       []int{1},
			wantDelivered: []int{1},
		},
		{
			name:         "single impossible",
			gas:          400_000,
			maxGas:       20_000_000,
			blockGas:     300_000,
			single:       true,
			wantFailures: 1,
		},
		{
			name:          "configured cap",
			gas:           200_000,
			maxGas:        300_000,
			blockGas:      500_000,
			split:         true,
			wantSim:       []int{1},
			wantDelivered: []int{1},
		},
		{
			name:          "priced fee cap",
			gas:           200_000,
			maxGas:        600_000,
			blockGas:      500_000,
			maxFee:        sdk.NewInt64Coin("uakt", 5000),
			split:         true,
			wantSim:       []int{1},
			wantDelivered: []int{1},
		},
		{
			name:          "fixed fee",
			gas:           200_000,
			maxGas:        600_000,
			blockGas:      500_000,
			fees:          sdk.NewCoins(sdk.NewInt64Coin("uakt", 10)),
			wantSim:       []int{2},
			wantDelivered: []int{2},
		},
		{
			name:           "indexed invalid round",
			gas:            200_000,
			maxGas:         600_000,
			blockGas:       500_000,
			indexedFailure: true,
			wantSim:        []int{2, 1},
			wantDelivered:  []int{1},
			wantFailures:   1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := setupReportingTestSuite(t)
			pair := nativeSameSlotPair(t, h)
			if tc.single {
				pair = pair[:1]
			}
			cp := cmttypes.DefaultConsensusParams().ToProto()
			cp.Block.MaxGas = tc.blockGas
			require.NoError(t, h.a.StoreConsensusParams(h.ctx(), cp))
			h.commitBlock(t, reportertx.WindowStart(reportertx.WindowOf(h.now)+1).Add(30*time.Second))
			for i := range pair {
				pair[i].PlanHeight = h.height
				pair[i].PlanTime = h.now
			}
			s := &nativeCountingSigner{
				keySigner: keySigner{
					key: h.reporters[0],
				},
			}
			account := h.a.Keepers.Cosmos.Acct.GetAccount(h.ctx(), s.Address())
			builder := reportertx.NewBuilder(h.a.TxConfig(), h.a.ChainID(), account.GetAccountNumber(), s)
			c := &nativeReporterChain{
				h:        h,
				service:  authtx.NewTxServer(client.Context{}, h.a.Simulate, h.a.InterfaceRegistry()),
				policy:   h.policy().Reporters[0],
				receipts: map[string]*akash.TxResult{},
			}
			opts := reportertx.Options{
				Reporter:        s.Address().String(),
				MaxMsgs:         25,
				MaxInFlight:     1,
				GasPerMessage:   tc.gas,
				GasAdjustment:   2,
				GasPrices:       testNativePrices(),
				Fees:            tc.fees,
				MaxFee:          tc.maxFee,
				MaxGas:          tc.maxGas,
				BlockMaxGas:     tc.blockGas,
				RefreshInterval: time.Millisecond,
				ShutdownGrace:   time.Second,
			}
			var output bytes.Buffer
			ev := events.NewWriter(&output)
			b := reportertx.New(opts, c, builder, ev)
			timeout := reportertx.SlotTimeout(reportertx.WindowOf(h.now), reportertx.SlotOf(pair[0].ID, pair[0].Key))
			before := h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), s.Address(), "uakt").Amount

			// Prove that every selected real transfer is processable independently, before the deliberate invalid-round case.
			for _, p := range pair {
				raw, err := builder.SimBytes([]sdk.Msg{p.Msg}, 200_000, sdk.NewCoins(coin(5000)), timeout)
				require.NoError(t, err)
				sim, err := c.service.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: raw})
				require.NoError(t, err)
				require.Positive(t, sim.GasInfo.GasUsed)
				t.Logf("individually valid id=%s gas=%d", p.ID, sim.GasInfo.GasUsed)
			}
			if tc.name == "manual block cap" {
				raw, err := b.SimBytes([]sdk.Msg{pair[0].Msg, pair[1].Msg}, timeout)
				require.NoError(t, err)
				_, err = c.service.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: raw})
				require.ErrorContains(t, err, "tx gas limit 400000 exceeds block max gas 300000")
				require.NotContains(t, err.Error(), "message index:")
				t.Logf("native ante error=%v; failing simulation input hex=%x", err, raw)
			}
			require.Equal(t, before, h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), s.Address(), "uakt").Amount)
			if tc.indexedFailure {
				msg := pair[1].Msg.(*sinktypes.MsgReportOutcome)
				msg.Round++
				pair[1].Key = policy.KeyOf(pair[1].ID, msg)
				// Keep the original batch slot: choose an invalid round that still hashes to the same slot.
				for reportertx.SlotOf(pair[1].ID, pair[1].Key) != reportertx.SlotOf(pair[0].ID, pair[0].Key) {
					msg.Round++
					pair[1].Key = policy.KeyOf(pair[1].ID, msg)
					require.Less(t, msg.Round, uint64(100_000))
				}
			}

			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- b.Run(ctx) }()
			var once sync.Once
			stop := func() {
				once.Do(func() {
					cancel()
					require.NoError(t, <-done)
					require.NoError(t, ev.Close())
				})
			}
			t.Cleanup(stop)
			for _, p := range pair {
				require.True(t, b.Queue(p))
			}
			b.Flush()
			wait, stopWait := context.WithTimeout(t.Context(), time.Second)
			failures := b.Wait(wait)
			stopWait()
			c.mu.Lock()
			sims := append([]int(nil), c.sims...)
			simErrors := append([]error(nil), c.simErrors...)
			delivered := append([]int(nil), c.delivered...)
			c.mu.Unlock()
			t.Logf("first window inputs=%d simulations=%v errors=%v signatures=%d delivered=%v failures=%d", len(pair), sims, simErrors, s.calls.Load(), delivered, failures)
			if tc.name == "manual block cap" && failures != 0 {
				for _, p := range pair {
					require.True(t, b.Queue(p))
				}
				b.Flush()
				repeated, stopRepeat := context.WithTimeout(t.Context(), time.Second)
				t.Logf("subsequent pass failures=%d", b.Wait(repeated))
				stopRepeat()
			}
			require.Equal(t, tc.wantFailures, failures)
			require.Equal(t, tc.wantSim, sims)
			require.Equal(t, tc.wantDelivered, delivered)
			require.EqualValues(t, len(delivered), s.calls.Load())
			if tc.indexedFailure {
				require.ErrorContains(t, simErrors[0], "message index: 1")
			}

			if tc.split {
				require.False(t, b.Queue(pair[1]), "the remainder cannot reuse this window's nonce")
				remaining := pair[1]
				remaining.PlanHeight, remaining.PlanTime = c.nextWindow(t)
				require.True(t, b.Queue(remaining), "a fresh later-window plan resumes the retained transfer")
				b.Flush()
				wait, stopWait = context.WithTimeout(t.Context(), time.Second)
				require.Zero(t, b.Wait(wait))
				stopWait()
			}
			stop()
			t.Logf("final simulations=%v delivered=%v signatures=%d fees=%s events=%s", c.sims, c.delivered, s.calls.Load(), c.fees, output.String())
			wantVotes := len(pair)
			if tc.wantFailures != 0 {
				wantVotes--
			}
			votes := 0
			for _, p := range pair {
				transfer, err := h.a.Keepers.Akash.Sink.GetTransfer(h.ctx(), p.ID.Proto())
				require.NoError(t, err)
				found := 0
				for _, vote := range transfer.Settlement.Votes {
					if vote.Reporter == s.Address().String() {
						found++
					}
				}
				require.LessOrEqual(t, found, 1)
				votes += found
			}
			require.Equal(t, wantVotes, votes, "native state must contain every expected reporter vote exactly once")
			if tc.split {
				require.Equal(t, []int{1, 1}, c.sims)
				require.Equal(t, []int{1, 1}, c.delivered)
				require.EqualValues(t, 2, s.calls.Load())
			}
			for _, receipt := range c.receipts {
				require.Zero(t, receipt.Code, receipt.Log)
			}
			for _, raw := range c.signed {
				tx, err := h.a.TxConfig().TxDecoder()(raw)
				require.NoError(t, err)
				feeTx := tx.(sdk.FeeTx)
				require.LessOrEqual(t, feeTx.GetGas(), reportertx.EffectiveMaxGas(tc.maxGas, tc.blockGas))
				require.Equal(t, reportertx.FeeFor(feeTx.GetGas(), opts.GasPrices, opts.Fees), feeTx.GetFee())
				if !tc.maxFee.Amount.IsNil() {
					require.True(t, feeTx.GetFee().AmountOf("uakt").LTE(tc.maxFee.Amount))
				}
			}
			if tc.gas == 0 {
				for i, gas := range c.simGas {
					require.Zero(t, gas)
					require.Equal(t, sdk.NewCoins(coin(1)), c.simFees[i])
				}
			}
			after := h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), s.Address(), "uakt").Amount
			require.True(t, c.fees.AmountOf("uakt").Equal(before.Sub(after)), "simulation does not charge and delivery pays the decoded fees")
		})
	}
}

func testNativePrices() sdk.DecCoins {
	return sdk.NewDecCoins(sdk.NewDecCoinFromDec("uakt", sdkmath.LegacyMustNewDecFromStr("0.025")))
}
