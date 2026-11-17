package signer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
)

const commandOutputLimit = 64 << 10

// CommandOptions selects the packaged reporter's externalSigner protocol.
// Command is an absolute executable path; Args are passed without a shell.
type CommandOptions struct {
	Command string
	Args    []string
	Timeout time.Duration
}

type commandSigner struct {
	opts    CommandOptions
	addr    sdk.AccAddress
	chainID string
	pub     *sdksecp.PubKey
}

var _ Signer = (*commandSigner)(nil)

// ParseCommandPublicKey validates canonical base64 and the reporter's compressed secp256k1 key.
// It performs no file access, process execution or account query.
func ParseCommandPublicKey(encoded string, reporter sdk.AccAddress) (cryptotypes.PubKey, error) {
	raw, err := commandBase64(encoded)
	if err != nil {
		return nil, errors.New("external signer public key is not canonical base64")
	}

	return commandPublicKey(raw, reporter)
}

func commandPublicKey(raw []byte, reporter sdk.AccAddress) (*sdksecp.PubKey, error) {
	pub, err := parseRemoteKey(raw)
	if err != nil {
		return nil, errors.New("external signer public key must be a valid 33-byte compressed secp256k1 point")
	}

	if !sdk.AccAddress(pub.Address()).Equals(reporter) {
		return nil, errMismatch
	}

	return pub, nil
}

// NewCommand prepares command custody without running the executable. pub is either configured
// or read from the reporter account by broadcast startup. Every response is verified before use.
func NewCommand(opts CommandOptions, reporter sdk.AccAddress, chainID string, pub cryptotypes.PubKey) (Signer, error) {
	if !filepath.IsAbs(opts.Command) {
		return nil, errors.New("external signer command must be an absolute path")
	}

	if len(opts.Args) > 32 {
		return nil, errors.New("external signer accepts at most 32 arguments")
	}

	if opts.Timeout <= 0 || opts.Timeout > 120*time.Second {
		return nil, errors.New("external signer timeout must be positive and at most 120 seconds")
	}

	if chainID == "" {
		return nil, errors.New("external signer chain ID is empty")
	}

	key, ok := pub.(*sdksecp.PubKey)
	if !ok || key == nil {
		return nil, errors.New("external signer requires a secp256k1 public key")
	}

	key, err := commandPublicKey(key.Key, reporter)
	if err != nil {
		return nil, err
	}

	opts.Args = append([]string(nil), opts.Args...)
	return &commandSigner{
		opts:    opts,
		addr:    append(sdk.AccAddress(nil), reporter...),
		chainID: chainID,
		pub:     key,
	}, nil
}

func (s *commandSigner) Address() sdk.AccAddress { return s.addr }

func (s *commandSigner) PubKey() cryptotypes.PubKey { return s.pub }

func (s *commandSigner) Sign(ctx context.Context, signBytes []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var doc txtypes.SignDoc
	if err := doc.Unmarshal(signBytes); err != nil {
		return nil, errors.New("external signer received an invalid SignDoc")
	}

	if doc.ChainId != s.chainID {
		return nil, errors.New("external signer sign request for another chain")
	}

	var auth txtypes.AuthInfo
	if err := auth.Unmarshal(doc.AuthInfoBytes); err != nil || len(auth.SignerInfos) != 1 {
		return nil, errors.New("external signer SignDoc must name exactly one direct signer")
	}

	info := auth.SignerInfos[0]
	if info == nil || info.ModeInfo.GetSingle().GetMode() != signing.SignMode_SIGN_MODE_DIRECT ||
		info.PublicKey == nil || info.PublicKey.TypeUrl != "/cosmos.crypto.secp256k1.PubKey" {
		return nil, errors.New("external signer SignDoc does not name this direct signer")
	}

	var pub sdksecp.PubKey
	if err := pub.Unmarshal(info.PublicKey.Value); err != nil || !bytes.Equal(pub.Key, s.pub.Key) {
		return nil, errors.New("external signer SignDoc does not name this public key")
	}

	input, err := json.Marshal(struct {
		ChainID       string `json:"chainId"`
		AccountNumber string `json:"accountNumber"`
		Sequence      string `json:"sequence"`
		Address       string `json:"address"`
		SignDocBase64 string `json:"signDocBase64"`
	}{
		ChainID:       doc.ChainId,
		AccountNumber: strconv.FormatUint(doc.AccountNumber, 10),
		Sequence:      strconv.FormatUint(info.Sequence, 10),
		Address:       s.addr.String(),
		SignDocBase64: base64.StdEncoding.EncodeToString(signBytes),
	})
	if err != nil {
		return nil, err
	}

	output, err := s.run(ctx, input)
	if err != nil {
		return nil, err
	}

	return s.signature(output, signBytes)
}

// commandOutput cancels a running command as soon as stdout exceeds the protocol limit.
// It retains at most the limit, even if one write is arbitrarily large.
type commandOutput struct {
	buffer   bytes.Buffer
	cancel   context.CancelFunc
	tooLarge bool
}

func (o *commandOutput) Write(p []byte) (int, error) {
	if len(p) > commandOutputLimit-o.buffer.Len() {
		o.tooLarge = true
		o.cancel()
		return 0, errors.New("external signer output is too large")
	}

	return o.buffer.Write(p)
}

func (s *commandSigner) run(ctx context.Context, input []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()

	output := &commandOutput{cancel: cancel}
	cmd := exec.CommandContext(ctx, s.opts.Command, s.opts.Args...)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = output
	cmd.Stderr = os.Stderr
	// A descendant retaining stdout must not hold the caller past cancellation or exit.
	cmd.WaitDelay = 100 * time.Millisecond
	err := cmd.Run()
	if output.tooLarge {
		return nil, errors.New("external signer output is too large")
	}

	if ctx.Err() != nil {
		return nil, fmt.Errorf("external signer: %w", ctx.Err())
	}

	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("external signer exited with code %d", exit.ExitCode())
		}

		return nil, errors.New("external signer did not complete")
	}

	return output.buffer.Bytes(), nil
}

func (s *commandSigner) signature(output, signBytes []byte) ([]byte, error) {
	// The TS protocol accepts extra response keys; only these two are consumed.
	var response map[string]json.RawMessage
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, errors.New("external signer returned invalid JSON")
	}

	var key, signature *string
	if json.Unmarshal(response["publicKeyBase64"], &key) != nil || key == nil ||
		json.Unmarshal(response["signatureBase64"], &signature) != nil || signature == nil {
		return nil, errors.New("external signer response needs publicKeyBase64 and signatureBase64 strings")
	}

	pub, err := ParseCommandPublicKey(*key, s.addr)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(pub.Bytes(), s.pub.Key) {
		return nil, errors.New("external signer used another key")
	}

	raw, err := commandBase64(*signature)
	if err != nil {
		return nil, errors.New("external signer signature is not canonical base64")
	}

	if len(raw) != sigLen {
		return nil, errors.New("external signer signature must be 64-byte r||s")
	}

	return NormalizeAndVerify(s.pub, signBytes, raw)
}

func commandBase64(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(raw) != encoded {
		return nil, errors.New("invalid canonical base64")
	}

	return raw, nil
}
