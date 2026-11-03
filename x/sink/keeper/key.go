package keeper

import (
	"cosmossdk.io/collections"

	types "pkg.akt.dev/go/node/sink/v1"
)

var (
	ParamsKey      = collections.NewPrefix([]byte{0x11, 0x00})
	TotalsKey      = collections.NewPrefix([]byte{0x11, 0x01})
	PoliciesKey    = collections.NewPrefix([]byte{0x11, 0x02})
	SendUsageKey   = collections.NewPrefix([]byte{0x11, 0x03})
	RefundUsageKey = collections.NewPrefix([]byte{0x11, 0x04})
	TransfersKey   = collections.NewPrefix(types.TransferKeyPrefix)
	VoidsKey       = collections.NewPrefix(types.VoidOrderKeyPrefix)
	SettlementsKey = collections.NewPrefix([]byte{0x13, 0x00})
	StatusKey      = collections.NewPrefix([]byte{0x13, 0x01})
	HistoryKey     = collections.NewPrefix([]byte{0x13, 0x02})
	DisputesKey    = collections.NewPrefix([]byte{0x13, 0x03})
	RefundQueueKey = collections.NewPrefix([]byte{0x13, 0x04})
	AccountsKey    = collections.NewPrefix([]byte{0x14, 0x00})
	SequenceKey    = collections.NewPrefix([]byte{0x01})
)
