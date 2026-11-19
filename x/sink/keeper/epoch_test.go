package keeper

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	epochstypes "pkg.akt.dev/go/node/epochs/v1beta1"
	types "pkg.akt.dev/go/node/sink/v1"
)

func TestQuotaEpochFailureIsAtomic(t *testing.T) {
	readFailure := errors.New("epoch store read failed")
	for name, epoch := range map[string]testEpochs{
		"missing":      {err: collections.ErrNotFound},
		"read failure": {err: readFailure},
		"unstarted": {
			info: epochstypes.EpochInfo{
				ID:       "day",
				Duration: time.Minute,
			},
		},
		"wrong identifier": {
			info: epochstypes.EpochInfo{
				ID:                   "hour",
				Duration:             time.Minute,
				CurrentEpoch:         20,
				EpochCountingStarted: true,
			},
		},
		"zero duration": {
			info: epochstypes.EpochInfo{
				ID:                   "day",
				CurrentEpoch:         20,
				EpochCountingStarted: true,
			},
		},
		"negative duration": {
			info: epochstypes.EpochInfo{
				ID:                   "day",
				Duration:             -time.Minute,
				CurrentEpoch:         20,
				EpochCountingStarted: true,
			},
		},
		"negative number": {
			info: epochstypes.EpochInfo{
				ID:                   "day",
				Duration:             time.Minute,
				CurrentEpoch:         -1,
				EpochCountingStarted: true,
			},
		},
		"started zero": {
			info: epochstypes.EpochInfo{
				ID:                   "day",
				Duration:             time.Minute,
				EpochCountingStarted: true,
			},
		},
		"unstarted nonzero": {
			info: epochstypes.EpochInfo{
				ID:           "day",
				Duration:     time.Minute,
				CurrentEpoch: 20,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			g.Params.Enabled = true
			g.Params.RefundsEnabled = true
			g.Params.MinAmountPerTransfer = testCoin(1)
			importTestGenesis(t, k, ctx, bank, g)
			bank.set(ctx, testOwner(6), sdk.NewCoins(testCoin(100)))
			before, err := k.ExportGenesis(ctx)
			require.NoError(t, err)
			balance := bank.GetAllBalances(ctx, testOwner(6))
			escrow := bank.GetAllBalances(ctx, Address())
			events := ctx.EventManager().Events()
			validEpochs := k.epochs
			k.epochs = epoch
			pub := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, 32)).Public().(ed25519.PublicKey)
			var recipient types.SolanaAddress
			copy(recipient[:], pub)

			_, err = k.Deposit(ctx, testOwner(6), recipient, testCoin(1))
			expected := epoch.err
			if expected == nil {
				expected = types.ErrInvalidClock
			}

			require.ErrorIs(t, err, expected)
			err = k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{
				Executor: testOwner(6).String(),
				ID:       g.Records[6].ID,
			})
			require.ErrorIs(t, err, expected)
			_, err = k.NewQuerier().State(ctx, nil)
			require.ErrorIs(t, err, expected)
			_, err = k.NewQuerier().Account(ctx, &types.QueryAccountRequest{Owner: testOwner(6).String()})
			require.ErrorIs(t, err, expected)
			_, err = k.ExportGenesis(ctx)
			require.Error(t, err)
			k.epochs = validEpochs
			after, err := k.ExportGenesis(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, balance, bank.GetAllBalances(ctx, testOwner(6)))
			require.Equal(t, escrow, bank.GetAllBalances(ctx, Address()))
			require.Equal(t, events, ctx.EventManager().Events())
			sequence, err := k.sequence.Peek(ctx)
			require.NoError(t, err)
			require.Zero(t, sequence)
			id, err := k.Deposit(ctx, testOwner(6), recipient, testCoin(1))
			require.NoError(t, err)
			require.Equal(t, uint64(1), id.Sequence)

			fresh, freshCtx, freshBank := testStore(t)
			fresh.epochs = epoch
			freshBank.set(freshCtx, Address(), sdk.NewCoins(escrow...))
			require.Error(t, fresh.InitGenesis(freshCtx, g))
			it := freshCtx.KVStore(fresh.skey).Iterator(nil, nil)
			require.False(t, it.Valid())
			require.NoError(t, it.Close())
			require.Equal(t, escrow, freshBank.GetAllBalances(freshCtx, Address()))
		})
	}
}

func TestQuotaEpochEmptyInstallation(t *testing.T) {
	for _, epoch := range []struct {
		native int64
		query  uint64
	}{{0, 0}, {57, 57}} {
		k, ctx, bank := testStore(t)
		k.epochs = testEpochs{
			info: epochstypes.EpochInfo{
				ID:                   "day",
				Duration:             10 * time.Minute,
				StartTime:            ctx.BlockTime().Add(time.Hour),
				CurrentEpoch:         epoch.native,
				EpochCountingStarted: epoch.native > 0,
			},
		}
		g := types.DefaultGenesis()
		importTestGenesis(t, k, ctx, bank, g)
		state, err := k.NewQuerier().State(ctx, nil)
		require.NoError(t, err)
		require.Equal(t, epoch.query, state.Epoch)
		require.Equal(t, types.EpochUsage{
			Epoch:  epoch.query,
			Amount: types.ZeroAmount(),
		}, state.SendUsage)
		account, err := k.NewQuerier().Account(ctx, &types.QueryAccountRequest{Owner: testOwner(6).String()})
		require.NoError(t, err)
		require.Equal(t, epoch.query, account.Epoch)
		exported, err := k.ExportGenesis(ctx.WithBlockTime(time.Time{}))
		require.NoError(t, err)
		want, err := g.Marshal()
		require.NoError(t, err)
		got, err := exported.Marshal()
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func requireGenesisEqual(t *testing.T, expected, actual *types.GenesisState) {
	t.Helper()
	require.ElementsMatch(t, expected.Records, actual.Records)
	want, got := *expected, *actual
	want.Records, got.Records = nil, nil
	require.Equal(t, want, got)
}

func TestGenesisPreservesEpochCounters(t *testing.T) {
	k, ctx, bank := testStore(t)
	k.epochs = testEpochs{
		info: epochstypes.EpochInfo{
			ID:                   "day",
			Duration:             10 * time.Minute,
			CurrentEpoch:         50000,
			EpochCountingStarted: true,
		},
	}
	g := populatedGenesis()

	g.SendUsage.Epoch = 20759
	g.RefundUsage.Epoch = 20759
	for i := range g.Accounts {
		g.Accounts[i].Usage.LastEpoch = 20759
	}

	importTestGenesis(t, k, ctx, bank, g)
	exported, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	requireGenesisEqual(t, g, exported)
	epoch, err := k.getEpochID(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(50000), epoch)
}
