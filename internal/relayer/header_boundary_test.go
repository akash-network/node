package relayer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"testing"

	solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/header"
)

func (h *headerJobRPC) uploadForTest(t *testing.T, perBatch int) (headerUploadResult, error) {
	t.Helper()
	var result headerUploadResult
	err := h.s.withRequiredPayer(context.Background(), func(p *payerSession) error {
		var err error
		result, err = p.uploadHeader(context.Background(), h.f, h.packet, 11, 10, perBatch)
		return err
	})
	return result, err
}

// The program's cleanup skips accounts it does
// not own (cleanup_incomplete_upload.rs:22-25), so one can remain.
func TestTemporaryAccountsMustBeGone(t *testing.T) {
	h := newHeaderJobFixture(t)
	layout, reason, err := layoutHeader(h.f, h.s.payer.PublicKey(), h.packet, 11, 10, signatureBatchCapacity)
	require.NoError(t, err)
	require.Empty(t, reason)

	// Another relayer already authenticated the target: no upload, cleanup only.
	h.latest = 11
	h.accounts[h.f.ClientState] = headerJobAccount(h.f.Program, h.clientData())
	target, err := h.f.ConsensusAt(11)
	require.NoError(t, err)
	h.accounts[target] = headerJobAccount(h.f.Program, headerJobConsensus(11, h.packet))
	h.accounts[layout.chunks[0]] = headerJobAccount(solana.PublicKey{99}, []byte{})
	payer := h.s.payer.PublicKey()
	chunk := append(headerJobDisc("account", "HeaderChunk"), payer[:]...)
	chunk = binary.LittleEndian.AppendUint32(chunk, 1)
	chunk = append(chunk, 7)
	h.accounts[layout.chunks[1]] = headerJobAccount(h.f.Program, chunk)

	_, err = h.uploadForTest(t, signatureBatchCapacity)
	require.ErrorContains(t, err, "temporary accounts remain")
	require.Equal(t, []string{"cleanup"}, h.stages)
	require.NotNil(t, h.accounts[layout.chunks[0]], "the foreign account is not closed by the program")
	require.Nil(t, h.accounts[layout.chunks[1]], "the owned chunk is closed")
}

func TestAuthenticatedTargetMustMatchUpload(t *testing.T) {
	h := newHeaderJobFixture(t)
	// The fixture stores the consensus of h.packet on assembly; upload a
	// packet whose root differs, as a conflicting authentication would look.
	uploaded := h.packet
	uploaded.Root = sha256.Sum256([]byte("uploaded root"))

	var result headerUploadResult
	err := h.s.withRequiredPayer(context.Background(), func(p *payerSession) error {
		var err error
		result, err = p.uploadHeader(context.Background(), h.f, uploaded, 11, 10, 1)
		return err
	})
	require.ErrorContains(t, err, "authenticated target differs from requested header")
	require.Contains(t, h.stages, "assembly")
	require.Contains(t, h.stages, "cleanup", "a terminal assembly still releases its temporary accounts")
	require.Empty(t, result.assembly.signature)
}

// A journal that records a successful terminal
// assembly cannot be accepted when no finalized target exists.
func TestTerminalSuccessNeedsAuthenticatedTarget(t *testing.T) {
	h := newHeaderJobFixture(t)
	layout, reason, err := layoutHeader(h.f, h.s.payer.PublicKey(), h.packet, 11, 10, 1)
	require.NoError(t, err)
	require.Empty(t, reason)
	id := hex.EncodeToString(layout.session[:])
	require.NoError(t, h.s.store.saveSession(id, &sessionDoc{
		Version: 1,
		Genesis: h.s.genesis,
		Payer:   h.s.payer.PublicKey().String(),
		Client:  h.f.Program.String(),
		Target:  11,
		Trusted: 10,
		Session: id,
		Assembly: &transactionDoc{
			Stage:     "assembly",
			Signature: "recorded-success",
			Error:     json.RawMessage("null"),
		},
	}))

	_, err = h.uploadForTest(t, 1)
	require.ErrorContains(t, err, "successful finalized assembly has no authenticated target")
	require.NotContains(t, h.stages, "assembly")
	require.Equal(t, uint64(10), h.latest)
}

func TestUploadLookupAuthority(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.active(t)
	_, address := h.savedHeaderLookup(t, 1)
	h.tables[address] = solana.PublicKeySlice{}
	h.accounts[address] = headerJobAccount(solana.AddressLookupTableProgramID, h.lookupData(address, math.MaxUint64, 1, h.f.Program))

	err := h.s.resumeHeader(context.Background(), nil)
	require.ErrorContains(t, err, "lookup authority mismatch")
	require.Empty(t, h.stages)
	active, err := h.s.store.loadActive()
	require.NoError(t, err)
	require.NotNil(t, active)
}

