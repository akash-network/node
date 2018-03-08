package txutil

import (
	crypto "github.com/tendermint/go-crypto"
	"github.com/tendermint/go-crypto/keys"
)

// Transaction signer
type Signer interface {
	Sign(tx keys.Signable) error
}

// Subset of crypto.PrivKey used to create Signer backed by
// an in-memory private key.
type KeySigner interface {
	PubKey() crypto.PubKey
	Sign([]byte) crypto.Signature
}

// Return a Signer backed by the given KeySigner (such as a crypto.PrivKey)
func NewPrivateKeySigner(key KeySigner) Signer {
	return privateKeySigner{key}
}

type privateKeySigner struct {
	key KeySigner
}

func (s privateKeySigner) Sign(tx keys.Signable) error {
	sig := s.key.Sign(tx.SignBytes())
	return tx.Sign(s.key.PubKey(), sig)
}

// Return a Signer backed by a keystore
func NewKeystoreSigner(store keys.Signer, keyName, password string) Signer {
	return keyStoreSigner{store, keyName, password}
}

type keyStoreSigner struct {
	store    keys.Signer
	keyName  string
	password string
}

func (s keyStoreSigner) Sign(tx keys.Signable) error {
	return s.store.Sign(s.keyName, s.password, tx)
}
