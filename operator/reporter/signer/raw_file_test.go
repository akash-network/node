package signer_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
	reportertx "pkg.akt.dev/node/v2/operator/reporter/tx"
)

func TestRawFileIdentityDirectSignatureAndCustody(t *testing.T) {
	private := sdksecp.GenPrivKeyFromSecret([]byte("R176 deterministic disposable file fixture"))
	address := sdk.AccAddress(private.PubKey().Address())
	path := filepath.Join(t.TempDir(), "raw.key")
	original := []byte(hex.EncodeToString(private.Key) + "\n")
	require.NoError(t, os.WriteFile(path, original, 0600))
	s, err := signer.NewRawFile(path, address)
	require.NoError(t, err)
	require.True(t, bytes.Equal(address, s.Address()))
	require.True(t, bytes.Equal(private.PubKey().Bytes(), s.PubKey().Bytes()))
	doc, err := proto.Marshal(&txtypes.SignDoc{
		BodyBytes:     []byte{10, 0},
		AuthInfoBytes: []byte{18, 0},
		ChainId:       "r176-test",
		AccountNumber: 93,
	})
	require.NoError(t, err)
	got, err := s.Sign(context.Background(), doc)
	require.NoError(t, err)
	expected, err := private.Sign(doc)
	require.NoError(t, err)
	require.True(t, bytes.Equal(expected, got), "signature differs from exact direct sign bytes")
	require.True(t, private.PubKey().VerifySignature(doc, got))
	require.False(t, private.PubKey().VerifySignature(append(doc, 0), got))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Sign(ctx, doc)
	require.ErrorIs(t, err, context.Canceled)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(original, after), "original key file changed")
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "persistent import created a file")
	require.Equal(t, "raw.key", entries[0].Name())
	_, err = signer.NewRawFile(path, sdk.AccAddress(bytes.Repeat([]byte{1}, 20)))
	require.ErrorContains(t, err, "does not match")
	require.NotContains(t, err.Error(), hex.EncodeToString(private.Key))
}

func TestRawFileRejectsUnsafeFilesAndScalars(t *testing.T) {
	address := sdk.AccAddress(bytes.Repeat([]byte{1}, 20))
	for _, tc := range []struct {
		name string
		data string
	}{
		{
			name: "zero",
			data: strings.Repeat("0", 64),
		},
		{
			name: "overflow",
			data: strings.Repeat("f", 64),
		},
		{
			name: "uppercase",
			data: strings.Repeat("A", 64),
		},
		{
			name: "short",
			data: strings.Repeat("1", 62),
		},
		{
			name: "long",
			data: strings.Repeat("1", 66),
		},
		{
			name: "nonhex",
			data: strings.Repeat("g", 64),
		},
		{
			name: "oversized",
			data: strings.Repeat("1", 4097),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "raw.key")
			require.NoError(t, os.WriteFile(path, []byte(tc.data), 0600))
			_, err := signer.NewRawFile(path, address)
			require.Error(t, err)
			require.NotContains(t, err.Error(), tc.data)
		})
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "raw.key")
	private := sdksecp.GenPrivKeyFromSecret([]byte("R176 deterministic disposable mode fixture"))
	require.NoError(t, os.WriteFile(path, []byte(hex.EncodeToString(private.Key)), 0600))
	for _, mode := range []os.FileMode{0640, 0604, 0666} {
		require.NoError(t, os.Chmod(path, mode))
		_, err := signer.NewRawFile(path, address)
		require.ErrorContains(t, err, "owner-only")
	}

	require.NoError(t, os.Chmod(path, 0600))
	link := filepath.Join(dir, "symlink")
	require.NoError(t, os.Symlink(path, link))
	_, err := signer.NewRawFile(link, address)
	require.ErrorContains(t, err, "symlinks")
	_, err = signer.NewRawFile(dir, address)
	require.ErrorContains(t, err, "regular file")
	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0600))
	_, err = signer.NewRawFile(fifo, address)
	require.ErrorContains(t, err, "regular file")
}

func TestRawFileSignsReporterBuilderExactly(t *testing.T) {
	private := sdksecp.GenPrivKeyFromSecret([]byte("R176 deterministic disposable builder fixture"))
	address := sdk.AccAddress(private.PubKey().Address())
	path := filepath.Join(t.TempDir(), "raw.key")
	require.NoError(t, os.WriteFile(path, []byte(hex.EncodeToString(private.Key)), 0600))
	s, err := signer.NewRawFile(path, address)
	require.NoError(t, err)
	enc := sdkutil.MakeEncodingConfig()
	sinkv1.RegisterInterfaces(enc.InterfaceRegistry)
	builder := reportertx.NewBuilder(enc.TxConfig, "r176-test", 73, s)
	message := &sinkv1.MsgReportOutcome{
		Reporter: address.String(),
		ID: sinkv1.ID{
			Owner:    address.String(),
			Height:   42,
			Sequence: 7,
		},
		PolicyVersion: 2,
		Round:         1,
		Observation: sinkv1.Observation{
			Outcome: sinkv1.Outcome(1),
			Slot:    100,
		},
	}
	fee := sdk.NewCoins(sdk.NewInt64Coin("uakt", 5000))
	timeout := time.Unix(200, 0)
	encoded, _, err := builder.Build(t.Context(), []sdk.Msg{message}, 200000, fee, timeout)
	require.NoError(t, err)
	var raw txtypes.TxRaw
	require.NoError(t, proto.Unmarshal(encoded, &raw))
	require.Len(t, raw.Signatures, 1)
	var body txtypes.TxBody
	require.NoError(t, proto.Unmarshal(raw.BodyBytes, &body))
	require.Len(t, body.Messages, 1)
	require.True(t, body.Unordered)
	require.Zero(t, body.TimeoutHeight)
	require.NotNil(t, body.TimeoutTimestamp)
	require.True(t, timeout.Equal(*body.TimeoutTimestamp))
	var auth txtypes.AuthInfo
	require.NoError(t, proto.Unmarshal(raw.AuthInfoBytes, &auth))
	require.Len(t, auth.SignerInfos, 1)
	require.Zero(t, auth.SignerInfos[0].Sequence)
	require.Equal(t, fee, auth.Fee.Amount)
	require.Equal(t, uint64(200000), auth.Fee.GasLimit)
	signDoc, err := proto.Marshal(&txtypes.SignDoc{
		BodyBytes:     raw.BodyBytes,
		AuthInfoBytes: raw.AuthInfoBytes,
		ChainId:       "r176-test",
		AccountNumber: 73,
	})
	require.NoError(t, err)
	expected, err := private.Sign(signDoc)
	require.NoError(t, err)
	require.True(t, bytes.Equal(expected, raw.Signatures[0]), "reporter direct signature differs")
	simulation, err := builder.SimBytes([]sdk.Msg{message}, 200000, fee, timeout)
	require.NoError(t, err)
	var simRaw txtypes.TxRaw
	require.NoError(t, proto.Unmarshal(simulation, &simRaw))
	require.True(t, bytes.Equal(raw.BodyBytes, simRaw.BodyBytes))
	require.True(t, bytes.Equal(raw.AuthInfoBytes, simRaw.AuthInfoBytes))
	require.Len(t, simRaw.Signatures, 1)
	require.True(t, bytes.Equal(make([]byte, 64), simRaw.Signatures[0]))
}
