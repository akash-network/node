package relayer

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

func (h *headerJobRPC) lookupSession(t *testing.T, label string, deactivation uint64) (string, solana.PublicKey) {
	t.Helper()
	session := sha256.Sum256([]byte(label))
	id := hex.EncodeToString(session[:])
	_, address, err := bridge.CreateLookup(h.s.payer.PublicKey(), h.s.payer.PublicKey(), uint64(len(h.tables)+1))
	require.NoError(t, err)
	h.tables[address] = solana.PublicKeySlice{}
	h.accounts[address] = headerJobAccount(solana.AddressLookupTableProgramID, h.lookupData(address, deactivation, 1, h.s.payer.PublicKey()))
	text, slot := address.String(), int64(1)
	require.NoError(t, h.s.store.saveSession(id, &sessionDoc{
		Version:    1,
		Genesis:    h.s.genesis,
		Payer:      h.s.payer.PublicKey().String(),
		Client:     h.f.Program.String(),
		Target:     11,
		Trusted:    10,
		Session:    id,
		Lookup:     &text,
		LookupSlot: &slot,
		Extra: map[string]json.RawMessage{
			"future": json.RawMessage(`{"keep":true}`),
		},
	}))
	data := binary.LittleEndian.AppendUint64(nil, 2)
	data = binary.LittleEndian.AppendUint64(data, 300)
	data = append(data, make([]byte, 32)...)
	data = binary.LittleEndian.AppendUint64(data, 200)
	data = append(data, make([]byte, 32)...)
	h.accounts[solana.SysVarSlotHashesPubkey] = headerJobAccount(solana.SystemProgramID, data)
	return id, address
}

func TestHeaderJobLookupClosureAndLimit(t *testing.T) {
	h := newHeaderJobFixture(t)
	id, address := h.lookupSession(t, "first", 100)
	id2, address2 := h.lookupSession(t, "second", 100)
	require.NoError(t, h.s.reclaimLookups(context.Background()))
	require.Equal(t, []string{"lookup-close"}, h.stages)
	require.NoError(t, h.s.reclaimLookups(context.Background()))
	require.Equal(t, 2, len(h.stages))
	for _, session := range []string{id, id2} {
		doc, err := h.s.store.loadSession(session)
		require.NoError(t, err)
		require.True(t, *doc.LookupClosed)
		require.Equal(t, int64(123), *doc.LookupRentReturned)
		require.Equal(t, json.RawMessage(`{"keep":true}`), doc.Extra["future"])
	}
	require.Nil(t, h.accounts[address])
	require.Nil(t, h.accounts[address2])
	require.Equal(t, 2, strings.Count(h.out.String(), `"event":"lookup-rent-returned"`))
}

func TestHeaderJobLookupFailureAndUnknownReplay(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			h := newHeaderJobFixture(t)
			id, _ := h.lookupSession(t, "retry", 100)
			if lost {
				h.lostStage = "lookup-close"
				require.Error(t, h.s.reclaimLookups(context.Background()))
				h.lostStage = ""
			} else {
				h.failStage = "lookup-close"
				h.failure = map[string]any{
					"InstructionError": []any{0, "InvalidArgument"},
				}
				require.NoError(t, h.s.reclaimLookups(context.Background()))
				doc, err := h.s.store.loadSession(id)
				require.NoError(t, err)
				require.NotNil(t, doc.LookupCloseFailure)
				require.Nil(t, doc.LookupClosed)
				h.failStage = ""
			}
			require.NoError(t, h.s.reclaimLookups(context.Background()))
			doc, err := h.s.store.loadSession(id)
			require.NoError(t, err)
			require.True(t, *doc.LookupClosed)
			require.Equal(t, int64(123), *doc.LookupRentReturned)
			if lost {
				require.Len(t, h.stages, 1)
			} else {
				require.Len(t, h.stages, 2)
				require.NotNil(t, doc.LookupCloseFailure)
			}
		})
	}
}

