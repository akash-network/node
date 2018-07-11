package testutil

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys"
	"github.com/ovrclk/akash/txutil"
	"github.com/stretchr/testify/require"
	crypto "github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmdb "github.com/tendermint/tendermint/libs/db"
)

const (
	KeyPasswd = "0123456789"
	KeyAlgo   = keys.Secp256k1
	KeyName   = "test"
	Language  = keys.English
)

func KeyManager(t *testing.T) keys.Keybase {
	db := tmdb.NewMemDB()
	return keys.New(db)
}

func PrivateKey(t *testing.T) crypto.PrivKey {
	return ed25519.GenPrivKey()
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
	info, _, err := kmgr.CreateMnemonic(KeyName, Language, KeyPasswd, KeyAlgo)
	require.NoError(t, err)
	return info, kmgr
}

func Signer(t *testing.T, kmgr keys.Keybase) txutil.Signer {
	return txutil.NewKeystoreSigner(kmgr, KeyName, KeyPasswd)
}
