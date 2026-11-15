package relayer

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func TestBusyResetMustNotReportSuccess(t *testing.T) {
	store := durableStore(t)
	id, err := native.ParseID(durableIdentity(t).ID)
	require.NoError(t, err)
	var output bytes.Buffer
	svc := &service{
		store: store,
		deps: dependencies{
			streams: Streams{
				Out: &output,
				Err: &output,
			},
		},
	}
	_, err = store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
		return true, ledger.charge(id, 123456, "existing-test-charge", 0)
	})
	require.NoError(t, err)
	before, err := optionalJSON(store.ledgerPath)
	require.NoError(t, err)
	locked, err := withFileLock(context.Background(), store.payerLock, 0, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resetErr := svc.resetBudget(ctx, "all")
		after, readErr := optionalJSON(store.ledgerPath)
		require.NoError(t, readErr)
		require.Equal(t, before, after, "the holder must prevent any reset")
		t.Logf("reset error=%v; ledger byte-identical=%v; output=%s", resetErr, bytes.Equal(before, after), output.String())
		if resetErr == nil {
			t.Error("busy reset returned success although no budget entry was reset")
		}
		if bytes.Contains(output.Bytes(), []byte("relay-budget-reset")) {
			t.Error("busy reset emitted a success event although the ledger is unchanged")
		}
		return nil
	})
	require.NoError(t, err)
	require.True(t, locked)
}

func TestBusyCoveredRelayMustNotClearRetry(t *testing.T) {
	h := newResolutionHarness(t)
	transfer := planningTransfer(1)
	h.proof(transfer, 0)
	_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
		ledger.set(transfer.ID, spendEntry{
			SpentLamports: 0,
			Attempts:      1,
			Retry: &retryState{
				Failures: 2,
				RetryAt:  h.now.UnixMilli() - 1,
			},
		})
		return true, nil
	})
	require.NoError(t, err)
	plans := []relayPlan{
		{
			id:           transfer.ID,
			transfer:     transfer,
			height:       20,
			resumeHeight: 20,
		},
	}
	locked, err := h.s.withPayer(context.Background(), func(*payerSession) error {
		return h.s.relayCovered(context.Background(), plans, func(context.Context) (relayHeader, error) {
			return relayHeader{
				followed: h.followed,
				height:   20,
			}, nil
		}, func() {})
	})
	require.NoError(t, err)
	require.True(t, locked)
	require.Empty(t, h.stages, "no paid work was performed")
	ledger, err := h.s.store.readLedger()
	require.NoError(t, err)
	t.Logf("paid stages=%v; retry after contention=%+v", h.stages, ledger.entries[transfer.ID].Retry)
	require.NotNil(t, ledger.entries[transfer.ID].Retry)
	require.Equal(t, int64(3), ledger.entries[transfer.ID].Retry.Failures)
	require.Greater(t, ledger.entries[transfer.ID].Retry.RetryAt, h.now.UnixMilli())
}

func TestBusyPayerStopsRequiredLifecycleWork(t *testing.T) {
	for _, operation := range []string{"recover", "resume-header", "update", "bootstrap", "lookups", "prune", "over-budget"} {
		t.Run(operation, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.s.cfg.local = true
			if operation == "resume-header" {
				h.active(t)
			}
			acquired, err := h.s.withPayer(context.Background(), func(*payerSession) error {
				ctx := context.Background()
				var failure error
				switch operation {
				case "recover":
					failure = h.s.recoverPayments(ctx)
				case "resume-header":
					failure = h.s.resumeHeader(ctx, nil)
				case "update":
					failure = h.s.update(ctx, 20, false, nil)
				case "bootstrap":
					failure = h.s.bootstrap(ctx)
				case "lookups":
					failure = h.s.reclaimLookups(ctx)
				case "prune":
					require.NoError(t, h.s.pruneConsensus(ctx, h.f))
					require.Contains(t, h.errOut.String(), "consensus-prune-failed")
					require.Contains(t, h.errOut.String(), errPayerBusy.Error())
					return nil
				case "over-budget":
					id, err := native.ParseID(durableIdentity(t).ID)
					require.NoError(t, err)
					ledger, err := h.s.store.readLedger()
					require.NoError(t, err)
					h.s.overBudget(ctx, id, ledger)
					require.Contains(t, h.errOut.String(), "proof-buffer-close-failed")
					require.Contains(t, h.errOut.String(), errPayerBusy.Error())
					return nil
				}
				require.ErrorIs(t, failure, errPayerBusy)
				return nil
			})
			require.NoError(t, err)
			require.True(t, acquired)
			require.Empty(t, h.stages)
		})
	}
}