func TestHeaderJobLookupCooldownAbsenceAndOutage(t *testing.T) {
	for _, mode := range []string{"active", "cooling", "absent", "outage", "authority", "genesis"} {
		t.Run(mode, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			deactivation := uint64(200)
			if mode == "active" {
				deactivation = math.MaxUint64
			}
			id, address := h.lookupSession(t, mode, deactivation)
			switch mode {
			case "absent":
				delete(h.accounts, address)
			case "outage":
				h.failAddress = address
			case "authority":
				h.accounts[address] = headerJobAccount(solana.AddressLookupTableProgramID, h.lookupData(address, 100, 1, h.f.Program))
			case "genesis":
				h.genesis = solana.Hash(sha256.Sum256([]byte("changed"))).String()
			}
			err := h.s.reclaimLookups(context.Background())
			if mode == "outage" || mode == "authority" || mode == "genesis" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			doc, err := h.s.store.loadSession(id)
			require.NoError(t, err)
			if mode == "absent" {
				require.Nil(t, doc.LookupClosed)
				require.True(t, *doc.LookupClosureUnobserved)
				require.Nil(t, doc.LookupRentReturned)
			} else {
				require.Nil(t, doc.LookupClosed)
			}
			require.Empty(t, h.stages)
		})
	}
}

func TestHeaderJobLookupObservedClosureRemainsTerminal(t *testing.T) {
	h := newHeaderJobFixture(t)
	id, address := h.lookupSession(t, "observed", 100)
	require.NoError(t, h.s.reclaimLookups(context.Background()))
	doc, err := h.s.store.loadSession(id)
	require.NoError(t, err)
	require.NotNil(t, doc.LookupCloseTransaction)
	unobserved := true
	doc.LookupClosureUnobserved = &unobserved
	require.NoError(t, h.s.store.saveSession(id, doc))
	before, err := json.Marshal(doc)
	require.NoError(t, err)
	h.failAddress = address
	require.NoError(t, h.s.reclaimLookups(context.Background()))
	doc, err = h.s.store.loadSession(id)
	require.NoError(t, err)
	after, err := json.Marshal(doc)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, []string{"lookup-close"}, h.stages)
}

func TestHeaderJobLookupUnobservedClosureSafety(t *testing.T) {
	for _, mode := range []string{"absent", "active", "cooling", "outage", "authority", "failed-close", "lost-close"} {
		t.Run(mode, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			deactivation := uint64(100)
			if mode == "active" {
				deactivation = math.MaxUint64
			} else if mode == "cooling" {
				deactivation = 200
			}
			id, address := h.lookupSession(t, mode, deactivation)
			doc, err := h.s.store.loadSession(id)
			require.NoError(t, err)
			closed := true
			doc.LookupClosed, doc.LookupClosureUnobserved = &closed, &closed
			require.NoError(t, h.s.store.saveSession(id, doc))
			switch mode {
			case "absent":
				delete(h.accounts, address)
			case "outage":
				h.failAddress = address
			case "authority":
				h.accounts[address] = headerJobAccount(solana.AddressLookupTableProgramID, h.lookupData(address, 100, 1, h.f.Program))
			case "failed-close":
				h.failStage = "lookup-close"
				h.failure = map[string]any{
					"InstructionError": []any{0, "InvalidArgument"},
				}
			case "lost-close":
				h.lostStage = "lookup-close"
			}
			err = h.s.reclaimLookups(context.Background())
			if mode == "outage" || mode == "authority" || mode == "lost-close" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			doc, err = h.s.store.loadSession(id)
			require.NoError(t, err)
			require.Nil(t, doc.LookupCloseTransaction)
			require.Nil(t, doc.LookupRentReturned)
			require.Equal(t, json.RawMessage(`{"keep":true}`), doc.Extra["future"])
			if mode == "outage" || mode == "authority" {
				require.True(t, *doc.LookupClosed)
				require.True(t, *doc.LookupClosureUnobserved)
			} else {
				require.Nil(t, doc.LookupClosed)
			}
			if mode != "failed-close" && mode != "lost-close" {
				require.Empty(t, h.stages)
				return
			}
			h.failStage, h.lostStage = "", ""
			require.NoError(t, h.s.reclaimLookups(context.Background()))
			doc, err = h.s.store.loadSession(id)
			require.NoError(t, err)
			require.True(t, *doc.LookupClosed)
			require.Nil(t, doc.LookupClosureUnobserved)
			require.Equal(t, int64(123), *doc.LookupRentReturned)
			require.Equal(t, 1, strings.Count(h.out.String(), `"event":"lookup-rent-returned"`))
			if mode == "lost-close" {
				require.Len(t, h.stages, 1)
			} else {
				require.Len(t, h.stages, 2)
			}
		})
	}
}

