package keeper

import (
	"cosmossdk.io/collections"
)

const (
	epochMint = "mint"
	epochBurn = "burn"
)

var (
	RemintCreditsKey         = collections.NewPrefix([]byte{0x01, 0x00})
	TotalBurnedKey           = collections.NewPrefix([]byte{0x02, 0x01})
	TotalMintedKey           = collections.NewPrefix([]byte{0x02, 0x02})
	LedgerPendingKey         = collections.NewPrefix([]byte{0x03, 0x01})
	LedgerKey                = collections.NewPrefix([]byte{0x03, 0x02})
	LedgerPendingBalancesKey = collections.NewPrefix([]byte{0x03, 0x03})
	LedgerFailedKey          = collections.NewPrefix([]byte{0x03, 0x04})
	MintStatusKey            = collections.NewPrefix([]byte{0x04, 0x00})
	MintEpochKey             = collections.NewPrefix([]byte{0x04, 0x01})
	MintStatusRecordsKey     = collections.NewPrefix([]byte{0x04, 0x02})
	ParamsKey                = collections.NewPrefix([]byte{0x09, 0x00}) // key for bme module params
)
