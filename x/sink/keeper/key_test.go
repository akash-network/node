package keeper

import (
	"testing"

	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

// Solana builds these keys itself to verify proofs, so the stored bytes are
// pinned here rather than derived from the helpers under test.
func TestProvenKeysAreFixedWidthBigEndian(t *testing.T) {
	k, ctx, key := openTestStore(t)
	const id = 0x0102030405060708
	putTransfer(t, k, ctx, id, types.Status_STATUS_PENDING)
	require.NoError(t, k.voids.Set(ctx, id, types.VoidValue(id)))
	record, err := types.Transfer{ID: id, Recipient: [32]byte{1}, Amount: 1, Height: 1}.Marshal()
	require.NoError(t, err)

	transferKey := []byte{0x02, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	voidKey := []byte{0x02, 0x01, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	store := ctx.KVStore(key)
	require.Equal(t, record, store.Get(transferKey))
	require.Equal(t, types.VoidValue(id), store.Get(voidKey))
	require.Equal(t, transferKey, types.TransferKey(id))
	require.Equal(t, voidKey, types.VoidKey(id))
}
