package keys

import (
	"cosmossdk.io/collections"

	mv1 "pkg.akt.dev/go/node/market/v1"
)

// OrderIDToKey converts a mv1.OrderID to an OrderPrimaryKey for use with the IndexedMap
func OrderIDToKey(id mv1.OrderID) OrderPrimaryKey {
	return collections.Join4(id.Owner, id.DSeq, id.GSeq, id.OSeq)
}

// KeyToOrderID converts an OrderPrimaryKey back to a mv1.OrderID
func KeyToOrderID(key OrderPrimaryKey) mv1.OrderID {
	return mv1.OrderID{
		Owner: key.K1(),
		DSeq:  key.K2(),
		GSeq:  key.K3(),
		OSeq:  key.K4(),
	}
}
