package sink_test

import (
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsign "github.com/cosmos/cosmos-sdk/x/auth/signing"
	gogoproto "github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	sinktypes "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
	reportertx "pkg.akt.dev/node/v2/operator/reporter/tx"
)

const unorderedFee = 10

// signedTx is the signing core shared by unorderedTx and depositTx: a SIGN_MODE_DIRECT transaction of key at the
// given sequence, which configure may adjust (for example to mark it unordered) before it is signed.
func (h *testSuite) signedTx(t *testing.T, key *secp256k1.PrivKey, sequence uint64, memo string, msgs []sdk.Msg, configure func(client.TxBuilder)) []byte {
	t.Helper()
	sender := sdk.AccAddress(key.PubKey().Address())
	cfg := h.a.TxConfig()
	b := cfg.NewTxBuilder()
	require.NoError(t, b.SetMsgs(msgs...))
	b.SetMemo(memo)
	b.SetGasLimit(500_000)
	b.SetFeeAmount(sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, unorderedFee)))
	if configure != nil {
		configure(b)
	}

	account := h.a.Keepers.Cosmos.Acct.GetAccount(h.ctx(), sender)
	sig := signing.SignatureV2{
		PubKey:   key.PubKey(),
		Data:     &signing.SingleSignatureData{SignMode: signing.SignMode_SIGN_MODE_DIRECT},
		Sequence: sequence,
	}

	require.NoError(t, b.SetSignatures(sig))
	signBytes, err := authsign.GetSignBytesAdapter(context.Background(), cfg.SignModeHandler(), signing.SignMode_SIGN_MODE_DIRECT,
		authsign.SignerData{
			Address:       sender.String(),
			ChainID:       h.a.ChainID(),
			AccountNumber: account.GetAccountNumber(),
			Sequence:      sequence,
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

// unorderedTx is the unordered variant of txFromMode: sequence 0, SIGN_MODE_DIRECT and the given timeout timestamp.
func (h *testSuite) unorderedTx(t *testing.T, key *secp256k1.PrivKey, timeout time.Time, memo string, msgs ...sdk.Msg) []byte {
	t.Helper()

	return h.signedTx(t, key, 0, memo, msgs, func(b client.TxBuilder) {
		b.SetUnordered(true)
		b.SetTimeoutTimestamp(timeout)
	})
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

// keySigner signs with a harness key, as the reporter's signer would.
type keySigner struct{ key *secp256k1.PrivKey }

var _ signer.Signer = keySigner{}

func (s keySigner) Address() sdk.AccAddress { return sdk.AccAddress(s.key.PubKey().Address()) }

func (s keySigner) PubKey() cryptotypes.PubKey { return s.key.PubKey() }

func (s keySigner) Sign(_ context.Context, signBytes []byte) ([]byte, error) {
	return s.key.Sign(signBytes)
}

// TestUnorderedSimulate runs the reporter's simulation bytes through the simulate path of gRPC
// cosmos.tx.v1beta1.Service/Simulate (BaseApp.Simulate) against the latest committed state.
func TestUnorderedSimulate(t *testing.T) {
	setup := func(t *testing.T) (*testSuite, *reportertx.Builder, sdk.Msg) {
		t.Helper()
		h := setupReportingTestSuite(t)
		res := h.deliver(t, "", h.deposit(10))
		require.Zero(t, res.Code, res.Log)

		account := h.a.Keepers.Cosmos.Acct.GetAccount(h.ctx(), h.reporter0())
		b := reportertx.NewBuilder(h.a.TxConfig(), h.a.ChainID(), account.GetAccountNumber(), keySigner{key: h.reporters[0]})

		return h, b, h.reportMsg(t, 0, 1, sinktypes.OutcomeMinted)
	}

	t.Run("records the vote without committing it", func(t *testing.T) {
		h, b, msg := setup(t)
		before := h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), h.reporter0(), sdkutil.DenomUakt)

		bz, err := b.SimBytes([]sdk.Msg{msg}, h.now.Add(time.Minute))
		require.NoError(t, err)

		gas, result, err := h.a.Simulate(bz)
		require.NoError(t, err)
		require.Positive(t, gas.GasUsed)

		reported := 0
		for _, event := range result.Events {
			if event.Type != gogoproto.MessageName(&sinktypes.EventOutcomeReported{}) {
				continue
			}

			typed, err := sdk.ParseTypedEvent(event)
			require.NoError(t, err)
			ev, ok := typed.(*sinktypes.EventOutcomeReported)
			require.True(t, ok)
			require.Equal(t, h.reporterAddress(0), ev.Reporter)
			require.Equal(t, h.id(1), ev.ID)
			require.Equal(t, sinktypes.OutcomeMinted, ev.Outcome)
			reported++
		}
		require.Equal(t, 1, reported, "simulation must record reporter 0's vote")

		for _, vote := range h.transfer(t, 1).Settlement.Votes {
			require.NotEqual(t, h.reporterAddress(0), vote.Reporter)
		}
		require.Equal(t, before, h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), h.reporter0(), sdkutil.DenomUakt))
	})

	t.Run("a timeout beyond the max TTL fails without a message index", func(t *testing.T) {
		h, b, msg := setup(t)

		bz, err := b.SimBytes([]sdk.Msg{msg}, h.now.Add(11*time.Minute))
		require.NoError(t, err)

		_, _, err = h.a.Simulate(bz)
		require.ErrorIs(t, err, sdkerrors.ErrInvalidRequest)
		require.ErrorContains(t, err, "unordered tx ttl exceeds")
		require.NotContains(t, err.Error(), "message index")
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
