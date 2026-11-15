package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/header"
)

func TestHeaderFailureEventsPreserveBaselineClassification(t *testing.T) {
	raw, err := os.ReadFile("header/testdata/template.json")
	require.NoError(t, err)
	template, err := header.ParseSource(raw)
	require.NoError(t, err)

	tests := []struct {
		name        string
		signers     int
		limit       int
		trust       *header.Fraction
		mutate      func(*header.Source)
		unsupported bool
		reason      string
	}{
		{
			name:        "insufficient current voting power",
			signers:     2,
			limit:       100,
			unsupported: true,
			reason:      "signers hold 20000 of 40000 voting power; the verifier needs more than 2/3",
		},
		{
			name:        "signature count limit",
			signers:     4,
			limit:       2,
			unsupported: true,
			reason:      "a quorum needs 3 of 4 signatures; at most 2 fit one header update",
		},
		{
			name:    "insufficient trusted voting power",
			signers: 3,
			limit:   100,
			trust: &header.Fraction{
				Numerator:   999,
				Denominator: 1000,
			},
			unsupported: true,
			reason:      "signers hold 30000 of 40000 trusted voting power; the verifier needs more than 999/1000",
		},
		{
			name:    "encoder round overflow",
			signers: 4,
			mutate: func(source *header.Source) {
				source.Signed.Commit.Round = 65536
			},
		},
		{
			name:    "encoder key type",
			signers: 4,
			mutate: func(source *header.Source) {
				source.Validators[0].PubKey = secp256k1.PubKey(make([]byte, 33))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source, err := header.Fixture(template, 4, tc.signers, false)
			require.NoError(t, err)

			var failure error
			if tc.mutate != nil {
				tc.mutate(&source)
				_, failure = header.Encode(source)
			} else {
				_, _, failure = header.Select(source, header.Selection{
					Trigger: 0,
					Limit:   tc.limit,
					Trust:   tc.trust,
				})
			}
			require.Error(t, failure)

			now := time.Unix(100, 0)
			var output bytes.Buffer
			s := &service{
				cfg: config{
					retryMaxSeconds: 900,
				},
				deps: dependencies{
					now: func() time.Time { return now },
					streams: Streams{
						Err: &output,
					},
				},
			}
			updated, err := s.headerStep(func() error { return fmt.Errorf("%w", failure) }, 29)
			require.NoError(t, err)
			require.False(t, updated)

			var event map[string]any
			require.NoError(t, json.Unmarshal(output.Bytes(), &event))
			require.Equal(t, float64(29), event["target"])
			if tc.unsupported {
				require.Equal(t, "header-unsupported", event["event"])
				require.Equal(t, tc.reason, event["reason"])
				require.Equal(t, float64(900), event["retrySeconds"])
				require.Equal(t, now.UnixMilli()+900000, s.unsupported.RetryAt)
				require.Zero(t, s.headerRetry.Failures)
			} else {
				require.Equal(t, "header-retry", event["event"])
				require.Equal(t, failure.Error(), event["error"])
				require.Equal(t, float64(6), event["retrySeconds"])
				require.Equal(t, now.UnixMilli()+6000, s.headerRetry.RetryAt)
				require.Zero(t, s.unsupported.Failures)
			}
		})
	}
}

