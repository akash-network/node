package txutil

import crypto "github.com/tendermint/tendermint/crypto"

// Transaction signer
type Signer interface {
	Sign(tx SignableTx) error
	SignBytes(bytes []byte) ([]byte, crypto.PubKey, error)
}

// Subset of crypto.PrivKey used to create Signer backed by
// an in-memory private key.
type KeySigner interface {
	PubKey() crypto.PubKey
	Sign([]byte) ([]byte, error)
}

type SignableTx interface {
	Sign(key crypto.PubKey, sig []byte) error
	SignBytes() []byte
}
