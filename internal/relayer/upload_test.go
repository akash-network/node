package relayer

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

func TestResolutionUploadResumeExactBytesAndRent(t *testing.T) {
	for _, mode := range []string{"fresh", "partial", "changed", "prefix", "owner", "outage", "lost-response", "budget", "stuck-grow", "stuck-append", "terminal-failure", "write-failure"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			proof := h.proof(planningTransfer(1), 1700)
			id := proof.Transfer.ID
			job := h.job(proof)
			address, err := h.s.deployment.ProofBuffer(h.s.payer.PublicKey(), proof.Key)
			require.NoError(t, err)
			if mode != "fresh" {
				saved := *job
				if mode == "changed" {
					saved.ProofHeight++
				}
				require.NoError(t, h.s.store.saveProof(&saved))
				nonce, err := hex.DecodeString(job.Nonce)
				require.NoError(t, err)
				h.buffer(address, bridge.ProofBuffer{
					Submitter:   h.s.payer.PublicKey(),
					Nonce:       [32]byte(nonce),
					ExpectedLen: uint32(len(proof.Merkle)),
					Data:        bytes.Clone(proof.Merkle[:450]),
					Allocated:   len(proof.Merkle) + proofHeaderSize,
					Lamports:    1200,
				})
			}
			switch mode {
			case "prefix":
				data := h.accounts[address].Data.GetBinary()
				data[proofHeaderSize] ^= 1
			case "owner":
				h.accounts[address].Owner = solana.SystemProgramID
			case "outage":
				h.failAccount = address
			case "lost-response":
				h.lostStage = "proof-append"
			case "budget":
				h.s.cfg.maxLamports = 0
			case "stuck-grow":
				delete(h.accounts, address)
				h.stuckStage = "proof-grow"
			case "stuck-append":
				h.stuckStage = "proof-append"
			case "terminal-failure":
				h.failStage = "proof-append"
			case "write-failure":
				h.s.store.writeJSON = func(string, []byte) error { return errors.New("disk full") }
			}
			upload := func(p *payerSession) error {
				_, err := p.uploadProof(context.Background(), id, job)
				return err
			}
			err = runDurablePayer(t, h.s, upload)
			if mode == "fresh" || mode == "partial" || mode == "changed" {
				require.NoError(t, err)
				saved, err := h.s.store.loadProof(id)
				require.NoError(t, err)
				require.Equal(t, job.Proof, saved.Proof)
				require.Equal(t, job.ProofHash, saved.ProofHash)
				require.Equal(t, len(proof.Merkle), saved.Uploaded)
				account, err := bridge.DecodeProofBuffer(h.accounts[address].Data.GetBinary())
				require.NoError(t, err)
				require.Equal(t, proof.Merkle, account.Data)
				if mode == "partial" {
					require.Equal(t, []string{"proof-append", "proof-append"}, h.stages)
				}
				if mode == "changed" {
					require.Equal(t, "proof-close", h.stages[0])
					require.Equal(t, "proof-create", h.stages[1])
				}
				return
			}
			require.Error(t, err)
			saved, loadErr := h.s.store.loadProof(id)
			require.NoError(t, loadErr)
			require.NotNil(t, saved)
			require.Equal(t, job.Proof, saved.Proof)
			if mode == "prefix" || mode == "owner" || mode == "outage" || mode == "write-failure" {
				require.Empty(t, h.stages)
			}
			if mode == "budget" {
				require.ErrorIs(t, err, errBudgetExhausted)
				require.NoError(t, runDurablePayer(t, h.s, func(p *payerSession) error {
					return p.closeProofBuffer(context.Background(), id)
				}))
				require.Equal(t, "proof-close", h.stages[len(h.stages)-1])
			}
			if mode == "lost-response" {
				path, err := h.s.store.paymentPath(id)
				require.NoError(t, err)
				intentBefore, err := os.ReadFile(path)
				require.NoError(t, err)
				h.lostStage = ""
				h.failMethod = "getSignatureStatuses"
				require.Error(t, runDurablePayer(t, h.s, func(p *payerSession) error {
					return p.resumePayment(context.Background(), id)
				}))
				intentAfter, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, intentBefore, intentAfter)
				h.failMethod = ""
				require.NoError(t, runDurablePayer(t, h.s, func(p *payerSession) error {
					if err := p.resumePayment(context.Background(), id); err != nil {
						return err
					}
					return upload(p)
				}))
				require.Equal(t, 2, h.calls["sendTransaction"])
				ledger, err := h.s.store.readLedger()
				require.NoError(t, err)
				require.Equal(t, int64(10), ledger.spent(id))
			}
		})
	}
}