func TestHeaderJobLookupRentAfterExpiredCreation(t *testing.T) {
	for _, reallocate := range []bool{false, true} {
		t.Run(fmt.Sprint(reallocate), func(t *testing.T) {
			ctx := context.Background()
			h := newHeaderJobFixture(t)
			h.active(t)
			h.dropStage = "lookup-create"
			h.changeResult = func(call headerJobCall, result any) any {
				if call.Method == "getLatestBlockhash" {
					result.(map[string]any)["value"].(map[string]any)["blockhash"] = expiryTestHash(850).String()
				}
				return result
			}
			require.Error(t, h.s.resumeHeader(ctx, nil))
			ids, err := h.s.store.sessionIDs()
			require.NoError(t, err)
			require.Len(t, ids, 1)
			id := ids[0]
			path, err := h.s.store.sessionPendingPath(id)
			require.NoError(t, err)
			pending, err := h.s.store.loadIntent(path)
			require.NoError(t, err)
			require.NotNil(t, pending)
			require.Equal(t, "lookup-create", pending.Stage)

			require.NoError(t, h.s.reclaimLookups(ctx))
			doc, err := h.s.store.loadSession(id)
			require.NoError(t, err)
			originalAddress := *doc.Lookup
			// Preserve the exact unobserved-closure state written by TS and RC0.
			closed := true
			doc.LookupClosed = &closed
			doc.LookupClosureUnobserved = &closed
			require.NoError(t, h.s.store.saveSession(id, doc))

			f := newExpiryFixture(t, "lookup-create")
			client, endpoint := h.s.solana, h.s.cfg.solanaRPC
			now, sleep := h.s.deps.now, h.s.deps.sleep
			h.s.solana, h.s.cfg.solanaRPC = f.service.solana, f.service.cfg.solanaRPC
			h.s.deps.now, h.s.deps.sleep = f.service.deps.now, f.service.deps.sleep
			f.doc, f.path, f.service = *pending, path, h.s
			require.True(t, isIntentNotIncluded(h.s.resumeHeader(ctx, nil)))
			require.Zero(t, f.calls["sendTransaction"])
			require.Empty(t, h.stages)
			remaining, err := h.s.store.loadIntent(path)
			require.NoError(t, err)
			require.Nil(t, remaining)
			h.s.solana, h.s.cfg.solanaRPC = client, endpoint
			h.s.deps.now, h.s.deps.sleep = now, sleep

			h.dropStage = ""
			h.changeResult = nil
			if reallocate {
				h.slot += 600
			}
			require.NoError(t, h.s.resumeHeader(ctx, nil))
			doc, err = h.s.store.loadSession(id)
			require.NoError(t, err)
			require.NotNil(t, doc.Assembly)
			address := solana.MustPublicKeyFromBase58(*doc.Lookup)
			require.Equal(t, reallocate, originalAddress != address.String())
			require.NotNil(t, h.accounts[address])
			rent := int64(h.accounts[address].Lamports)
			data := binary.LittleEndian.AppendUint64(nil, 1)
			data = binary.LittleEndian.AppendUint64(data, h.slot+1)
			data = append(data, make([]byte, 32)...)
			h.accounts[solana.SysVarSlotHashesPubkey] = headerJobAccount(solana.SystemProgramID, data)

			require.NoError(t, h.s.reclaimLookups(ctx))
			require.Nil(t, h.accounts[address], "recovered header lookup rent must be reclaimed")
			doc, err = h.s.store.loadSession(id)
			require.NoError(t, err)
			require.True(t, *doc.LookupClosed)
			require.Nil(t, doc.LookupClosureUnobserved)
			require.Equal(t, rent, *doc.LookupRentReturned)
			require.NotEmpty(t, *doc.LookupCloseTransaction)
			before, err := json.Marshal(doc)
			require.NoError(t, err)
			require.NoError(t, h.s.reclaimLookups(ctx))
			doc, err = h.s.store.loadSession(id)
			require.NoError(t, err)
			after, err := json.Marshal(doc)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, 1, strings.Count(h.out.String(), `"event":"lookup-rent-returned"`))
		})
	}
}
