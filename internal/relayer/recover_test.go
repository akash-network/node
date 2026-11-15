package relayer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

func TestResolutionRecoveryRetainsOrReclaimsExactJob(t *testing.T) {
	for _, mode := range []string{"retained", "resolved", "missing-consensus", "foreign-consensus", "repinned", "value-changed", "unbacked", "consensus-outage", "buffer-outage"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			transfer := planningTransfer(1)
			proof := h.proof(transfer, 0)
			job := h.job(proof)
			if mode == "repinned" {
				job.LightClient = solana.SystemProgramID.String()
			}
			require.NoError(t, h.s.store.saveProof(job))
			buffer, err := h.s.deployment.ProofBuffer(h.s.payer.PublicKey(), proof.Key)
			require.NoError(t, err)
			nonce, err := hex.DecodeString(job.Nonce)
			require.NoError(t, err)
			h.buffer(buffer, bridge.ProofBuffer{
				Submitter:   h.s.payer.PublicKey(),
				Nonce:       [32]byte(nonce),
				ExpectedLen: uint32(len(proof.Merkle)),
				Data:        proof.Merkle,
				Allocated:   proofHeaderSize + len(proof.Merkle),
				Lamports:    1000,
			})
			consensus, err := h.followed.ConsensusAt(20)
			require.NoError(t, err)
			switch mode {
			case "resolved":
				h.status(transfer.ID)
			case "missing-consensus":
				delete(h.accounts, consensus)
			case "foreign-consensus":
				h.accounts[consensus].Owner = solana.SystemProgramID
			case "value-changed":
				transfer.Data.Dest = h.s.payer.PublicKey().String()
				h.transfers[transfer.ID] = transfer
			case "unbacked":
				transfer.Status = sink.StatusRefunded
				h.transfers[transfer.ID] = transfer
			case "consensus-outage":
				h.failAccount = consensus
			case "buffer-outage":
				delete(h.accounts, consensus)
				h.failAccount = buffer
			}
			path, err := h.s.store.proofPath(transfer.ID)
			require.NoError(t, err)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			err = h.s.recoverPayments(context.Background())
			if mode == "consensus-outage" || mode == "buffer-outage" {
				require.Error(t, err)
				require.Empty(t, h.stages)
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, before, after)
				return
			}
			require.NoError(t, err)
			saved, err := h.s.store.loadProof(transfer.ID)
			require.NoError(t, err)
			if mode == "retained" {
				require.Equal(t, job, saved)
				require.Empty(t, h.stages)
				require.Equal(t, []string{"finalized"}, h.accountReads[consensus])
			} else {
				require.Nil(t, saved)
				require.Equal(t, []string{"proof-close"}, h.stages)
				require.Equal(t, []string{"confirmed"}, h.accountReads[buffer])
			}
		})
	}
}

func TestResolutionRecoveryLedgerOrderAndCompletedSkip(t *testing.T) {
	h := newResolutionHarness(t)
	var ordered []solana.PublicKey
	var fields []string
	for _, sequence := range []uint64{9, 1, 5} {
		transfer := planningTransfer(sequence)
		h.transfers[transfer.ID] = transfer
		complete := sequence == 5
		fields = append(fields, fmt.Sprintf("%q:{\"spentLamports\":0,\"attempts\":1,\"completed\":%v}", transfer.ID.String(), complete))
		if !complete {
			address, err := h.s.deployment.Status(transfer.ID)
			require.NoError(t, err)
			ordered = append(ordered, address)
		}
	}
	ledger := fmt.Sprintf(`{"version":2,"genesis":%q,"programId":%q,"transfers":{%s,%s,%s}}`, h.s.genesis, h.s.cfg.program.String(), fields[0], fields[1], fields[2])
	require.NoError(t, atomicJSON(h.s.store.ledgerPath, []byte(ledger)))
	require.NoError(t, h.s.recoverPayments(context.Background()))
	require.Equal(t, ordered, h.statusIDs)
}

func TestResolutionResumeBeforePreparedFailureAndReset(t *testing.T) {
	h := newResolutionHarness(t)
	proof := h.proof(planningTransfer(1), 0)
	id := proof.Transfer.ID
	job := h.job(proof)
	h.lostStage = "proof-create"
	require.Error(t, runDurablePayer(t, h.s, func(p *payerSession) error {
		_, err := p.uploadProof(context.Background(), id, job)
		return err
	}))
	path, err := h.s.store.paymentPath(id)
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	h.lostStage = ""
	h.noMetadata = true
	failure := errors.New("prepared work must wait")
	err = runDurablePayer(t, h.s, func(p *payerSession) error {
		_, err := p.relayWith(context.Background(), h.followed, id, 20, &preparedProof{err: failure})
		return err
	})
	require.Error(t, err)
	require.NotErrorIs(t, err, failure)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	h.noMetadata = false
	err = runDurablePayer(t, h.s, func(p *payerSession) error {
		_, err := p.relayWith(context.Background(), h.followed, id, 20, &preparedProof{err: failure})
		return err
	})
	require.ErrorIs(t, err, failure)
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	ledger, err := h.s.store.readLedger()
	require.NoError(t, err)
	require.Equal(t, int64(5), ledger.spent(id))
	require.NoError(t, h.s.resetBudget(context.Background(), id.String()))
	ledger, err = h.s.store.readLedger()
	require.NoError(t, err)
	require.Empty(t, ledger.ids())
	require.Contains(t, h.out.String(), "relay-budget-reset")
}

func TestResolutionResetSettlesPendingBeforeErasingLedger(t *testing.T) {
	h := newResolutionHarness(t)
	proof := h.proof(planningTransfer(1), 0)
	id := proof.Transfer.ID
	_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
		return true, ledger.attempt(id)
	})
	require.NoError(t, err)
	h.lostStage = "proof-create"
	require.Error(t, runDurablePayer(t, h.s, func(p *payerSession) error {
		_, err := p.uploadProof(context.Background(), id, h.job(proof))
		return err
	}))
	before, err := os.ReadFile(h.s.store.ledgerPath)
	require.NoError(t, err)
	h.lostStage = ""
	h.failMethod = "getSignatureStatuses"
	require.Error(t, h.s.resetBudget(context.Background(), "all"))
	after, err := os.ReadFile(h.s.store.ledgerPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
	h.failMethod = ""
	require.NoError(t, h.s.resetBudget(context.Background(), "all"))
	ledger, err := h.s.store.readLedger()
	require.NoError(t, err)
	require.Empty(t, ledger.ids())
	require.Equal(t, 1, h.calls["sendTransaction"], "reset must settle the original transaction")
	path, err := h.s.store.paymentPath(id)
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}
