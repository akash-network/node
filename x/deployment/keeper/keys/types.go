package keys

import (
	"cosmossdk.io/collections"
)

// DeploymentPrimaryKey is the composite primary key for a deployment: (owner, dseq)
type DeploymentPrimaryKey = collections.Pair[string, uint64]

// DeploymentPrimaryKeyCodec is the key codec for DeploymentPrimaryKey
var DeploymentPrimaryKeyCodec = collections.PairKeyCodec(
	collections.StringKey,
	collections.Uint64Key,
)
