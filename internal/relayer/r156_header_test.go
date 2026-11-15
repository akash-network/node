package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

func r156Donation(lamports uint64) *rpc.Account {
	account := headerJobAccount(solana.SystemProgramID, []byte{})
	account.Lamports = lamports
	return account
}

func (h *r156RPC) layout(t *testing.T) headerLayout {
	t.Helper()
	layout, reason, err := layoutHeader(h.f, h.s.payer.PublicKey(), h.packet, 11, 10, signatureBatchCapacity)
	require.NoError(t, err)
	require.Empty(t, reason)
	return layout
}

func (h *r156RPC) restart(t *testing.T) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	next := *h.s
	store, err := newStateStore(next.cfg.stateDir, next.cfg.chainID, next.genesis, next.cfg.program.String(), next.payer.PublicKey().String(), nil)
	require.NoError(t, err)
	next.store = store
	next.reader = bridge.NewReader(next.solana, next.cfg.program, next.cfg.chainID, next.cfg.pinnedClient)
	h.s = &next
}

func (h *r156RPC) pending(t *testing.T) (string, *intent) {
	t.Helper()
	ids, err := h.s.store.sessionIDs()
	require.NoError(t, err)
	require.Len(t, ids, 1)
	path, err := h.s.store.sessionPendingPath(ids[0])
	require.NoError(t, err)
	pending, err := h.s.store.loadIntent(path)
	require.NoError(t, err)
	require.NotNil(t, pending)
	return path, pending
}

func (h *r156RPC) laterHeader(t *testing.T) {
	t.Helper()
	h.clock = 50_000
	h.setClock()
	upkeepHeaderNative(t, h.headerJobRPC)
	before := countHeaderStage(h.stages, "assembly")
	u := startUpkeep(t, h.s)
	require.Equal(t, headerHeight(29), u.current.height)
	require.Equal(t, before+1, countHeaderStage(h.stages, "assembly"))
	stored, err := h.s.reader.Consensus(context.Background(), h.f, 29)
	require.NoError(t, err)
	require.True(t, matchingConsensus(stored, h.packet))
	active, err := h.s.store.loadActive()
	require.NoError(t, err)
	require.Nil(t, active)
}

func TestR156FundedHeaderProgress(t *testing.T) {
	for _, kind := range []string{"chunk", "batch"} {
		for _, lamports := range []uint64{122, 123, 124} {
			t.Run(fmt.Sprintf("%s/%d", kind, lamports), func(t *testing.T) {
				h := newR156Fixture(t)
				h.active(t)
				layout := h.layout(t)
				address, stage := layout.chunks[0], "chunk"
				if kind == "batch" {
					address, stage = layout.batches[0], "signature-batch-create"
				}
				h.accounts[address] = r156Donation(lamports)
				h.lostStage = stage
				h.s.headerRetry = retryState{
					Failures: 3,
					RetryAt:  90000,
				}
				beforeActive, err := os.ReadFile(h.s.store.activePath)
				require.NoError(t, err)
				err = h.s.resumeHeader(context.Background(), nil)
				require.ErrorContains(t, err, "503", "funded account must reach initialization before the injected lost response")
				path, pending := h.pending(t)
				require.Equal(t, stage, pending.Stage)
				intentBytes, err := os.ReadFile(path)
				require.NoError(t, err)
				h.mu.Lock()
				h.lostStage = ""
				h.metadataMissing = true
				h.mu.Unlock()
				h.restart(t)
				paid := len(h.broadcasts)
				for i := 0; i < 3; i++ {
					require.ErrorContains(t, h.s.resumeHeader(context.Background(), nil), "missing transaction metadata")
					retained, err := os.ReadFile(path)
					require.NoError(t, err)
					require.Equal(t, intentBytes, retained)
					require.Len(t, h.broadcasts, paid)
				}
				retained, err := os.ReadFile(h.s.store.activePath)
				require.NoError(t, err)
				require.Equal(t, beforeActive, retained)
				h.mu.Lock()
				h.metadataMissing = false
				h.mu.Unlock()
				h.restart(t)
				require.NoError(t, h.s.resumeHeader(context.Background(), nil))
				active, err := h.s.store.loadActive()
				require.NoError(t, err)
				require.Nil(t, active)
				require.Equal(t, uint64(11), h.latest)
				require.Equal(t, int64(3), h.s.headerRetry.Failures)
				require.Equal(t, int64(90000), h.s.headerRetry.RetryAt)
				require.Equal(t, pending.Raw, h.transactions[pending.Signature].raw)
				topup := uint64(0)
				if lamports < r156MockRent {
					topup = r156MockRent - lamports
				}
				require.Equal(t, topup, h.topups[address])
				require.Equal(t, max(lamports, r156MockRent), h.refunds[address])
				require.Nil(t, h.accounts[address])
				log, err := os.ReadFile(filepath.Join(h.s.store.headerDirectory, "transactions.jsonl"))
				require.NoError(t, err)
				require.Contains(t, string(log), `"event":"transaction"`)
				require.Contains(t, string(log), `"fee":5000`)
				observed := make(map[string]int)
				decoder := json.NewDecoder(bytes.NewReader(log))
				for decoder.More() {
					var event map[string]any
					require.NoError(t, decoder.Decode(&event))
					if event["event"] == "transaction" {
						observed[event["signature"].(string)]++
						require.Equal(t, float64(5000), event["fee"])
						require.Equal(t, float64(5000), event["payerLamports"])
					}
				}
				require.Len(t, observed, len(h.transactions))
				for _, count := range observed {
					require.Equal(t, 1, count, "receipt observer records each signature once")
				}
				h.laterHeader(t)
				h.checkCommitments(t, layout)
			})
		}
	}
}

