package keeper

import (
	"cosmossdk.io/collections"

	types "pkg.akt.dev/go/node/sink/v1"
)

// Store layout. Two-byte prefixes group related collections, as in x/bme, and
// every integer key is a fixed-width big-endian value, so iteration follows
// numeric order. Solana proves transfer records and void orders, so their
// prefixes and key encoding come from the SDK, where Solana clients read
// them; changing either breaks every Solana proof. The open and refund-queue
// indexes are derived: InitGenesis rebuilds them and export omits them.
var (
	ParamsKey = collections.NewPrefix([]byte{0x01, 0x00})
	TotalsKey = collections.NewPrefix([]byte{0x01, 0x01})

	TransfersKey = collections.NewPrefix(types.TransferKeyPrefix())
	VoidsKey     = collections.NewPrefix(types.VoidKeyPrefix())

	SettlementsKey   = collections.NewPrefix([]byte{0x03, 0x00})
	OpenTransfersKey = collections.NewPrefix([]byte{0x03, 0x01})
	RefundQueueKey   = collections.NewPrefix([]byte{0x03, 0x02})

	AccountsKey = collections.NewPrefix([]byte{0x04, 0x00})
)