// A confirmed failure of any stage before
// assembly stops the upload; nothing is assembled or cleaned up.
func TestFailedStageStopsUpload(t *testing.T) {
	for _, stage := range []string{"lookup-extend", "signature-batch-create", "signature", "chunk"} {
		t.Run(stage, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.active(t)
			h.failStage = stage
			h.failure = map[string]any{
				"InstructionError": []any{0, "InvalidArgument"},
			}

			err := h.s.resumeHeader(context.Background(), nil)
			require.ErrorContains(t, err, stage+" failed")
			require.NotContains(t, h.stages, "assembly")
			require.NotContains(t, h.stages, "cleanup")
			active, err := h.s.store.loadActive()
			require.NoError(t, err)
			require.NotNil(t, active)
		})
	}
}

// An inactive client is rejected before a new packet is
// fetched or made durable.
func TestInactiveClientSavesNoNewJob(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.attachNative(t, 11, false)
	h.clock += int64(h.trusting)
	h.setClock()

	err := h.s.update(context.Background(), 11, false, nil)
	require.ErrorContains(t, err, "light client is expired")
	active, err := h.s.store.loadActive()
	require.NoError(t, err)
	require.Nil(t, active, "no header job may be saved for an expired client")
	require.Empty(t, h.stages)
}

// The verifier reads the Ed25519
// instruction at index zero of the receipt transaction, and assembly needs
// its compute and heap requests in front.
func TestUploadTransactionComposition(t *testing.T) {
	h := newHeaderJobFixture(t)
	_, err := h.uploadForTest(t, 1)
	require.NoError(t, err)

	ed25519Program := solana.MustPublicKeyFromBase58("Ed25519SigVerify111111111111111111111111111")
	seen := map[string]int{}
	for _, encoded := range h.broadcasts {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		require.NoError(t, err)
		tx, err := solana.TransactionFromBytes(raw)
		require.NoError(t, err)
		stage := h.stage(tx)
		seen[stage]++
		programs := make([]solana.PublicKey, len(tx.Message.Instructions))
		for i, ix := range tx.Message.Instructions {
			programs[i] = tx.Message.AccountKeys[ix.ProgramIDIndex]
		}
		tabled := tx.Message.AddressTableLookups.NumLookups() != 0
		switch stage {
		case "signature":
			require.Equal(t, []solana.PublicKey{ed25519Program, h.f.Program}, programs)
			native, receipt := []byte(tx.Message.Instructions[0].Data), []byte(tx.Message.Instructions[1].Data)
			require.Empty(t, tx.Message.Instructions[0].Accounts)
			require.Equal(t, receipt[40:72], native[16:48], "public key")
			require.Equal(t, receipt[len(receipt)-64:], native[48:112], "signature")
			require.Equal(t, receipt[76:len(receipt)-64], native[112:], "message")
			require.False(t, tabled)
		case "assembly":
			require.Equal(t, []solana.PublicKey{solana.ComputeBudget, solana.ComputeBudget, h.f.Program}, programs)
			require.Equal(t, "02c05c1500", hex.EncodeToString(tx.Message.Instructions[0].Data))
			require.Equal(t, "0100000400", hex.EncodeToString(tx.Message.Instructions[1].Data))
			require.True(t, tabled)
		case "chunk":
			require.Equal(t, []solana.PublicKey{h.f.Program}, programs)
			require.True(t, tabled)
		case "signature-batch-create", "cleanup":
			require.Equal(t, []solana.PublicKey{h.f.Program}, programs)
			require.False(t, tabled)
		case "lookup-create", "lookup-extend", "lookup-deactivate":
			require.Equal(t, []solana.PublicKey{solana.AddressLookupTableProgramID}, programs)
			require.False(t, tabled)
		default:
			t.Fatalf("unexpected stage %s", stage)
		}
	}
	require.Equal(t, 2, seen["signature"])
	require.Equal(t, 1, seen["assembly"])
	require.Equal(t, 3, seen["chunk"])
}

