package market

import (
	crypto "github.com/tendermint/tendermint/crypto"
)

type Actor interface {
	Sign([]byte) ([]byte, error)
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

func (a actor) Sign(msg []byte) ([]byte, error) {
	return a.key.Sign(msg)
}
