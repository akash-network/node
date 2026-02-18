package keys

import (
	"cosmossdk.io/collections"
)

// OrderPrimaryKey represents the order portion of a BidID: (owner, dseq, gseq, oseq)
type OrderPrimaryKey = collections.Quad[string, uint64, uint32, uint32]

// ProviderPartKey represents the provider portion of a BidID: (provider, bseq)
type ProviderPartKey = collections.Pair[string, uint32]

// GroupPartKey represents (owner, dseq, gseq) for group-based index lookups
type GroupPartKey = collections.Triple[string, uint64, uint32]

// OrderPrimaryKeyCodec is the key codec for OrderPrimaryKey
var OrderPrimaryKeyCodec = collections.QuadKeyCodec(
	collections.StringKey,
	collections.Uint64Key,
	collections.Uint32Key,
	collections.Uint32Key,
)
