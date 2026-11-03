package keeper

import (
	"cosmossdk.io/collections"

	types "pkg.akt.dev/go/node/sink/v1"
)

var (
	ParamsKey        = collections.NewPrefix([]byte{0x01, 0x00})
	TransfersKey     = collections.NewPrefix([]byte{0x02, 0x00})
	SettlementsKey   = collections.NewPrefix([]byte{0x03, 0x01})
	TotalsKey        = collections.NewPrefix([]byte{0x04, 0x00})
	AccountsKey      = collections.NewPrefix([]byte{0x04, 0x01})
	SendUsageKey     = collections.NewPrefix([]byte{0x04, 0x02})
	RefundUsageKey   = collections.NewPrefix([]byte{0x04, 0x03})
	RefundQueueKey   = collections.NewPrefix([]byte{0x05, 0x00})
	SequenceKey      = collections.NewPrefix([]byte{0x06, 0x00})
	PoliciesKey      = collections.NewPrefix([]byte{0x07, 0x00})
	CurrentPolicyKey = collections.NewPrefix([]byte{0x07, 0x01})
	HistoryKey       = collections.NewPrefix([]byte{0x08, 0x00})
	DisputesKey      = collections.NewPrefix([]byte{0x08, 0x01})
)

func TransferKey(id types.ID) ([]byte, error) {
	if err := id.ValidateBasic(); err != nil {
		return nil, err
	}

	return collections.EncodeKeyWithPrefix(TransfersKey.Bytes(), RecordIDKey, id)
}

func SettlementKey(id types.SettlementID) ([]byte, error) {
	if err := id.ValidateBasic(); err != nil {
		return nil, err
	}

	return collections.EncodeKeyWithPrefix(SettlementsKey.Bytes(), SettlementIDKey, id)
}
