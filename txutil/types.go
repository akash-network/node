package txutil

import crypto "github.com/tendermint/go-crypto"

// Transaction signer
type Signer interface {
	Sign(tx SignableTx) error
	SignBytes(bytes []byte) (crypto.Signature, crypto.PubKey, error)
}

// Subset of crypto.PrivKey used to create Signer backed by
// an in-memory private key.
type KeySigner interface {
	PubKey() crypto.PubKey
	Sign([]byte) crypto.Signature
}

type SignableTx interface {
	Sign(key crypto.PubKey, sig crypto.Signature) error
	SignBytes() []byte
}
