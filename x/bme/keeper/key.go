package keeper

import (
	"cosmossdk.io/collections"
)

var (
	RemintCreditsKey = collections.NewPrefix([]byte{0x01, 0x00})
	TotalBurnedKey   = collections.NewPrefix([]byte{0x02, 0x01})
	TotalMintedKey   = collections.NewPrefix([]byte{0x02, 0x02})
	LedgerKey        = collections.NewPrefix([]byte{0x03, 0x00})
	ParamsKey        = collections.NewPrefix([]byte{0x09, 0x00}) // key for bme module params
)
