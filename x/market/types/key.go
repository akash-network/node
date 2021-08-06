package types

const (
	// ModuleName is the module name constant used in many places
	ModuleName = "market"

	// StoreKey is the store key string for market
	StoreKey = ModuleName

	// RouterKey is the message route for market
	RouterKey = ModuleName
)

var (
	OrderPrefix = []byte{0x01, 0x00}
	BidPrefix   = []byte{0x02, 0x00}
	LeasePrefix = []byte{0x03, 0x00}
)
