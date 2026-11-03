package keeper

import (
	"bytes"
	"crypto/ed25519"
	"math"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func depositContext(ctx sdk.Context, msg *types.MsgSink) sdk.Context {
	return ctx.WithValue(sinkRequestContextKey{}, sinkRequest{
		sender:   msg.Sender,
		destType: msg.DestType,
		dest:     msg.Dest,
		denom:    msg.Amount.Denom,
		amount:   msg.Amount.Amount.String(),
	})
}

func TestAllocatorAtomicDepositAndReset(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := types.DefaultGenesis()
	g.Params.MinAmountPerTransfer = testCoin(1)
	g.Params.Enabled = true
	for n := byte(1); n <= 4; n++ {
		g.Params.Reporters = append(g.Params.Reporters, testOwner(n).String())
	}

	g.PolicySequence = 1
	g.Policies = []types.ReporterPolicy{
		{
			Version:   g.PolicySequence,
			Members:   g.Params.Reporters,
			Threshold: g.Params.ReportersThreshold,
		},
	}
	importTestGenesis(t, k, ctx, bank, g)
	pub := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, 32)).Public().(ed25519.PublicKey)
	var recipient types.SolanaAddress
	copy(recipient[:], pub)
	msg := &types.MsgSink{
		Sender:   testOwner(6).String(),
		DestType: types.DestTypeSolana,
		Dest:     recipient.String(),
		Amount:   testCoin(10),
	}

	bank.set(ctx, testOwner(6), sdk.NewCoins(testCoin(100)))
	bank.set(ctx, testOwner(7), sdk.NewCoins(testCoin(100)))

	for _, name := range []string{"insufficient", "failure", "redirect"} {
		t.Run(name, func(t *testing.T) {
			m := *msg
			switch name {
			case "insufficient":
				m.Amount = testCoin(101)
			case "failure":
				bank.fail = true
			case "redirect":
				bank.redirect = testOwner(8)
			}

			before, err := k.ExportGenesis(ctx)
			require.NoError(t, err)
			events := ctx.EventManager().Events()
			balance := bank.GetAllBalances(ctx, testOwner(6))
			_, err = k.Deposit(depositContext(ctx, &m), &m)
			require.Error(t, err)
			after, err := k.ExportGenesis(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, events, ctx.EventManager().Events())
			require.Equal(t, balance, bank.GetAllBalances(ctx, testOwner(6)))
			n, err := k.sequence.Peek(ctx)
			require.NoError(t, err)
			require.Zero(t, n)
			bank.fail = false
			bank.redirect = nil
		})
	}

	id, err := k.Deposit(depositContext(ctx, msg), msg)
	require.NoError(t, err)
	require.Equal(t, uint64(1), id.Sequence)
	msg.Sender = testOwner(7).String()
	id2, err := k.Deposit(depositContext(ctx, msg), msg)
	require.NoError(t, err)
	require.Equal(t, uint64(2), id2.Sequence)
	require.NoError(t, k.sequence.Set(ctx, math.MaxUint64))
	_, err = k.Deposit(depositContext(ctx, msg), msg)
	require.ErrorIs(t, err, types.ErrOverflow)
	require.NoError(t, k.BeginBlocker(ctx))
	ctx = ctx.WithBlockHeight(101)
	id3, err := k.Deposit(depositContext(ctx, msg), msg)
	require.NoError(t, err)
	require.Equal(t, uint64(1), id3.Sequence)
	require.Equal(t, int64(101), id3.Height)
	require.NoError(t, k.BeginBlocker(ctx))
	_, err = k.Deposit(depositContext(ctx, msg), msg)
	require.ErrorIs(t, err, types.ErrTransferExists)
}

func TestSenderAndPolicyEligibility(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := types.DefaultGenesis()
	g.Params.MinAmountPerTransfer = testCoin(1)
	g.Params.Enabled = true
	for n := byte(1); n <= 4; n++ {
		g.Params.Reporters = append(g.Params.Reporters, testOwner(n).String())
	}

	g.PolicySequence = 1
	g.Policies = []types.ReporterPolicy{
		{
			Version:   g.PolicySequence,
			Members:   g.Params.Reporters,
			Threshold: g.Params.ReportersThreshold,
		},
	}
	importTestGenesis(t, k, ctx, bank, g)
	accounts := k.accounts.(genesisAccounts)
	base := authtypes.NewBaseAccountWithAddress(testOwner(6))
	accounts[testOwner(6).String()] = authtypes.NewModuleAccount(base, "reserved")
	msg := &types.MsgSink{
		Sender:   testOwner(6).String(),
		DestType: types.DestTypeSolana,
		Dest:     types.SolanaAddress{1}.String(),
		Amount:   testCoin(1),
	}
	_, err := k.Deposit(depositContext(ctx, msg), msg)
	require.ErrorIs(t, err, types.ErrSenderNotSigningAccount)
	accounts[testOwner(6).String()] = base
	_, err = k.Deposit(depositContext(ctx, msg), msg)
	require.ErrorIs(t, err, types.ErrSenderNotSigningAccount)
	delete(accounts, testOwner(6).String())
	_, err = k.Deposit(depositContext(ctx, msg), msg)
	require.ErrorIs(t, err, types.ErrSenderNotSigningAccount)
	p := g.Params
	p.Reporters = []string{g.Params.Reporters[3], g.Params.Reporters[2], g.Params.Reporters[1], g.Params.Reporters[0]}
	p.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.7")
	require.NoError(t, k.SetParams(ctx, p))

	policy, err := k.GetCurrentPolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(2), policy.Version)
	p.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.8")
	require.NoError(t, k.SetParams(ctx, p))

	policy, err = k.GetCurrentPolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(3), policy.Version)
	require.True(t, p.ReportersThreshold.Equal(policy.Threshold))
	accounts[testOwner(6).String()] = authtypes.NewModuleAccount(base, "reserved")
	p.Reporters[0] = testOwner(6).String()
	require.Error(t, k.SetParams(ctx, p))
}
