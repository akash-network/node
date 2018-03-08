package market

import (
	crypto "github.com/tendermint/go-crypto"
)

type Actor interface {
	Sign([]byte) crypto.Signature
	PubKey() crypto.PubKey
	Address() []byte
}

type actor struct {
	key crypto.PrivKey
}

func NewActor(key crypto.PrivKey) Actor {
	return actor{key}
}

func (a actor) PubKey() crypto.PubKey {
	return a.key.PubKey()
}

func (a actor) Address() []byte {
	return a.PubKey().Address()
}

func (a actor) Sign(msg []byte) crypto.Signature {
	return a.key.Sign(msg)
}
