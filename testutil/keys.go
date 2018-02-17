package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/go-crypto/keys"
	"github.com/tendermint/go-crypto/keys/cryptostore"
	"github.com/tendermint/go-crypto/keys/storage/memstorage"
)

const (
	KeyPasswd = "0123456789"
	KeyAlgo   = "ed25519"
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
