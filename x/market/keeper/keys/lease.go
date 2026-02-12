package keys

import (
	"cosmossdk.io/collections"
	mv1 "pkg.akt.dev/go/node/market/v1"
)

// LeasePrimaryKey is the full composite primary key for a lease in the IndexedMap
type LeasePrimaryKey = collections.Pair[OrderPrimaryKey, ProviderPartKey]

// LeasePrimaryKeyCodec is the key codec for LeasePrimaryKey, composed from stdlib codecs
var LeasePrimaryKeyCodec = collections.PairKeyCodec(
	collections.QuadKeyCodec(
		collections.StringKey,
		collections.Uint64Key,
		collections.Uint32Key,
		collections.Uint32Key,
	),
	collections.PairKeyCodec(
		collections.StringKey,
		collections.Uint32Key,
	),
)

// LeaseIDToKey converts a mv1.LeaseID to a LeasePrimaryKey for use with the IndexedMap
func LeaseIDToKey(id mv1.LeaseID) LeasePrimaryKey {
	return collections.Join(
		collections.Join4(id.Owner, id.DSeq, id.GSeq, id.OSeq),
		collections.Join(id.Provider, id.BSeq),
	)
}

// KeyToLeaseID converts a LeasePrimaryKey back to a mv1.LeaseID
func KeyToLeaseID(key LeasePrimaryKey) mv1.LeaseID {
	orderPart := key.K1()
	providerPart := key.K2()
	return mv1.LeaseID{
		Owner:    orderPart.K1(),
		DSeq:     orderPart.K2(),
		GSeq:     orderPart.K3(),
		OSeq:     orderPart.K4(),
		Provider: providerPart.K1(),
		BSeq:     providerPart.K2(),
	}
}
