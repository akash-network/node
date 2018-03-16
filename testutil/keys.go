package testutil

import (
	"testing"

	"github.com/ovrclk/akash/txutil"
	"github.com/stretchr/testify/require"
	crypto "github.com/tendermint/go-crypto"
	"github.com/tendermint/go-crypto/keys"
	"github.com/tendermint/go-crypto/keys/cryptostore"
	"github.com/tendermint/go-crypto/keys/storage/memstorage"
)

const (
	KeyPasswd = "0123456789"
	KeyAlgo   = "ed25519"
	KeyName   = "test"
)

func KeyManager(t *testing.T) cryptostore.Manager {
	codec, err := keys.LoadCodec("english")
	require.NoError(t, err)

	return cryptostore.New(
		cryptostore.SecretBox,
		memstorage.New(),
		codec,
	)
}

func PrivateKey(t *testing.T) crypto.PrivKey {
	secret := crypto.CRandBytes(16)
	key, err := cryptostore.GenEd25519.Generate(secret)
	require.NoError(t, err)
	return key
}

func PrivateKeySigner(t *testing.T) (txutil.Signer, crypto.PrivKey) {
	key := PrivateKey(t)
	return txutil.NewPrivateKeySigner(key), key
}

func NewNamedKey(t *testing.T) (keys.Info, cryptostore.Manager) {
	kmgr := KeyManager(t)
	info, _, err := kmgr.Create(KeyName, KeyPasswd, KeyAlgo)
	require.NoError(t, err)
	return info, kmgr
}
