package txutil

import crypto "github.com/tendermint/go-crypto"

// Return a Signer backed by the given KeySigner (such as a crypto.PrivKey)
func NewPrivateKeySigner(key KeySigner) Signer {
	return privateKeySigner{key}
}

type privateKeySigner struct {
	key KeySigner
}

func (s privateKeySigner) Sign(tx SignableTx) error {
	sig := s.key.Sign(tx.SignBytes())
	return tx.Sign(s.key.PubKey(), sig)
}

func (s privateKeySigner) SignBytes(bytes []byte) (crypto.Signature, crypto.PubKey, error) {
	sig := s.key.Sign(bytes)
	return sig, s.key.PubKey(), nil
}

type StoreSigner interface {
	Sign(name, passphrase string, msg []byte) (crypto.Signature, crypto.PubKey, error)
}

// Return a Signer backed by a keystore
func NewKeystoreSigner(store StoreSigner, keyName, password string) Signer {
	return keyStoreSigner{store, keyName, password}
}

type keyStoreSigner struct {
	store    StoreSigner
	keyName  string
	password string
}

func (s keyStoreSigner) Sign(tx SignableTx) error {
	sig, pubkey, err := s.SignBytes(tx.SignBytes())
	if err != nil {
		return err
	}
	return tx.Sign(pubkey, sig)
}

func (s keyStoreSigner) SignBytes(bytes []byte) (crypto.Signature, crypto.PubKey, error) {
	return s.store.Sign(s.keyName, s.password, bytes)
}
