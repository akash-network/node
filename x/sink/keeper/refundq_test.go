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

func TestSettlementWriteAuthenticatesGovernanceVeto(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)

	// Records[8] holds a reporter-scope veto; rescoping it to governance
	// requires the configured authority as its signer.
	before, err := k.GetTransfer(ctx, g.Records[8].ID)
	require.NoError(t, err)
	require.Equal(t, types.HoldVeto, before.Settlement.Hold)

	after := before
	after.Settlement.VetoPolicyVersion = types.GovernanceRefundScope
	after.Settlement.VetoBy = testOwner(1).String()
	err = k.put(ctx, &before, after)
	require.ErrorIs(t, err, types.ErrInvalidSettlement)
	require.ErrorContains(t, err, "governance veto signer differs from authority")

	after.Settlement.VetoBy = k.authority
	require.NoError(t, k.put(ctx, &before, after))
	stored, err := k.GetTransfer(ctx, g.Records[8].ID)
	require.NoError(t, err)
	require.Equal(t, k.authority, stored.Settlement.VetoBy)
}
