package signer

import (
	"context"
	"errors"
	"fmt"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
)

var errMismatch = errors.New("signer does not match reporter")

type keyringSigner struct {
	kr   keyring.Keyring
	uid  string
	addr sdk.AccAddress
	pub  cryptotypes.PubKey
}

var _ Signer = (*keyringSigner)(nil)

// NewKeyring opens the key `from` in kr, by name first and then by bech32
// address. The key must be secp256k1 and derive reporter.
func NewKeyring(kr keyring.Keyring, from string, reporter sdk.AccAddress) (Signer, error) {
	rec, err := lookup(kr, from)
	if err != nil {
		return nil, err
	}

	pub, err := rec.GetPubKey()
	if err != nil {
		return nil, fmt.Errorf("read public key of %q: %w", from, err)
	}

	if _, ok := pub.(*secp256k1.PubKey); !ok {
		return nil, fmt.Errorf("key %q is not secp256k1", from)
	}

	addr := sdk.AccAddress(pub.Address())
	if !addr.Equals(reporter) {
		return nil, errMismatch
	}

	return &keyringSigner{kr: kr, uid: rec.Name, addr: addr, pub: pub}, nil
}

func lookup(kr keyring.Keyring, from string) (*keyring.Record, error) {
	rec, nameErr := kr.Key(from)
	if nameErr == nil {
		return rec, nil
	}

	_, raw, err := bech32.DecodeAndConvert(from)
	if err != nil {
		return nil, fmt.Errorf("find key %q: %w", from, nameErr)
	}

	rec, err = kr.KeyByAddress(sdk.AccAddress(raw))
	if err != nil {
		return nil, fmt.Errorf("find key %q: %w", from, err)
	}

	return rec, nil
}

func (s *keyringSigner) Address() sdk.AccAddress { return s.addr }

func (s *keyringSigner) PubKey() cryptotypes.PubKey { return s.pub }

func (s *keyringSigner) Sign(ctx context.Context, signBytes []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sig, _, err := s.kr.Sign(s.uid, signBytes, signing.SignMode_SIGN_MODE_DIRECT)
	if err != nil {
		return nil, fmt.Errorf("keyring sign: %w", err)
	}

	return NormalizeAndVerify(s.pub, signBytes, sig)
}