func TestResolutionTwoPreparationSlotsSequentialPayments(t *testing.T) {
	h := newResolutionHarness(t)
	order := []uint64{3, 1, 4, 2}
	plans := make([]relayPlan, 0, len(order))
	for _, sequence := range order {
		transfer := planningTransfer(sequence)
		h.proof(transfer, 0)
		plans = append(plans, relayPlan{
			id:           transfer.ID,
			transfer:     transfer,
			height:       20,
			resumeHeight: 20,
		})
	}
	var mu sync.Mutex
	active, maximum, started := 0, 0, 0
	gates := []chan struct{}{make(chan struct{}), make(chan struct{})}
	h.prepareGate = func(id sink.ID) {
		mu.Lock()
		wave := started / 2
		started++
		active++
		maximum = max(maximum, active)
		if started%2 == 0 {
			close(gates[wave])
		}
		mu.Unlock()
		select {
		case <-gates[wave]:
		case <-time.After(2 * time.Second):
			t.Error("proof preparations did not overlap")
		}
		mu.Lock()
		active--
		mu.Unlock()
	}
	require.NoError(t, h.s.relayCovered(context.Background(), plans, func(context.Context) (relayHeader, error) {
		return relayHeader{
			followed: h.followed,
			height:   20,
		}, nil
	}, func() {}))
	require.Equal(t, 2, maximum)
	require.Equal(t, 4, started)
	var paidIDs []string
	decoder := json.NewDecoder(&h.out)
	for decoder.More() {
		var event map[string]any
		require.NoError(t, decoder.Decode(&event))
		if event["event"] == "transfer-resolved" {
			paidIDs = append(paidIDs, event["id"].(string))
		}
	}
	var want []string
	for _, plan := range plans {
		want = append(want, plan.id.String())
	}
	require.Equal(t, want, paidIDs)
	require.Equal(t, 4, h.calls["/store/sink/key"])
	ledger, err := h.s.store.readLedger()
	require.NoError(t, err)
	for _, plan := range plans {
		require.True(t, *ledger.entries[plan.id].Completed)
		require.Nil(t, ledger.entries[plan.id].Retry)
	}
}

func TestResolutionHeaderBackoffAndResumeSemantics(t *testing.T) {
	h := newResolutionHarness(t)
	called := 0
	failure := func() error {
		called++
		return errors.New("update rejected")
	}
	for _, delay := range []int64{6000, 12000, 24000, 48000, 96000, 120000, 120000} {
		updated, err := h.s.headerStep(failure, 25)
		require.NoError(t, err)
		require.False(t, updated)
		require.Equal(t, h.now.UnixMilli()+delay, h.s.headerRetry.RetryAt)
		updated, err = h.s.headerStep(failure, 25)
		require.NoError(t, err)
		require.False(t, updated)
		h.now = h.now.Add(time.Duration(delay) * time.Millisecond)
	}
	require.Equal(t, 7, called)
	before := h.s.headerRetry
	updated, err := h.s.headerStep(func() error { return nil }, 0)
	require.NoError(t, err)
	require.True(t, updated)
	require.Equal(t, before, h.s.headerRetry, "resume/no job cannot clear update backoff")
	updated, err = h.s.headerStep(func() error { return fmt.Errorf("committee: %w", header.ErrUnsupported) }, 29)
	require.NoError(t, err)
	require.False(t, updated)
	require.Equal(t, h.now.UnixMilli()+900000, h.s.unsupported.RetryAt)
	require.Contains(t, h.errOut.String(), "header-unsupported")
	require.Contains(t, h.errOut.String(), "header-retry")
	h.s.unsupported = retryState{}
	h.s.cfg.retryMaxSeconds = maxSafeInteger
	updated, err = h.s.headerStep(func() error { return header.ErrUnsupported }, 29)
	require.NoError(t, err)
	require.False(t, updated)
	require.Equal(t, h.now.UnixMilli()+maxSafeInteger*1000, h.s.unsupported.RetryAt)
}