func TestR156HeaderRestartBoundaries(t *testing.T) {
	for _, stage := range []string{"signature", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			h := newR156Fixture(t)
			h.active(t)
			layout := h.layout(t)
			h.accounts[layout.chunks[0]] = r156Donation(10)
			h.accounts[layout.batches[0]] = r156Donation(200)
			h.lostStage = stage
			require.ErrorContains(t, h.s.resumeHeader(context.Background(), nil), "503")
			_, pending := h.pending(t)
			require.Equal(t, stage, pending.Stage)
			h.mu.Lock()
			h.lostStage = ""
			h.mu.Unlock()
			h.restart(t)
			require.NoError(t, h.s.resumeHeader(context.Background(), nil))
			require.Equal(t, pending.Raw, h.transactions[pending.Signature].raw)
			active, err := h.s.store.loadActive()
			require.NoError(t, err)
			require.Nil(t, active)
			h.laterHeader(t)
		})
	}
}

func TestR156MatchingTargetResidualProgress(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(fmt.Sprint(concurrent), func(t *testing.T) {
			h := newR156Fixture(t)
			h.active(t)
			layout := h.layout(t)
			address := layout.chunks[0]
			if concurrent {
				h.failStage = "assembly"
				h.concurrent = true
				h.failure = map[string]any{
					"InstructionError": []any{2, "already initialized"},
				}
				h.afterApply = func(_ *solana.Transaction, stage string) {
					if stage == "cleanup" {
						h.accounts[address] = r156Donation(321)
					}
				}
			} else {
				h.latest = 11
				h.accounts[h.f.ClientState] = headerJobAccount(h.f.Program, h.clientData())
				target, err := h.f.ConsensusAt(11)
				require.NoError(t, err)
				h.accounts[target] = headerJobAccount(h.f.Program, headerJobConsensus(11, h.packet))
				h.accounts[address] = r156Donation(321)
			}
			require.NoError(t, h.s.resumeHeader(context.Background(), nil))
			require.Equal(t, uint64(321), h.accounts[address].Lamports, "donation remains; cleanup does not refund it")
			require.Equal(t, solana.SystemProgramID, h.accounts[address].Owner)
			if concurrent {
				require.Equal(t, uint64(r156MockRent), h.refunds[address], "only the old allocated object was closed")
				log, err := os.ReadFile(filepath.Join(h.s.store.headerDirectory, "transactions.jsonl"))
				require.NoError(t, err)
				require.Contains(t, string(log), "authenticated-concurrently")
			} else {
				require.Zero(t, h.refunds[address])
				require.NotContains(t, h.stages, "assembly")
			}
			h.mu.Lock()
			h.afterApply = nil
			h.failStage = ""
			h.mu.Unlock()
			h.restart(t)
			h.laterHeader(t)
		})
	}
}

