package tx

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/signer"
)

const (
	testChainID       = "akashnet-test"
	testAccountNumber = 17
)

func testTxConfig() client.TxConfig {
	cfg := sdkutil.MakeEncodingConfig()
	sinkv1.RegisterInterfaces(cfg.InterfaceRegistry)

	return cfg.TxConfig
}

func testSigner(t *testing.T) signer.Signer {
	t.Helper()

	reg := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(reg)

	kr, err := keyring.New("akt-reporter-test", keyring.BackendTest, t.TempDir(), nil, codec.NewProtoCodec(reg))
	require.NoError(t, err)

	rec, _, err := kr.NewMnemonic("r", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	require.NoError(t, err)

	addr, err := rec.GetAddress()
	require.NoError(t, err)

	s, err := signer.NewKeyring(kr, "r", addr)
	require.NoError(t, err)

	return s
}

// recordingSigner keeps the sign bytes it receives and may declare a sign-bytes limit.
type recordingSigner struct {
	signer.Signer
	limit int
	got   [][]byte
}

func (r *recordingSigner) SignBytesLimit() int { return r.limit }

func (r *recordingSigner) Sign(ctx context.Context, signBytes []byte) ([]byte, error) {
	r.got = append(r.got, append([]byte(nil), signBytes...))
	return r.Signer.Sign(ctx, signBytes)
}

func voteMsg(s signer.Signer) sdk.Msg {
	return &sinkv1.MsgReportOutcome{
		Reporter:      s.Address().String(),
		ID:            sinkv1.ID{Owner: s.Address().String(), Height: 42, Sequence: 7},
		PolicyVersion: 2,
		Round:         1,
		Observation:   sinkv1.Observation{Outcome: sinkv1.Outcome(1), Slot: 100},
	}
}

func decodeRaw(t *testing.T, bz []byte) (*txtypes.TxRaw, *txtypes.TxBody, *txtypes.AuthInfo) {
	t.Helper()

	var raw txtypes.TxRaw
	require.NoError(t, raw.Unmarshal(bz))

	var body txtypes.TxBody
	require.NoError(t, body.Unmarshal(raw.BodyBytes))

	var auth txtypes.AuthInfo
	require.NoError(t, auth.Unmarshal(raw.AuthInfoBytes))

	return &raw, &body, &auth
}

func requireUnorderedBody(t *testing.T, body *txtypes.TxBody, auth *txtypes.AuthInfo, timeout time.Time) {
	t.Helper()

	require.True(t, body.Unordered)
	require.NotNil(t, body.TimeoutTimestamp)
	require.True(t, timeout.Equal(*body.TimeoutTimestamp))
	require.Zero(t, body.TimeoutHeight)
	require.Equal(t, "AKT sink reporter", body.Memo)
	require.Len(t, body.Messages, 1)

	require.Len(t, auth.SignerInfos, 1)
	info := auth.SignerInfos[0]
	require.Zero(t, info.Sequence)
	require.NotNil(t, info.ModeInfo.GetSingle())
	require.Equal(t, signing.SignMode_SIGN_MODE_DIRECT, info.ModeInfo.GetSingle().Mode)
}

func TestBuildUnordered(t *testing.T) {
	cfg := testTxConfig()
	s := testSigner(t)
	timeout := SlotTimeout(15_000_000, 5)
	fee := sdk.NewCoins(sdk.NewInt64Coin("uakt", 2500))
	msgs := []sdk.Msg{voteMsg(s)}

	b := NewBuilder(cfg, testChainID, testAccountNumber, s)

	txBytes, hash, err := b.Build(context.Background(), msgs, 100_000, fee, timeout)
	require.NoError(t, err)

	sum := sha256.Sum256(txBytes)
	require.Equal(t, sum[:], hash)

	raw, body, auth := decodeRaw(t, txBytes)
	requireUnorderedBody(t, body, auth, timeout)
	require.Equal(t, uint64(100_000), auth.Fee.GasLimit)
	require.Equal(t, fee, auth.Fee.Amount)
	pub, err := codectypes.NewAnyWithValue(s.PubKey())
	require.NoError(t, err)
	require.Equal(t, pub.TypeUrl, auth.SignerInfos[0].PublicKey.TypeUrl)
	require.Equal(t, pub.Value, auth.SignerInfos[0].PublicKey.Value)

	signBytes, err := (&txtypes.SignDoc{
		BodyBytes:     raw.BodyBytes,
		AuthInfoBytes: raw.AuthInfoBytes,
		ChainId:       testChainID,
		AccountNumber: testAccountNumber,
	}).Marshal()
	require.NoError(t, err)
	require.Len(t, raw.Signatures, 1)
	require.True(t, s.PubKey().VerifySignature(signBytes, raw.Signatures[0]))

	decoded, err := cfg.TxDecoder()(txBytes)
	require.NoError(t, err)
	require.Equal(t, msgs[0].String(), decoded.GetMsgs()[0].String())
}

func TestSimBytes(t *testing.T) {
	s := testSigner(t)
	timeout := SlotTimeout(15_000_000, 200)

	bz, err := NewBuilder(testTxConfig(), testChainID, testAccountNumber, s).SimBytes([]sdk.Msg{voteMsg(s)}, timeout)
	require.NoError(t, err)

	raw, body, auth := decodeRaw(t, bz)
	requireUnorderedBody(t, body, auth, timeout)
	require.Len(t, raw.Signatures, 1)
	require.Empty(t, raw.Signatures[0])
}

func TestSignBytesLimit(t *testing.T) {
	cfg := testTxConfig()
	base := testSigner(t)
	timeout := SlotTimeout(15_000_000, 0)
	fee := sdk.NewCoins(sdk.NewInt64Coin("uakt", 2500))
	msgs := []sdk.Msg{voteMsg(base)}

	t.Run("length matches the signed bytes", func(t *testing.T) {
		rec := &recordingSigner{Signer: base}
		b := NewBuilder(cfg, testChainID, testAccountNumber, rec)

		n, err := b.SignBytesLen(msgs, 100_000, fee, timeout)
		require.NoError(t, err)

		_, _, err = b.Build(context.Background(), msgs, 100_000, fee, timeout)
		require.NoError(t, err)
		require.Len(t, rec.got, 1)
		require.Equal(t, len(rec.got[0]), n)
	})

	t.Run("over the limit is refused before signing", func(t *testing.T) {
		rec := &recordingSigner{Signer: base, limit: 64}
		b := NewBuilder(cfg, testChainID, testAccountNumber, rec)

		n, err := b.SignBytesLen(msgs, 100_000, fee, timeout)
		require.NoError(t, err)
		require.Greater(t, n, rec.limit)

		_, _, err = b.Build(context.Background(), msgs, 100_000, fee, timeout)
		require.True(t, errors.Is(err, ErrSignBytesLimit), err)
		require.Empty(t, rec.got)
	})

	t.Run("at the limit is signed", func(t *testing.T) {
		rec := &recordingSigner{Signer: base}
		b := NewBuilder(cfg, testChainID, testAccountNumber, rec)

		n, err := b.SignBytesLen(msgs, 100_000, fee, timeout)
		require.NoError(t, err)

		rec.limit = n
		_, _, err = b.Build(context.Background(), msgs, 100_000, fee, timeout)
		require.NoError(t, err)
		require.Len(t, rec.got, 1)
	})
}

func planned(n int) []policy.Planned {
	out := make([]policy.Planned, n)
	for i := range out {
		out[i] = policy.Planned{Msg: &sinkv1.MsgVetoVoid{ID: sinkv1.ID{Height: int64(i + 1), Sequence: 1}}}
	}

	return out
}

// scriptedSim answers simulations in order and records the batch size of each call.
type scriptedSim struct {
	answers []error
	gas     uint64
	sizes   []int
}

func (s *scriptedSim) sim(msgs []sdk.Msg) (uint64, error) {
	s.sizes = append(s.sizes, len(msgs))
	if len(s.answers) == 0 {
		return s.gas, nil
	}

	err := s.answers[0]
	s.answers = s.answers[1:]

	if err != nil {
		return 0, err
	}

	return s.gas, nil
}

func TestPrune(t *testing.T) {
	t.Run("drops the failing message", func(t *testing.T) {
		batch := planned(3)
		failure := errors.New("failed to execute message; message index: 1: transfer not found")
		sim := &scriptedSim{answers: []error{failure}, gas: 777}

		res := Prune(sim.sim, batch)
		require.Equal(t, []policy.Planned{batch[0], batch[2]}, res.Kept)
		require.Equal(t, uint64(777), res.GasUsed)
		require.Equal(t, []Dropped{{Planned: batch[1], Err: failure}}, res.Dropped)
		require.Empty(t, res.Deferred)
		require.Equal(t, []int{3, 2}, sim.sizes)
		require.Equal(t, planned(3), batch)
	})

	t.Run("three failing rounds defer the rest", func(t *testing.T) {
		batch := planned(5)
		errs := []error{
			errors.New("message index: 0: a"),
			errors.New("message index: 2: b"),
			errors.New("message index: 0: c"),
		}
		sim := &scriptedSim{answers: errs}

		res := Prune(sim.sim, batch)
		require.Empty(t, res.Kept)
		require.Zero(t, res.GasUsed)
		require.Equal(t, []Dropped{
			{Planned: batch[0], Err: errs[0]},
			{Planned: batch[3], Err: errs[1]},
			{Planned: batch[1], Err: errs[2]},
		}, res.Dropped)
		require.Equal(t, []policy.Planned{batch[2], batch[4]}, res.Deferred)
		require.Equal(t, []int{5, 4, 3}, sim.sizes)
	})

	t.Run("an error without an index drops the batch", func(t *testing.T) {
		batch := planned(3)
		failure := errors.New("insufficient funds")
		sim := &scriptedSim{answers: []error{failure}}

		res := Prune(sim.sim, batch)
		require.Empty(t, res.Kept)
		require.Empty(t, res.Deferred)
		require.Equal(t, []Dropped{
			{Planned: batch[0], Err: failure},
			{Planned: batch[1], Err: failure},
			{Planned: batch[2], Err: failure},
		}, res.Dropped)
	})

	t.Run("an index outside the batch drops the batch", func(t *testing.T) {
		batch := planned(2)
		failure := errors.New("message index: 2: out of range")

		res := Prune((&scriptedSim{answers: []error{failure}}).sim, batch)
		require.Empty(t, res.Kept)
		require.Len(t, res.Dropped, 2)
	})

	t.Run("dropping every message stops simulating", func(t *testing.T) {
		batch := planned(1)
		failure := errors.New("message index: 0: bad")
		sim := &scriptedSim{answers: []error{failure}}

		res := Prune(sim.sim, batch)
		require.Empty(t, res.Kept)
		require.Empty(t, res.Deferred)
		require.Equal(t, []Dropped{{Planned: batch[0], Err: failure}}, res.Dropped)
		require.Equal(t, []int{1}, sim.sizes)
	})
}

func TestFeeFor(t *testing.T) {
	prices := sdk.NewDecCoins(sdk.NewDecCoinFromDec("uakt", sdkmath.LegacyMustNewDecFromStr("0.025")))

	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 2500)), FeeFor(100_000, prices, nil))
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 2501)), FeeFor(100_001, prices, nil))

	explicit := sdk.NewCoins(sdk.NewInt64Coin("uakt", 9))
	require.Equal(t, explicit, FeeFor(100_000, prices, explicit))
}
