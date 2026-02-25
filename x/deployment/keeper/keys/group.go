package keys

import (
	"cosmossdk.io/collections"

	v1 "pkg.akt.dev/go/node/deployment/v1"
)

// GroupPrimaryKey is the composite primary key for a group: (owner, dseq, gseq)
type GroupPrimaryKey = collections.Triple[string, uint64, uint32]

// GroupPrimaryKeyCodec is the key codec for GroupPrimaryKey
var GroupPrimaryKeyCodec = collections.TripleKeyCodec(
	collections.StringKey,
	collections.Uint64Key,
	collections.Uint32Key,
)

// GroupIDToKey converts a v1.GroupID to a GroupPrimaryKey for use with the IndexedMap
func GroupIDToKey(id v1.GroupID) GroupPrimaryKey {
	return collections.Join3(id.Owner, id.DSeq, id.GSeq)
}

// KeyToGroupID converts a GroupPrimaryKey back to a v1.GroupID
func KeyToGroupID(key GroupPrimaryKey) v1.GroupID {
	return v1.GroupID{
		Owner: key.K1(),
		DSeq:  key.K2(),
		GSeq:  key.K3(),
	}
}