func TestR156CleanupRejectsOtherResiduals(t *testing.T) {
	for _, kind := range []string{"foreign-empty", "foreign-populated", "system-populated", "program-empty", "program-malformed", "executable", "wrong-submitter", "wrong-batch-bump"} {
		t.Run(kind, func(t *testing.T) {
			h := newR156Fixture(t)
			h.active(t)
			layout := h.layout(t)
			address := layout.chunks[0]
			a := headerJobAccount(solana.PublicKey{99}, []byte{})
			switch kind {
			case "foreign-populated":
				a.Data = rpc.DataBytesOrJSONFromBytes([]byte{1})
			case "system-populated":
				a.Owner = solana.SystemProgramID
				a.Data = rpc.DataBytesOrJSONFromBytes([]byte{1})
			case "program-empty":
				a.Owner = h.f.Program
			case "program-malformed":
				a.Owner = h.f.Program
				a.Data = rpc.DataBytesOrJSONFromBytes([]byte{1})
			case "executable":
				a.Owner = solana.SystemProgramID
				a.Executable = true
			case "wrong-submitter":
				payer := solana.PublicKey{99}
				data := append(headerJobDisc("account", "HeaderChunk"), payer[:]...)
				data = append(data, make([]byte, 4)...)
				a = headerJobAccount(h.f.Program, data)
			case "wrong-batch-bump":
				address = layout.batches[0]
				payer := h.s.payer.PublicKey()
				_, bump, err := solana.FindProgramAddress([][]byte{[]byte("sig_batch"), payer[:], layout.sessions[0][:]}, h.f.Program)
				require.NoError(t, err)
				a = headerJobAccount(h.f.Program, r156BatchBytes(payer, layout.sessions[0], bump^1, nil))
			}
			h.accounts[address] = a
			h.latest = 11
			h.accounts[h.f.ClientState] = headerJobAccount(h.f.Program, h.clientData())
			target, err := h.f.ConsensusAt(11)
			require.NoError(t, err)
			h.accounts[target] = headerJobAccount(h.f.Program, headerJobConsensus(11, h.packet))
			before, err := json.Marshal(a)
			require.NoError(t, err)
			require.ErrorContains(t, h.s.resumeHeader(context.Background(), nil), "temporary accounts remain")
			after, err := json.Marshal(h.accounts[address])
			require.NoError(t, err)
			require.Equal(t, before, after)
			active, err := h.s.store.loadActive()
			require.NoError(t, err)
			require.NotNil(t, active)
			require.Zero(t, h.refunds[address])
		})
	}
}

func TestR156WrongRootRetainsJob(t *testing.T) {
	h := newR156Fixture(t)
	h.active(t)
	layout := h.layout(t)
	h.accounts[layout.chunks[0]] = r156Donation(321)
	target, err := h.f.ConsensusAt(11)
	require.NoError(t, err)
	wrong := h.packet
	wrong.Root[0] ^= 1
	h.accounts[target] = headerJobAccount(h.f.Program, headerJobConsensus(11, wrong))
	require.ErrorContains(t, h.s.resumeHeader(context.Background(), nil), "stored target differs from upload")
	require.Empty(t, h.broadcasts)
	active, err := h.s.store.loadActive()
	require.NoError(t, err)
	require.NotNil(t, active)
}

func TestR156FundingBetweenReadAndSend(t *testing.T) {
	h := newR156Fixture(t)
	h.active(t)
	layout := h.layout(t)
	funded := map[string]bool{}
	h.beforeSend = func(_ *solana.Transaction, stage string) {
		if funded[stage] {
			return
		}
		switch stage {
		case "chunk":
			h.accounts[layout.chunks[0]] = r156Donation(321)
		case "signature-batch-create":
			h.accounts[layout.batches[0]] = r156Donation(321)
		default:
			return
		}
		funded[stage] = true
	}
	require.NoError(t, h.s.resumeHeader(context.Background(), nil))
	require.True(t, funded["chunk"])
	require.True(t, funded["signature-batch-create"])
	require.Equal(t, uint64(321), h.refunds[layout.chunks[0]])
	require.Equal(t, uint64(321), h.refunds[layout.batches[0]])
}

func TestR156PostCreateRequiresAllocatedBatch(t *testing.T) {
	h := newR156Fixture(t)
	h.active(t)
	layout := h.layout(t)
	h.afterApply = func(_ *solana.Transaction, stage string) {
		if stage == "signature-batch-create" {
			h.accounts[layout.batches[0]] = r156Donation(321)
		}
	}
	require.ErrorContains(t, h.s.resumeHeader(context.Background(), nil), "signature batch unavailable after creation")
	require.NotContains(t, h.stages, "signature")
	require.NotContains(t, h.stages, "assembly")
}

