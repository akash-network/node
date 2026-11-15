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
				require.True(t, *doc.LookupClosed)
				require.True(t, *doc.LookupClosureUnobserved)
				require.Nil(t, doc.LookupRentReturned)
			} else {
				require.Nil(t, doc.LookupClosed)
			}
			require.Empty(t, h.stages)
		})
	}
}
