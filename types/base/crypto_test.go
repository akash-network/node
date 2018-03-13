package base_test

import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPubKey_JSON(t *testing.T) {
	kmgr := testutil.KeyManager(t)

	kinfo, _, err := kmgr.Create("key", testutil.KeyPasswd, testutil.KeyAlgo)

	key := base.PubKey(kinfo.PubKey)

	js, err := key.MarshalJSON()
	require.NoError(t, err)

	nkey := new(base.PubKey)

	require.NoError(t, nkey.UnmarshalJSON(js))

	assert.Equal(t, key, *nkey)
}

func TestSignature_JSON(t *testing.T) {
	const nonce = 1

	kmgr := testutil.KeyManager(t)

	_, _, err := kmgr.Create("key", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	b, err := txutil.NewTxBuilder(nonce, &types.TxSend{})
	require.NoError(t, err)

	require.NoError(t, kmgr.Sign("key", testutil.KeyPasswd, b))

	sig := b.Signature()
	require.NotNil(t, sig)

	js, err := sig.MarshalJSON()
	require.NoError(t, err)

	nsig := new(base.Signature)

	require.NoError(t, nsig.UnmarshalJSON(js))
	require.Equal(t, sig, nsig)
}