func TestR156PendingExpiryStopsNewSends(t *testing.T) {
	h := newR156Fixture(t)
	h.active(t)
	h.accounts[h.layout(t).batches[0]] = r156Donation(321)
	h.dropStage = "signature-batch-create"
	require.ErrorContains(t, h.s.resumeHeader(context.Background(), nil), "503")
	path, pending := h.pending(t)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	paid := len(h.broadcasts)
	h.mu.Lock()
	h.dropStage = ""
	h.blockHeight = 1001
	h.mu.Unlock()
	h.restart(t)
	require.ErrorContains(t, h.s.resumeHeader(context.Background(), nil), "expired transaction history unresolved")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(before, after))
	require.Len(t, h.broadcasts, paid)
	require.NotContains(t, h.stages, "cleanup")
	require.Equal(t, "signature-batch-create", pending.Stage)
}

func TestR156ResidualDonationIsNotReturnedRent(t *testing.T) {
	for _, beforeAssembly := range []bool{false, true} {
		t.Run(fmt.Sprint(beforeAssembly), func(t *testing.T) {
			h := newR156Fixture(t)
			layout := h.layout(t)
			address := layout.chunks[0]
			authenticate := func() {
				h.latest = 11
				h.accounts[h.f.ClientState] = headerJobAccount(h.f.Program, h.clientData())
				target, err := h.f.ConsensusAt(11)
				require.NoError(t, err)
				h.accounts[target] = headerJobAccount(h.f.Program, headerJobConsensus(11, h.packet))
				h.accounts[address] = r156Donation(321)
			}
			if beforeAssembly {
				h.failStage = "assembly"
				h.failure = "already authenticated by another transaction"
				h.afterApply = func(_ *solana.Transaction, stage string) {
					if stage == "chunk" && h.accounts[layout.chunks[len(layout.chunks)-1]] != nil {
						authenticate()
					}
				}
			} else {
				authenticate()
			}
			var result headerUploadResult
			err := h.s.withRequiredPayer(context.Background(), func(p *payerSession) error {
				var err error
				result, err = p.uploadHeader(context.Background(), h.f, h.packet, 11, 10, signatureBatchCapacity)
				return err
			})
			require.NoError(t, err)
			require.False(t, transactionFailed(result.assembly))
			require.Equal(t, uint64(321), h.accounts[address].Lamports)
			require.Zero(t, h.refunds[address], "this cleanup never refunds a donated residual")
			rent := uint64(0)
			if beforeAssembly {
				rent = uint64(len(layout.chunks)+len(layout.batches)-1) * r156MockRent
			}
			require.Equal(t, rent, result.temporaryRentReturned)
			require.Equal(t, uint64(result.transactions)*5000, result.fees, "real observer consumes mock receipts")
			require.Zero(t, result.balanceDelta, "the fixture's balance response is constant; no live balance claim")
		})
	}
}

func (h *r156RPC) checkCommitments(t *testing.T, layout headerLayout) {
	t.Helper()
	temporary := append(append([]solana.PublicKey{}, layout.chunks...), layout.batches...)
	target, err := h.f.ConsensusAt(11)
	require.NoError(t, err)
	sawCleanup, sawTarget, sawClient := false, false, false
	for _, call := range h.calls {
		if call.Method != "getAccountInfo" && call.Method != "getMultipleAccounts" {
			continue
		}
		var options map[string]any
		require.NoError(t, json.Unmarshal(call.Params[1], &options))
		if call.Method == "getMultipleAccounts" {
			var addresses []solana.PublicKey
			require.NoError(t, json.Unmarshal(call.Params[0], &addresses))
			if len(addresses) > 0 && addresses[0] == temporary[0] && options["commitment"] == "finalized" {
				sawCleanup = true
			}
			continue
		}
		var address solana.PublicKey
		require.NoError(t, json.Unmarshal(call.Params[0], &address))
		for _, key := range temporary {
			if address == key {
				require.Equal(t, "confirmed", options["commitment"])
			}
		}
		if options["commitment"] == "finalized" {
			sawTarget = sawTarget || address == target
			sawClient = sawClient || address == h.f.ClientState
		}
	}
	require.True(t, sawCleanup)
	require.True(t, sawTarget)
	require.True(t, sawClient)
}