// Only expired states are selected.
func TestPruneSkipsUnexpiredState(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.expiredStates(t, 3)
	fresh, err := h.f.ConsensusAt(50)
	require.NoError(t, err)
	// trusting 10 s, clock 1000 s: 995 s expires at 1005 s.
	h.accounts[fresh] = headerJobAccount(h.f.Program, headerJobConsensus(50, header.Packet{Timestamp: 995_000_000_000}))
	boundary, err := h.f.ConsensusAt(51)
	require.NoError(t, err)
	// 990 s + 10 s == 1000 s: expired exactly at the boundary.
	h.accounts[boundary] = headerJobAccount(h.f.Program, headerJobConsensus(51, header.Packet{Timestamp: 990_000_000_000}))

	require.NoError(t, h.s.pruneConsensus(context.Background(), h.f))
	require.Equal(t, []uint64{1, 2, 3, 51}, h.pruned)
	require.NotNil(t, h.accounts[fresh])
	// Three expired, the fixture's unexpired height 10, the latest, the
	// noncanonical address, and the two states added above.
	require.Contains(t, h.out.String(), `"scanned":8`)
	require.Contains(t, h.out.String(), `"expired":4`)
}

// A session whose close was observed is final.
func TestObservedLookupCloseIsFinal(t *testing.T) {
	h := newHeaderJobFixture(t)
	id, _ := h.lookupSession(t, "observed", 100)
	require.NoError(t, h.s.reclaimLookups(context.Background()))
	require.Equal(t, []string{"lookup-close"}, h.stages)

	for i := 0; i < 2; i++ {
		require.NoError(t, h.s.reclaimLookups(context.Background()))
	}
	doc, err := h.s.store.loadSession(id)
	require.NoError(t, err)
	require.True(t, *doc.LookupClosed)
	require.Nil(t, doc.LookupClosureUnobserved, "an observed close must not be relabelled as unobserved")
	require.Equal(t, int64(123), *doc.LookupRentReturned)
	require.Equal(t, []string{"lookup-close"}, h.stages)
}

func (h *headerJobRPC) readCommitments(t *testing.T, method string, address solana.PublicKey) []string {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	var found []string
	for _, call := range h.calls {
		if call.Method != method || !bytes.Contains(call.Params[0], []byte(address.String())) {
			continue
		}
		var options struct {
			Commitment string `json:"commitment"`
		}
		require.NoError(t, json.Unmarshal(call.Params[1], &options))
		found = append(found, options.Commitment)
	}
	return found
}

// The fixture serves one state for every commitment. Bind each call site to
// the commitment it requests, including post-transaction finalized reads.
func TestUploadReadCommitments(t *testing.T) {
	h := newHeaderJobFixture(t)
	layout, reason, err := layoutHeader(h.f, h.s.payer.PublicKey(), h.packet, 11, 10, 1)
	require.NoError(t, err)
	require.Empty(t, reason)
	_, err = h.uploadForTest(t, 1)
	require.NoError(t, err)

	target, err := h.f.ConsensusAt(11)
	require.NoError(t, err)
	require.Equal(t, []string{"finalized", "finalized", "confirmed"}, h.readCommitments(t, "getAccountInfo", target),
		"existing target, authenticated target, then the rent observation")
	require.Equal(t, []string{"confirmed", "finalized"}, h.readCommitments(t, "getAccountInfo", h.f.ClientState),
		"active guard, then the client after assembly")
	require.Equal(t, []string{"confirmed", "finalized"}, h.readCommitments(t, "getMultipleAccounts", layout.chunks[0]),
		"temporary rent, then absence after cleanup")

	ids, err := h.s.store.sessionIDs()
	require.NoError(t, err)
	doc, err := h.s.store.loadSession(ids[0])
	require.NoError(t, err)
	lookup := solana.MustPublicKeyFromBase58(*doc.Lookup)
	lookupReads := h.readCommitments(t, "getAccountInfo", lookup)
	require.NotEmpty(t, lookupReads)
	for _, commitment := range lookupReads {
		require.Equal(t, "confirmed", commitment, "lookup setup and deactivation read at confirmed")
	}

	// The lookup address derives from a finalized slot; warm-up polls confirmed.
	var slots []string
	h.mu.Lock()
	for _, call := range h.calls {
		if call.Method != "getSlot" {
			continue
		}
		var options struct {
			Commitment string `json:"commitment"`
		}
		require.NoError(t, json.Unmarshal(call.Params[0], &options))
		slots = append(slots, options.Commitment)
	}
	h.mu.Unlock()
	require.GreaterOrEqual(t, len(slots), 3)
	require.Equal(t, []string{"finalized", "finalized"}, slots[:2])
	for _, commitment := range slots[2:] {
		require.Equal(t, "confirmed", commitment)
	}
}