func TestResolutionTickActivityAndBudgetGuards(t *testing.T) {
	for _, mode := range []string{"empty", "frozen", "expired", "recovery-first", "client-outage"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			if mode == "frozen" || mode == "expired" {
				state, err := bridge.DecodeClient(h.accounts[h.followed.ClientState].Data.GetBinary())
				require.NoError(t, err)
				if mode == "frozen" {
					state.FrozenHeight.RevisionHeight = 1
				} else {
					state.TrustingPeriod = 0
				}
				h.accounts[h.followed.ClientState] = resolutionAccountData(t, h.followed.Program, "ClientState", state)
			}
			if mode == "client-outage" || mode == "recovery-first" {
				h.failAccount = h.followed.ClientState
			}
			if mode == "recovery-first" {
				proof := h.proof(planningTransfer(1), 0)
				_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
					return true, ledger.attempt(proof.Transfer.ID)
				})
				require.NoError(t, err)
				h.status(proof.Transfer.ID)
			}
			err := h.s.tick(context.Background())
			if mode == "empty" {
				require.NoError(t, err)
				require.Contains(t, h.out.String(), `"stage":"discovery"`)
			} else {
				require.Error(t, err)
				require.Zero(t, h.calls["/akash.sink.v1.Query/State"])
				if mode == "recovery-first" {
					ledger, err := h.s.store.readLedger()
					require.NoError(t, err)
					require.True(t, *ledger.entries[planningTransfer(1).ID].Completed)
				}
			}
			require.Equal(t, 1, h.calls["getGenesisHash"], "lookup cleanup must still check live genesis")
			require.Empty(t, h.stages)
			require.Contains(t, h.out.String(), `"stage":"tick"`)
		})
	}
}

func TestResolutionExhaustionAndRetryAlerts(t *testing.T) {
	h := newResolutionHarness(t)
	id := planningTransfer(1).ID
	_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
		return true, ledger.charge(id, h.s.cfg.maxLamports+1, "test-signature", 0)
	})
	require.NoError(t, err)
	ledger, err := h.s.store.readLedger()
	require.NoError(t, err)
	h.s.overBudget(context.Background(), id, ledger)
	first := h.errOut.String()
	h.s.overBudget(context.Background(), id, ledger)
	require.Equal(t, first, h.errOut.String())
	require.NoError(t, h.s.relayFailed(context.Background(), id, errors.New("proof failed")))
	ledger, err = h.s.store.readLedger()
	require.NoError(t, err)
	require.False(t, ledger.ready(id, h.now.UnixMilli()))
	require.Equal(t, h.now.UnixMilli()+6000, ledger.entries[id].Retry.RetryAt)
	require.Contains(t, h.errOut.String(), "relay-budget-exhausted")
	require.Contains(t, h.errOut.String(), "relay-retry")
}

func TestResolutionTickDiscoveredTransfers(t *testing.T) {
	for _, mode := range []string{"mint", "budget", "retry", "unbacked", "header-blocked", "payment-failure"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			transfer := planningTransfer(1)
			if mode == "header-blocked" {
				transfer.ID.Height = 25
				h.transfers[transfer.ID] = transfer
			} else {
				h.proof(transfer, 0)
			}
			h.visible = []sink.ID{transfer.ID}
			switch mode {
			case "budget":
				_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
					return true, ledger.charge(transfer.ID, h.s.cfg.maxLamports+1, "prior-spend", 0)
				})
				require.NoError(t, err)
			case "retry":
				require.NoError(t, h.s.relayFailed(context.Background(), transfer.ID, errors.New("prior failure")))
			case "unbacked":
				transfer.Status = sink.StatusRefunded
				h.transfers[transfer.ID] = transfer
			case "payment-failure":
				h.simulationFail = true
			}
			require.NoError(t, h.s.tick(context.Background()))
			ledger, err := h.s.store.readLedger()
			require.NoError(t, err)
			switch mode {
			case "mint":
				require.Contains(t, h.stages, "resolve")
				require.True(t, *ledger.entries[transfer.ID].Completed)
			case "payment-failure":
				require.Empty(t, h.stages)
				require.NotNil(t, ledger.entries[transfer.ID].Retry)
				require.Contains(t, h.errOut.String(), "relay-retry")
			default:
				require.Empty(t, h.stages)
				if mode != "retry" {
					event := map[string]string{
						"budget":         "relay-budget-exhausted",
						"unbacked":       "relay-unbacked-settlement",
						"header-blocked": "relay-header-blocked",
					}[mode]
					require.Contains(t, h.errOut.String(), event)
				}
			}
		})
	}
}
