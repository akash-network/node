package keys

import (
	"cosmossdk.io/collections"

	mv1 "pkg.akt.dev/go/node/market/v1"
)

// BidPrimaryKey is the full composite primary key for a bid in the IndexedMap
type BidPrimaryKey = collections.Pair[OrderPrimaryKey, ProviderPartKey]

// BidPrimaryKeyCodec is the key codec for BidPrimaryKey, composed from stdlib codecs
var BidPrimaryKeyCodec = collections.PairKeyCodec(
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

// BidIDToKey converts a mv1.BidID to a BidPrimaryKey for use with the IndexedMap
func BidIDToKey(id mv1.BidID) BidPrimaryKey {
	return collections.Join(
		collections.Join4(id.Owner, id.DSeq, id.GSeq, id.OSeq),
		collections.Join(id.Provider, id.BSeq),
	)
}

// KeyToBidID converts a BidPrimaryKey back to a mv1.BidID
func KeyToBidID(key BidPrimaryKey) mv1.BidID {
	orderPart := key.K1()
	providerPart := key.K2()
	return mv1.BidID{
		Owner:    orderPart.K1(),
		DSeq:     orderPart.K2(),
		GSeq:     orderPart.K3(),
		OSeq:     orderPart.K4(),
		Provider: providerPart.K1(),
		BSeq:     providerPart.K2(),
	}
}
