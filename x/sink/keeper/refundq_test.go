package keeper

import (
	"testing"

	"cosmossdk.io/collections"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestRefundQueueRetainsPolicyAndGovernanceScopes(t *testing.T) {
	k, ctx, bank := testStore(t)
	genesis := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, genesis)

	iterator, err := k.refunds.Iterate(ctx, nil)
	require.NoError(t, err)
	keys, err := iterator.Keys()
	require.NoError(t, err)
	require.NoError(t, iterator.Close())
	require.ElementsMatch(t, []collections.Triple[uint64, int64, types.ID]{
		collections.Join3(uint64(1), genesis.Records[5].Settlement.ChallengeEnd, genesis.Records[5].ID),
		collections.Join3(uint64(2), genesis.Records[6].Settlement.ChallengeEnd, genesis.Records[6].ID),
		collections.Join3(types.GovernanceRefundScope, genesis.Records[7].Settlement.ChallengeEnd, genesis.Records[7].ID),
	}, keys)
}
