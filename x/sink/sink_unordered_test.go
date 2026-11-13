package sink_test

import (
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsign "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/stretchr/testify/require"

	sinktypes "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"
)

const unorderedFee = 10

// unorderedTx is the unordered variant of txFromMode: sequence 0, SIGN_MODE_DIRECT and the given timeout timestamp.
func (h *testSuite) unorderedTx(t *testing.T, key *secp256k1.PrivKey, timeout time.Time, memo string, msgs ...sdk.Msg) []byte {
	t.Helper()
	sender := sdk.AccAddress(key.PubKey().Address())
	cfg := h.a.TxConfig()
	b := cfg.NewTxBuilder()
	require.NoError(t, b.SetMsgs(msgs...))
	b.SetMemo(memo)
	b.SetGasLimit(500_000)
	b.SetFeeAmount(sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, unorderedFee)))
	b.SetUnordered(true)
	b.SetTimeoutTimestamp(timeout)
	account := h.a.Keepers.Cosmos.Acct.GetAccount(h.ctx(), sender)
	sig := signing.SignatureV2{
		PubKey:   key.PubKey(),
		Data:     &signing.SingleSignatureData{SignMode: signing.SignMode_SIGN_MODE_DIRECT},
		Sequence: 0,
	}

	require.NoError(t, b.SetSignatures(sig))
	signBytes, err := authsign.GetSignBytesAdapter(context.Background(), cfg.SignModeHandler(), signing.SignMode_SIGN_MODE_DIRECT,
		authsign.SignerData{
			Address:       sender.String(),
			ChainID:       h.a.ChainID(),
			AccountNumber: account.GetAccountNumber(),
			Sequence:      0,
			PubKey:        key.PubKey(),
		}, b.GetTx())
	require.NoError(t, err)
	signature, err := key.Sign(signBytes)
	require.NoError(t, err)
	sig.Data.(*signing.SingleSignatureData).Signature = signature
	require.NoError(t, b.SetSignatures(sig))
	bz, err := cfg.TxEncoder()(b.GetTx())
	require.NoError(t, err)

	return bz
}

type replayCase struct {
	h       *testSuite
	first   []byte
	second  []byte
	timeout time.Time
	before  sdk.Coin
}

// newReplayCase deposits transfer 1 and builds two unordered votes of reporter 0 for it that share one timeout
// but differ in memo, so their bytes differ.
func newReplayCase(t *testing.T, timeoutAfter time.Duration) *replayCase {
	t.Helper()
	h := setupReportingTestSuite(t)
	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)

	msg := h.reportMsg(t, 0, 1, sinktypes.OutcomeMinted)
	timeout := h.now.Add(timeoutAfter)
	first := h.unorderedTx(t, h.reporters[0], timeout, "first", msg)
	second := h.unorderedTx(t, h.reporters[0], timeout, "second", msg)
	require.NotEqual(t, first, second)

	return &replayCase{
		h:       h,
		first:   first,
		second:  second,
		timeout: timeout,
		before:  h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), h.reporter0(), sdkutil.DenomUakt),
	}
}

func (h *testSuite) reporter0() sdk.AccAddress {
	return sdk.AccAddress(h.reporters[0].PubKey().Address())
}

func requireReplayRefused(t *testing.T, res *abci.ExecTxResult) {
	t.Helper()
	require.Equal(t, sdkerrors.RootCodespace, res.Codespace, res.Log)
	require.Equal(t, sdkerrors.ErrInvalidRequest.ABCICode(), res.Code, res.Log)
	require.Contains(t, res.Log, "failed to add unordered nonce")
}

// requireOneExecution checks that the reporter paid exactly one fee and that the transfer holds its vote once.
func (c *replayCase) requireOneExecution(t *testing.T) {
	t.Helper()
	h := c.h
	after := h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), h.reporter0(), sdkutil.DenomUakt)
	require.Equal(t, c.before.SubAmount(sdkmath.NewInt(unorderedFee)), after)

	votes := 0
	for _, vote := range h.transfer(t, 1).Settlement.Votes {
		if vote.Reporter == h.reporterAddress(0) {
			votes++
		}
	}
	require.Equal(t, 1, votes)
}

func TestUnorderedReplayKey(t *testing.T) {
	t.Run("same block", func(t *testing.T) {
		c := newReplayCase(t, time.Minute)
		res := c.h.commitBlock(t, c.h.now.Add(time.Second), c.first, c.second)
		require.Zero(t, res[0].Code, res[0].Log)
		requireReplayRefused(t, res[1])
		c.requireOneExecution(t)
	})

	t.Run("consecutive blocks", func(t *testing.T) {
		c := newReplayCase(t, time.Minute)
		res := c.h.deliverBytes(t, c.first)
		require.Zero(t, res.Code, res.Log)
		requireReplayRefused(t, c.h.deliverBytes(t, c.second))
		c.requireOneExecution(t)
	})
}

func TestUnorderedReplayKeyBoundary(t *testing.T) {
	t.Skip("the Akash cosmos-sdk fork accepts a timeout equal to the block time; un-skip once it adopts cosmos/cosmos-sdk#26672 (spec §14)")

	c := newReplayCase(t, time.Minute)
	res := c.h.deliverBytes(t, c.first)
	require.Zero(t, res.Code, res.Log)
	res = c.h.commitBlock(t, c.timeout, c.second)[0]
	require.NotZero(t, res.Code, res.Log)
	c.requireOneExecution(t)
}