func TestReclaimReadCommitments(t *testing.T) {
	h := newHeaderJobFixture(t)
	_, address := h.lookupSession(t, "commitments", 100)
	require.NoError(t, h.s.reclaimLookups(context.Background()))
	require.Equal(t, []string{"lookup-close"}, h.stages)
	require.Equal(t, []string{"finalized", "confirmed", "finalized"}, h.readCommitments(t, "getAccountInfo", address),
		"table before closing, its rent, then its absence")
	require.Equal(t, []string{"confirmed"}, h.readCommitments(t, "getAccountInfo", solana.SysVarSlotHashesPubkey))
}

// Upload progress is read at finalized, and a funded
// address that is not a proof buffer stays a foreign account there.
func TestUploadProgressIsFinalizedAndStrict(t *testing.T) {
	h := newResolutionHarness(t)
	proof := h.proof(planningTransfer(1), 1700)
	id := proof.Transfer.ID
	job := h.job(proof)
	address, err := h.s.deployment.ProofBuffer(h.s.payer.PublicKey(), proof.Key)
	require.NoError(t, err)
	upload := func(p *payerSession) error {
		_, err := p.uploadProof(context.Background(), id, job)
		return err
	}
	require.NoError(t, runDurablePayer(t, h.s, upload))
	h.mu.Lock()
	reads := append([]string(nil), h.accountReads[address]...)
	h.mu.Unlock()
	require.NotEmpty(t, reads)
	for _, commitment := range reads {
		require.Equal(t, "finalized", commitment)
	}

	funded := newResolutionHarness(t)
	proof = funded.proof(planningTransfer(1), 1700)
	job = funded.job(proof)
	address, err = funded.s.deployment.ProofBuffer(funded.s.payer.PublicKey(), proof.Key)
	require.NoError(t, err)
	funded.accounts[address] = fundedUninitializedAccount()
	err = runDurablePayer(t, funded.s, func(p *payerSession) error {
		_, err := p.uploadProof(context.Background(), proof.Transfer.ID, job)
		return err
	})
	require.ErrorContains(t, err, "account owner mismatch")
	require.Empty(t, funded.stages)
}

func TestLookupCloseRequiresFinalizedAbsence(t *testing.T) {
	h := newHeaderJobFixture(t)
	_, address := h.lookupSession(t, "still-present", 100)
	before := h.accounts[address]
	// The mock lands the close, but the subsequent RPC read still reports
	// an account. A successful receipt alone does not satisfy the postcondition.
	h.changeResult = func(call headerJobCall, result any) any {
		if call.Method == "getAccountInfo" && bytes.Contains(call.Params[0], []byte(address.String())) && len(h.stages) > 0 {
			result.(map[string]any)["value"] = before
		}
		return result
	}

	err := h.s.reclaimLookups(context.Background())
	require.ErrorContains(t, err, "closed lookup account remains")
	require.Equal(t, []string{"lookup-close"}, h.stages)
	require.Equal(t, []string{"finalized", "confirmed", "finalized"}, h.readCommitments(t, "getAccountInfo", address))
}

func TestLookupCloseRequiresReadableSafeRent(t *testing.T) {
	for _, mode := range []string{"missing", "unsafe-integer", "maximum-safe"} {
		t.Run(mode, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			id, address := h.lookupSession(t, mode, 100)
			reads := 0
			h.changeResult = func(call headerJobCall, result any) any {
				if call.Method != "getAccountInfo" || !bytes.Contains(call.Params[0], []byte(address.String())) {
					return result
				}
				reads++
				if reads == 2 {
					value := result.(map[string]any)
					if mode == "missing" {
						value["value"] = nil
					} else {
						account := *h.accounts[address]
						account.Lamports = maxSafeInteger
						if mode == "unsafe-integer" {
							account.Lamports++
						}
						value["value"] = &account
					}
				}
				return result
			}

			var err error
			require.NotPanics(t, func() {
				err = h.s.reclaimLookups(context.Background())
			})
			doc, loadErr := h.s.store.loadSession(id)
			require.NoError(t, loadErr)
			if mode == "maximum-safe" {
				require.NoError(t, err)
				require.Equal(t, []string{"lookup-close"}, h.stages)
				require.NotNil(t, doc.LookupRentClosing)
				require.Equal(t, int64(maxSafeInteger), *doc.LookupRentClosing)
				return
			}
			require.ErrorContains(t, err, "lookup rent unavailable or exceeds safe integer")
			require.Empty(t, h.stages)
			require.Nil(t, doc.LookupRentClosing)
			require.Nil(t, doc.LookupClosed)
			require.Equal(t, []string{"finalized", "confirmed"}, h.readCommitments(t, "getAccountInfo", address))
		})
	}
}
