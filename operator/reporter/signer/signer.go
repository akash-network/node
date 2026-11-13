// Package signer signs reporter transactions. The keyring signer works in
// process; remote signers sit behind the same interface.
package signer

import (
	"context"
	"errors"
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const sigLen = 64

// Signer signs transaction sign bytes with the reporter key.
type Signer interface {
	// Address is the reporter account the key derives.
	Address() sdk.AccAddress
	// PubKey is the secp256k1 key that signatures verify against.
	PubKey() cryptotypes.PubKey
	// Sign returns a 64-byte low-S r||s signature over SHA-256(signBytes).
	Sign(ctx context.Context, signBytes []byte) ([]byte, error)
}

// NormalizeAndVerify turns a raw signer answer into the signature the chain
// accepts. sig is r||s (64 bytes) or r||s||v (65 bytes, v is dropped). It
// rejects zero or out-of-range r and s, converts a high s to low-S, and checks
// the result against pub; pub.VerifySignature hashes signBytes with SHA-256
// itself. The returned slice is a new 64-byte r||s. sig is not modified.
func NormalizeAndVerify(pub cryptotypes.PubKey, signBytes, sig []byte) ([]byte, error) {
	if pub == nil {
		return nil, errors.New("missing public key")
	}

	if len(sig) != sigLen && len(sig) != sigLen+1 {
		return nil, fmt.Errorf("signature must be 64 or 65 bytes, got %d", len(sig))
	}

	var r, s secp256k1.ModNScalar
	if r.SetByteSlice(sig[:32]) || r.IsZero() {
		return nil, errors.New("signature r is out of range")
	}

	if s.SetByteSlice(sig[32:sigLen]) || s.IsZero() {
		return nil, errors.New("signature s is out of range")
	}

	if s.IsOverHalfOrder() {
		s.Negate()
	}

	out := make([]byte, sigLen)
	r.PutBytesUnchecked(out[:32])
	s.PutBytesUnchecked(out[32:])

	if !pub.VerifySignature(signBytes, out) {
		return nil, errors.New("signature does not verify against the signer key")
	}

	return out, nil
}
