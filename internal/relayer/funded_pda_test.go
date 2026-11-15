package relayer

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"
)

// A system-owned address holding only lamports: what any wallet creates with
// a plain SOL transfer to a publicly derivable PDA address.
func fundedUninitializedAccount() *rpc.Account {
	return &rpc.Account{
		RentEpoch: new(big.Int),
		Owner:     solana.SystemProgramID,
		Lamports:  890_880,
		Data:      rpc.DataBytesOrJSONFromBytes([]byte{}),
	}
}

func TestRecoveryAbsentStatusResolvesInOneTick(t *testing.T) {
	h := newResolutionHarness(t)
	transfer := planningTransfer(1)
	h.proof(transfer, 0)
	h.visible = []sink.ID{transfer.ID}
	require.NoError(t, h.s.tick(context.Background()))
	t.Logf("control stages=%v", h.stages)
	require.Contains(t, h.stages, "resolve")
}

func TestRecoveryFundedUninitializedStatus(t *testing.T) {
	h := newResolutionHarness(t)
	transfer := planningTransfer(1)
	h.proof(transfer, 0)
	h.visible = []sink.ID{transfer.ID}
	address, err := h.s.deployment.Status(transfer.ID)
	require.NoError(t, err)
	h.accounts[address] = fundedUninitializedAccount()
	// The program's `init` turns the funded address into the status account.
	h.afterSend = func(stage string) {
		if stage == "resolve" {
			h.status(transfer.ID)
		}
	}

	first := h.s.tick(context.Background())
	ledger, err := h.s.store.readLedger()
	require.NoError(t, err)
	entry := ledger.entries[transfer.ID]
	t.Logf("tick1 error=%v stages=%v spent=%d attempts=%d retry=%+v", first, h.stages, entry.SpentLamports, entry.Attempts, entry.Retry)
	t.Logf("tick1 alerts=%s", h.errOut.String())
	job, err := h.s.store.loadProof(transfer.ID)
	require.NoError(t, err)
	t.Logf("tick1 proof job retained=%v", job != nil)
	if first != nil {
		t.Errorf("baseline: tick 1 does not fail; got %v", first)
	}
	resolved := false
	for _, stage := range h.stages {
		resolved = resolved || stage == "resolve"
	}
	if !resolved {
		t.Errorf("baseline: a funded but uninitialized status address is unresolved and the transfer is resolved; Go paid %v and did not resolve", h.stages)
	}
	require.NotNil(t, entry.Completed)
	require.True(t, *entry.Completed)
	require.Nil(t, job)

	h.mu.Lock()
	h.now = h.now.Add(10 * time.Minute)
	stateQueries := h.calls["/akash.sink.v1.Query/State"]
	stagesBefore := len(h.stages)
	h.mu.Unlock()
	second := h.s.tick(context.Background())
	t.Logf("tick2 error=%v new stages=%v native state queries during tick2=%d", second, h.stages[stagesBefore:], h.calls["/akash.sink.v1.Query/State"]-stateQueries)
	if second != nil {
		t.Errorf("baseline: later ticks keep running; Go aborts the whole tick in recoverPayments: %v", second)
	}
}

func TestRecoveryFundedUninitializedProofBuffer(t *testing.T) {
	for _, mode := range []string{"control-absent-buffer", "resolved-elsewhere", "stale-consensus"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			transfer := planningTransfer(1)
			proof := h.proof(transfer, 0)
			job := h.job(proof)
			require.NoError(t, h.s.store.saveProof(job))
			_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
				return true, ledger.attempt(transfer.ID)
			})
			require.NoError(t, err)
			buffer, err := h.s.deployment.ProofBuffer(h.s.payer.PublicKey(), proof.Key)
			require.NoError(t, err)
			if mode != "control-absent-buffer" {
				h.accounts[buffer] = fundedUninitializedAccount()
			}
			if mode == "stale-consensus" {
				consensus, err := h.followed.ConsensusAt(20)
				require.NoError(t, err)
				delete(h.accounts, consensus)
			} else {
				h.status(transfer.ID)
			}
			path, err := h.s.store.proofPath(transfer.ID)
			require.NoError(t, err)

			recoverErr := h.s.recoverPayments(context.Background())
			_, statErr := os.Stat(path)
			ledger, err := h.s.store.readLedger()
			require.NoError(t, err)
			completed := ledger.entries[transfer.ID].Completed
			t.Logf("%s: recoverPayments error=%v stages=%v proof job exists=%v completed=%v", mode, recoverErr, h.stages, statErr == nil, completed != nil && *completed)
			if recoverErr != nil {
				t.Errorf("baseline: closeProofBuffer treats a zero-data address as no buffer and drops the job; Go fails recovery: %v", recoverErr)
			}
			if statErr == nil {
				t.Errorf("baseline: the proof job is unlinked; Go retains it")
			}
			if mode != "stale-consensus" && (completed == nil || !*completed) {
				t.Errorf("baseline: the resolved transfer is marked completed")
			}
			require.Empty(t, h.stages, "no paid transaction is expected in any variant")
		})
	}
}

func TestHeaderJobFundedUninitializedTarget(t *testing.T) {
	for _, outcome := range []string{"authenticated", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			failAssembly := outcome == "rejected"
			h := newHeaderJobFixture(t)
			address, err := h.f.ConsensusAt(11)
			require.NoError(t, err)
			h.accounts[address] = fundedUninitializedAccount()
			if failAssembly {
				h.failStage = "assembly"
				h.failure = map[string]any{
					"InstructionError": []any{2, "InvalidInstructionData"},
				}
			}

			var result headerUploadResult
			err = h.s.withRequiredPayer(context.Background(), func(p *payerSession) error {
				var err error
				result, err = p.uploadHeader(context.Background(), h.f, h.packet, 11, 10, 1)
				return err
			})
			require.NoError(t, err)
			require.Contains(t, h.stages, "assembly")
			require.Contains(t, h.stages, "cleanup")
			require.Equal(t, failAssembly, transactionFailed(result.assembly))
			if failAssembly {
				require.Equal(t, uint64(10), h.latest)
				require.Empty(t, h.accounts[address].Data.GetBinary())
			} else {
				require.Equal(t, uint64(11), h.latest)
				require.NotEmpty(t, h.accounts[address].Data.GetBinary())
			}
		})
	}
}

func TestHeaderJobBootstrapFundedUninitializedAccounts(t *testing.T) {
	for _, kind := range []string{"access", "launch", "client", "bridge"} {
		t.Run(kind, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.attachNative(t, 11, true)
			accounts := map[string]solana.PublicKey{
				"access": h.f.AccessState,
				"launch": h.s.deployment.Config,
				"client": h.f.ClientState,
				"bridge": h.s.deployment.Bridge,
			}
			for _, address := range accounts {
				delete(h.accounts, address)
			}
			h.accounts[accounts[kind]] = fundedUninitializedAccount()

			require.NoError(t, h.s.bootstrap(context.Background()))
			require.Len(t, h.stages, 6)
			require.NoError(t, h.s.bootstrap(context.Background()))
			require.Len(t, h.stages, 6, "initialized state is retained on restart")
		})
	}
}
