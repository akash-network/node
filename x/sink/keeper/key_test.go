package keeper

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestProofStorageAndChronologicalCodecs(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	id := g.Records[7].ID
	key, err := types.TransferKey(id)
	require.NoError(t, err)
	value, err := g.Records[7].Data.Marshal()
	require.NoError(t, err)
	require.Equal(t, value, ctx.KVStore(k.skey).Get(key))
	key, err = types.SettlementKey(types.NewSettlementID(g.Records[7].Status, id))
	require.NoError(t, err)
	value, err = g.Records[7].Settlement.Marshal()
	require.NoError(t, err)
	require.Equal(t, value, ctx.KVStore(k.skey).Get(key))

	for n := 0; n < types.IDKeySize; n++ {
		_, err := chronoIDKey.Encode(make([]byte, n), id)
		require.Error(t, err)
		_, _, err = chronoIDKey.Decode(make([]byte, n))
		require.Error(t, err)
	}

	b := make([]byte, types.IDKeySize)
	n, err := chronoIDKey.Encode(b, id)
	require.NoError(t, err)
	require.Equal(t, len(b), n)
	_, got, err := chronoIDKey.Decode(b)
	require.NoError(t, err)
	require.Equal(t, id, got)
	require.Equal(t, uint64(7), binary.BigEndian.Uint64(b[:8]))
	require.Equal(t, uint64(8), binary.BigEndian.Uint64(b[8:16]))
}

func TestSettlementIDCodec(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	id := g.Records[7].ID
	key := append(SettlementsKey.Bytes(), byte(types.StatusRefundPending), 0x14)
	key = append(key, bytes.Repeat([]byte{7}, 20)...)
	key = binary.BigEndian.AppendUint64(key, 7)
	key = binary.BigEndian.AppendUint64(key, 8)
	require.True(t, ctx.KVStore(k.skey).Has(key), "canonical settlement stores status || IDKey")
	sid := types.NewSettlementID(types.StatusRefundPending, id)
	encoded := key[len(SettlementsKey.Bytes()):]
	for n := 0; n < types.SettlementIDKeySize; n++ {
		_, err := types.SettlementIDKey.Encode(make([]byte, n), sid)
		require.Error(t, err)
		_, _, err = types.SettlementIDKey.Decode(encoded[:n])
		require.Error(t, err)
	}

	for _, status := range []byte{0, 6} {
		_, err := types.SettlementIDKey.Encode(make([]byte, types.SettlementIDKeySize), types.NewSettlementID(types.Status(status), id))
		require.Error(t, err)
		unknown := bytes.Clone(encoded)
		unknown[0] = status
		_, _, err = types.SettlementIDKey.Decode(unknown)
		require.Error(t, err)
	}

	_, err := types.SettlementIDKey.Encode(make([]byte, types.SettlementIDKeySize), types.SettlementID{Status: types.StatusMinted, Owner: id.Owner, Sequence: 1})
	require.Error(t, err, "zero height")
	n, got, err := types.SettlementIDKey.Decode(encoded)
	require.NoError(t, err)
	require.Equal(t, types.SettlementIDKeySize, n)
	require.Equal(t, sid, got)
	last := types.ID{Owner: sdk.AccAddress(bytes.Repeat([]byte{0xff}, 20)).String(), Height: math.MaxInt64, Sequence: math.MaxUint64}
	first := types.ID{Owner: testOwner(1).String(), Height: 1, Sequence: 1}
	pending, minted := make([]byte, types.SettlementIDKeySize), make([]byte, types.SettlementIDKeySize)
	_, err = types.SettlementIDKey.Encode(pending, types.NewSettlementID(types.StatusTransferPending, last))
	require.NoError(t, err)
	_, err = types.SettlementIDKey.Encode(minted, types.NewSettlementID(types.StatusMinted, first))
	require.NoError(t, err)
	require.Negative(t, bytes.Compare(pending, minted), "status orders before owner, height and sequence")
}
