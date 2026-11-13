package tx

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
)

// memo is the memo of every reporter transaction.
const memo = "AKT sink reporter"

// ErrSignBytesLimit reports sign bytes longer than the signer's declared limit.
var ErrSignBytesLimit = errors.New("sign bytes exceed the signer's limit")

// Builder builds the reporter's unordered transactions: sequence 0, SIGN_MODE_DIRECT, no timeout height.
type Builder struct {
	cfg           client.TxConfig
	chainID       string
	accountNumber uint64
	signer        signer.Signer
}

// NewBuilder returns a builder for the account accountNumber that s signs for.
func NewBuilder(cfg client.TxConfig, chainID string, accountNumber uint64, s signer.Signer) *Builder {
	return &Builder{cfg: cfg, chainID: chainID, accountNumber: accountNumber, signer: s}
}

// SimBytes returns the transaction to simulate msgs with: unsigned, with no fee and no gas limit.
func (b *Builder) SimBytes(msgs []sdk.Msg, timeout time.Time) ([]byte, error) {
	txb, _, err := b.unsigned(msgs, 0, nil, timeout)
	if err != nil {
		return nil, err
	}

	return b.cfg.TxEncoder()(txb.GetTx())
}

// SignBytesLen returns the length of the direct sign doc that Build would sign for the same arguments.
func (b *Builder) SignBytesLen(msgs []sdk.Msg, gasLimit uint64, fee sdk.Coins, timeout time.Time) (int, error) {
	txb, sig, err := b.unsigned(msgs, gasLimit, fee, timeout)
	if err != nil {
		return 0, err
	}

	signBytes, err := b.signBytes(context.Background(), txb, sig)
	if err != nil {
		return 0, err
	}

	return len(signBytes), nil
}

// Build signs the transaction and returns its bytes and its hash, SHA-256 of the bytes. It refuses sign bytes
// longer than the signer's limit with ErrSignBytesLimit before asking the signer.
func (b *Builder) Build(ctx context.Context, msgs []sdk.Msg, gasLimit uint64, fee sdk.Coins, timeout time.Time) (txBytes, hash []byte, err error) {
	txb, sig, err := b.unsigned(msgs, gasLimit, fee, timeout)
	if err != nil {
		return nil, nil, err
	}

	signBytes, err := b.signBytes(ctx, txb, sig)
	if err != nil {
		return nil, nil, err
	}

	if limit := signer.SignBytesLimit(b.signer); limit > 0 && len(signBytes) > limit {
		return nil, nil, fmt.Errorf("%w: %d bytes, limit %d", ErrSignBytesLimit, len(signBytes), limit)
	}

	signature, err := b.signer.Sign(ctx, signBytes)
	if err != nil {
		return nil, nil, err
	}

	sig.Data = &signing.SingleSignatureData{SignMode: signing.SignMode_SIGN_MODE_DIRECT, Signature: signature}
	if err := txb.SetSignatures(sig); err != nil {
		return nil, nil, fmt.Errorf("set signature: %w", err)
	}

	txBytes, err = b.cfg.TxEncoder()(txb.GetTx())
	if err != nil {
		return nil, nil, fmt.Errorf("encode transaction: %w", err)
	}

	sum := sha256.Sum256(txBytes)

	return txBytes, sum[:], nil
}

func (b *Builder) unsigned(msgs []sdk.Msg, gasLimit uint64, fee sdk.Coins, timeout time.Time) (client.TxBuilder, signing.SignatureV2, error) {
	txb := b.cfg.NewTxBuilder()
	if err := txb.SetMsgs(msgs...); err != nil {
		return nil, signing.SignatureV2{}, fmt.Errorf("set messages: %w", err)
	}

	txb.SetMemo(memo)
	txb.SetGasLimit(gasLimit)
	txb.SetFeeAmount(fee)
	txb.SetUnordered(true)
	txb.SetTimeoutTimestamp(timeout)

	sig := signing.SignatureV2{
		PubKey:   b.signer.PubKey(),
		Data:     &signing.SingleSignatureData{SignMode: signing.SignMode_SIGN_MODE_DIRECT},
		Sequence: 0,
	}

	if err := txb.SetSignatures(sig); err != nil {
		return nil, signing.SignatureV2{}, fmt.Errorf("set signer info: %w", err)
	}

	return txb, sig, nil
}

func (b *Builder) signBytes(ctx context.Context, txb client.TxBuilder, sig signing.SignatureV2) ([]byte, error) {
	signBytes, err := authsigning.GetSignBytesAdapter(ctx, b.cfg.SignModeHandler(), signing.SignMode_SIGN_MODE_DIRECT,
		authsigning.SignerData{
			Address:       b.signer.Address().String(),
			ChainID:       b.chainID,
			AccountNumber: b.accountNumber,
			Sequence:      sig.Sequence,
			PubKey:        sig.PubKey,
		}, txb.GetTx())
	if err != nil {
		return nil, fmt.Errorf("sign bytes: %w", err)
	}

	return signBytes, nil
}
