package v1beta1

const (
	// ModuleName is the module name constant used in many places
	ModuleName = "market"

	// StoreKey is the store key string for market
	StoreKey = ModuleName

	// RouterKey is the message route for market
	RouterKey = ModuleName
)

func OrderPrefix() []byte {
	return []byte{0x01, 0x00}
}

func BidPrefix() []byte {
	return []byte{0x02, 0x00}
}

func LeasePrefix() []byte {
	return []byte{0x03, 0x00}
}
