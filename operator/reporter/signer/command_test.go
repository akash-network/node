package signer_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
	"pkg.akt.dev/node/v2/operator/reporter/tx"
)

const commandTestChain = "r196-command-test"

// This identity is generated only for the subprocess fixture, never from operator custody.
func commandTestKey() *sdksecp.PrivKey {
	return sdksecp.GenPrivKeyFromSecret([]byte("R196 disposable command protocol fixture"))
}

// TestCommandProcess is re-executed as a real command. Protocol stdout has no testing output.
func TestCommandProcess(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "--r196-command" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}

	args := os.Args[marker+1:]
	if len(args) != 3 || args[2] != "literal;$(never-run) ' quoted" {
		os.Exit(91)
	}
	mode, capture := args[0], args[1]
	if mode == "sleep" {
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
	if mode == "nonzero" {
		fmt.Fprint(os.Stderr, "R196 inherited stderr sentinel\n")
		os.Exit(23)
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil || os.WriteFile(capture, input, 0600) != nil {
		os.Exit(92)
	}
	var request map[string]string
	if json.Unmarshal(input, &request) != nil || len(request) != 5 {
		os.Exit(93)
	}
	doc, err := base64.StdEncoding.DecodeString(request["signDocBase64"])
	if err != nil {
		os.Exit(94)
	}
	key := commandTestKey()
	sig, err := key.Sign(doc)
	if err != nil {
		os.Exit(95)
	}
	pub := key.PubKey().Bytes()
	switch mode {
	case "high-s":
		var scalar secp256k1.ModNScalar
		scalar.SetByteSlice(sig[32:])
		scalar.Negate()
		scalar.PutBytesUnchecked(sig[32:])
	case "65":
		sig = append(sig, 0)
	case "63":
		sig = sig[:63]
	case "zero":
		sig = make([]byte, 64)
	case "range":
		sig = bytes.Repeat([]byte{255}, 64)
	case "tamper":
		sig, _ = key.Sign(append(doc, 0))
	case "other-pub":
		pub = sdksecp.GenPrivKeyFromSecret([]byte("R196 other disposable identity")).PubKey().Bytes()
	case "bad-point":
		pub = append([]byte{2}, bytes.Repeat([]byte{255}, 32)...)
	case "stderr":
		fmt.Fprint(os.Stderr, "R196 inherited stderr sentinel\n")
	}
	response := map[string]any{
		"publicKeyBase64": base64.StdEncoding.EncodeToString(pub),
		"signatureBase64": base64.StdEncoding.EncodeToString(sig),
	}
	switch mode {
	case "extra":
		response["operatorMetadata"] = map[string]any{"accepted": true}
	case "key-base64":
		response["publicKeyBase64"] = response["publicKeyBase64"].(string) + "\n"
	case "sig-base64":
		response["signatureBase64"] = strings.TrimRight(response["signatureBase64"].(string), "=")
	case "missing":
		delete(response, "signatureBase64")
	case "null":
		response["signatureBase64"] = nil
	case "number":
		response["signatureBase64"] = 1
	case "case":
		response["SignatureBase64"] = response["signatureBase64"]
		delete(response, "signatureBase64")
	}
	output, _ := json.Marshal(response)
	switch mode {
	case "bad-json":
		output = []byte("not JSON")
	case "array":
		output = []byte("[]")
	case "trailing":
		output = append(output, []byte(" {}")...)
	case "exact-limit":
		output = append(output, bytes.Repeat([]byte{' '}, (64<<10)-len(output))...)
	case "oversize":
		output = append(output, bytes.Repeat([]byte{' '}, (64<<10)+1-len(output))...)
	}
	_, _ = os.Stdout.Write(output)
	os.Exit(0)
}

func commandFixture(t *testing.T, mode string) (signer.Signer, string) {
	t.Helper()
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	bin, err := os.Executable()
	require.NoError(t, err)
	capture := filepath.Join(t.TempDir(), "request.json")
	key := commandTestKey()
	s, err := signer.NewCommand(signer.CommandOptions{
		Command: bin,
		Args:    []string{"-test.run=^TestCommandProcess$", "--", "--r196-command", mode, capture, "literal;$(never-run) ' quoted"},
		Timeout: 3 * time.Second,
	}, sdk.AccAddress(key.PubKey().Address()), commandTestChain, key.PubKey())
	require.NoError(t, err)
	return s, capture
}

func directDoc(t *testing.T, sequence uint64) ([]byte, *txtypes.SignDoc, *txtypes.AuthInfo) {
	t.Helper()
	pub, err := codectypes.NewAnyWithValue(commandTestKey().PubKey())
	require.NoError(t, err)
	auth := &txtypes.AuthInfo{
		SignerInfos: []*txtypes.SignerInfo{
			{
				PublicKey: pub,
				ModeInfo: &txtypes.ModeInfo{
					Sum: &txtypes.ModeInfo_Single_{
						Single: &txtypes.ModeInfo_Single{
							Mode: signing.SignMode_SIGN_MODE_DIRECT,
						},
					},
				},
				Sequence: sequence,
			},
		},
	}
	authBytes, err := auth.Marshal()
	require.NoError(t, err)
	doc := &txtypes.SignDoc{
		BodyBytes:     []byte{10, 0},
		AuthInfoBytes: authBytes,
		ChainId:       commandTestChain,
		AccountNumber: math.MaxUint64,
	}
	data, err := doc.Marshal()
	require.NoError(t, err)
	return data, doc, auth
}

func TestCommandProtocol(t *testing.T) {
	for _, mode := range []string{"good", "high-s", "extra", "exact-limit", "stderr"} {
		t.Run(mode, func(t *testing.T) {
			s, capture := commandFixture(t, mode)
			data, _, _ := directDoc(t, math.MaxUint64-1)
			got, err := s.Sign(t.Context(), data)
			require.NoError(t, err)
			want, err := commandTestKey().Sign(data)
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.True(t, s.PubKey().VerifySignature(data, got))
			requestBytes, err := os.ReadFile(capture)
			require.NoError(t, err)
			var request map[string]string
			require.NoError(t, json.Unmarshal(requestBytes, &request))
			require.Equal(t, map[string]string{
				"chainId":       commandTestChain,
				"accountNumber": "18446744073709551615",
				"sequence":      "18446744073709551614",
				"address":       s.Address().String(),
				"signDocBase64": base64.StdEncoding.EncodeToString(data),
			}, request)
		})
	}
}

func TestCommandRejectsResponse(t *testing.T) {
	cases := map[string]string{
		"65": "64-byte", "63": "64-byte", "zero": "range", "range": "range", "tamper": "verify",
		"other-pub": "match reporter", "bad-point": "compressed", "key-base64": "canonical base64",
		"sig-base64": "canonical base64", "missing": "needs", "null": "needs", "number": "needs", "case": "needs",
		"bad-json": "invalid JSON", "array": "invalid JSON", "trailing": "invalid JSON", "oversize": "too large", "nonzero": "code 23",
	}
	for mode, message := range cases {
		t.Run(mode, func(t *testing.T) {
			s, _ := commandFixture(t, mode)
			data, _, _ := directDoc(t, 9)
			got, err := s.Sign(t.Context(), data)
			require.ErrorContains(t, err, message)
			require.Nil(t, got)
			require.NotContains(t, err.Error(), base64.StdEncoding.EncodeToString(data))
		})
	}
}

func TestCommandRejectsSignDocBeforeExecution(t *testing.T) {
	for _, name := range []string{"malformed-doc", "chain", "malformed-auth", "no-signer", "two-signers", "mode", "no-mode", "multi-mode", "no-key", "key-type", "key-bytes", "key-value"} {
		t.Run(name, func(t *testing.T) {
			s, capture := commandFixture(t, "good")
			_, doc, auth := directDoc(t, 0)
			switch name {
			case "chain":
				doc.ChainId = "another-chain"
			case "no-signer":
				auth.SignerInfos = nil
			case "two-signers":
				auth.SignerInfos = append(auth.SignerInfos, auth.SignerInfos[0])
			case "mode":
				auth.SignerInfos[0].ModeInfo.GetSingle().Mode = signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON
			case "no-mode":
				auth.SignerInfos[0].ModeInfo = nil
			case "multi-mode":
				auth.SignerInfos[0].ModeInfo = &txtypes.ModeInfo{
					Sum: &txtypes.ModeInfo_Multi_{
						Multi: &txtypes.ModeInfo_Multi{},
					},
				}
			case "no-key":
				auth.SignerInfos[0].PublicKey = nil
			case "key-type":
				auth.SignerInfos[0].PublicKey.TypeUrl = "/cosmos.crypto.ed25519.PubKey"
			case "key-bytes":
				pub, err := codectypes.NewAnyWithValue(sdksecp.GenPrivKeyFromSecret([]byte("other")).PubKey())
				require.NoError(t, err)
				auth.SignerInfos[0].PublicKey = pub
			case "key-value":
				auth.SignerInfos[0].PublicKey.Value = []byte{255}
			}
			var err error
			doc.AuthInfoBytes, err = auth.Marshal()
			require.NoError(t, err)
			if name == "malformed-auth" {
				doc.AuthInfoBytes = []byte{255}
			}
			data, err := doc.Marshal()
			require.NoError(t, err)
			if name == "malformed-doc" {
				data = []byte{255}
			}
			got, err := s.Sign(t.Context(), data)
			require.Error(t, err)
			require.Nil(t, got)
			_, err = os.Stat(capture)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestCommandTimeoutCancelAndMissing(t *testing.T) {
	for _, name := range []string{"timeout", "cancel", "already-canceled", "missing"} {
		t.Run(name, func(t *testing.T) {
			bin, err := os.Executable()
			require.NoError(t, err)
			capture := filepath.Join(t.TempDir(), "never-written")
			opts := signer.CommandOptions{
				Command: bin,
				Args:    []string{"-test.run=^TestCommandProcess$", "--", "--r196-command", "sleep", capture, "literal;$(never-run) ' quoted"},
				Timeout: 200 * time.Millisecond,
			}
			if name == "missing" {
				opts.Command = filepath.Join(t.TempDir(), "missing-executable")
			}
			key := commandTestKey()
			s, err := signer.NewCommand(opts, sdk.AccAddress(key.PubKey().Address()), commandTestChain, key.PubKey())
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if name == "already-canceled" {
				cancel()
			} else if name == "cancel" {
				timer := time.AfterFunc(50*time.Millisecond, cancel)
				defer timer.Stop()
			}
			data, _, _ := directDoc(t, 0)
			start := time.Now()
			got, err := s.Sign(ctx, data)
			require.Error(t, err)
			require.Nil(t, got)
			require.Less(t, time.Since(start), time.Second)
			if name == "timeout" {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			} else if strings.Contains(name, "cancel") {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestCommandConstructorAndPublicKey(t *testing.T) {
	key := commandTestKey()
	addr := sdk.AccAddress(key.PubKey().Address())
	encoded := base64.StdEncoding.EncodeToString(key.PubKey().Bytes())
	pub, err := signer.ParseCommandPublicKey(encoded, addr)
	require.NoError(t, err)
	require.Equal(t, key.PubKey().Bytes(), pub.Bytes())
	for _, bad := range []string{encoded + "\n", "!", "", base64.StdEncoding.EncodeToString(make([]byte, 33)), base64.StdEncoding.EncodeToString(append([]byte{2}, bytes.Repeat([]byte{255}, 32)...))} {
		_, err := signer.ParseCommandPublicKey(bad, addr)
		require.Error(t, err)
	}
	_, err = signer.ParseCommandPublicKey(encoded, sdk.AccAddress(make([]byte, 20)))
	require.Error(t, err)
	for _, name := range []string{"relative", "args", "zero-timeout", "long-timeout", "chain", "nil", "typed-nil", "ed25519", "wrong-reporter"} {
		t.Run(name, func(t *testing.T) {
			opts := signer.CommandOptions{
				Command: "/nonexistent/allowed-without-spawn",
				Timeout: time.Second,
			}
			chain := commandTestChain
			var pub cryptotypes.PubKey = key.PubKey()
			reporter := addr
			switch name {
			case "relative":
				opts.Command = "relative"
			case "args":
				opts.Args = make([]string, 33)
			case "zero-timeout":
				opts.Timeout = 0
			case "long-timeout":
				opts.Timeout = 121 * time.Second
			case "chain":
				chain = ""
			case "nil":
				pub = nil
			case "typed-nil":
				pub = (*sdksecp.PubKey)(nil)
			case "ed25519":
				pub = ed25519.GenPrivKey().PubKey()
			case "wrong-reporter":
				reporter = sdk.AccAddress(make([]byte, 20))
			}
			_, err := signer.NewCommand(opts, reporter, chain, pub)
			require.Error(t, err)
		})
	}
}

func TestCommandRealBuilder(t *testing.T) {
	s, capture := commandFixture(t, "high-s")
	enc := sdkutil.MakeEncodingConfig()
	sinkv1.RegisterInterfaces(enc.InterfaceRegistry)
	builder := tx.NewBuilder(enc.TxConfig, commandTestChain, math.MaxUint64, s)
	timeout := time.Date(2026, 11, 15, 1, 2, 3, 456000000, time.UTC)
	msgs := []sdk.Msg{
		&sinkv1.MsgReportOutcome{
			Reporter: s.Address().String(),
			ID: sinkv1.ID{
				Owner:    s.Address().String(),
				Height:   42,
				Sequence: 7,
			},
			PolicyVersion: 2,
			Round:         1,
			Observation: sinkv1.Observation{
				Outcome: sinkv1.Outcome(1),
				Slot:    100,
			},
		},
	}
	fees := sdk.NewCoins(sdk.NewInt64Coin("uakt", 5000))
	data, hash, err := builder.Build(t.Context(), msgs, 200000, fees, timeout)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	require.Equal(t, sum[:], hash)
	var raw txtypes.TxRaw
	require.NoError(t, raw.Unmarshal(data))
	var body txtypes.TxBody
	require.NoError(t, body.Unmarshal(raw.BodyBytes))
	require.True(t, body.Unordered)
	require.Equal(t, uint64(0), body.TimeoutHeight)
	require.True(t, timeout.Equal(*body.TimeoutTimestamp))
	require.Len(t, body.Messages, 1)
	var auth txtypes.AuthInfo
	require.NoError(t, auth.Unmarshal(raw.AuthInfoBytes))
	require.Equal(t, uint64(0), auth.SignerInfos[0].Sequence)
	require.Equal(t, fees, auth.Fee.Amount)
	require.Equal(t, uint64(200000), auth.Fee.GasLimit)
	exact, err := (&txtypes.SignDoc{
		BodyBytes:     raw.BodyBytes,
		AuthInfoBytes: raw.AuthInfoBytes,
		ChainId:       commandTestChain,
		AccountNumber: math.MaxUint64,
	}).Marshal()
	require.NoError(t, err)
	require.Len(t, raw.Signatures, 1)
	require.True(t, s.PubKey().VerifySignature(exact, raw.Signatures[0]))
	input, err := os.ReadFile(capture)
	require.NoError(t, err)
	var request map[string]string
	require.NoError(t, json.Unmarshal(input, &request))
	require.Equal(t, "0", request["sequence"])
	require.Equal(t, base64.StdEncoding.EncodeToString(exact), request["signDocBase64"])
	for _, mode := range []string{"tamper", "nonzero", "65", "oversize"} {
		bad, _ := commandFixture(t, mode)
		builder := tx.NewBuilder(enc.TxConfig, commandTestChain, math.MaxUint64, bad)
		bytes, hash, err := builder.Build(t.Context(), msgs, 200000, fees, timeout)
		require.Error(t, err)
		require.Nil(t, bytes)
		require.Nil(t, hash)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = builder.Build(ctx, msgs, 200000, fees, timeout)
	require.True(t, errors.Is(err, context.Canceled))
}
