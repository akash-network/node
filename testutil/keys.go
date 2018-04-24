package testutil

import (
	"testing"

	"github.com/ovrclk/akash/txutil"
	"github.com/stretchr/testify/require"
	crypto "github.com/tendermint/go-crypto"
	"github.com/tendermint/go-crypto/keys"
	"github.com/tendermint/go-crypto/keys/words"
	tmdb "github.com/tendermint/tmlibs/db"
)

const (
	KeyPasswd = "0123456789"
	KeyAlgo   = "ed25519"
	KeyName   = "test"
)

func KeyManager(t *testing.T) keys.Keybase {
	codec, err := words.LoadCodec("english")
	require.NoError(t, err)
	db := tmdb.NewMemDB()
	return keys.New(db, codec)
}

func PrivateKey(t *testing.T) crypto.PrivKey {
	return crypto.GenPrivKeyEd25519()
}

func PublicKey(t *testing.T) crypto.PubKey {
	return PrivateKey(t).PubKey()
}

func PrivateKeySigner(t *testing.T) (txutil.Signer, crypto.PrivKey) {
	key := PrivateKey(t)
	return txutil.NewPrivateKeySigner(key), key
}

func NewNamedKey(t *testing.T) (keys.Info, keys.Keybase) {
	kmgr := KeyManager(t)
	info, _, err := kmgr.Create(KeyName, KeyPasswd, KeyAlgo)
	require.NoError(t, err)
	return info, kmgr
}

func Signer(t *testing.T, kmgr keys.Keybase) txutil.Signer {
	return txutil.NewKeystoreSigner(kmgr, KeyName, KeyPasswd)
}
