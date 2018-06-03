package manifest

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	crypto "github.com/tendermint/go-crypto"
)

func TestSignManifest(t *testing.T) {
	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	deployment := testutil.DeploymentAddress(t)

	mr, buf, err := SignManifest(mani, signer, deployment)
	assert.NoError(t, err)

	gotmr, err := unmarshalRequest(bytes.NewReader(buf))
	assert.NoError(t, err)

	assert.Equal(t, mr.Key, gotmr.Key)
	assert.Equal(t, mr.Signature, gotmr.Signature)
	assert.Equal(t, mr.Deployment, gotmr.Deployment)

	_, err = verifySignature(gotmr)
	assert.NoError(t, err)
}

func TestVerifySig(t *testing.T) {
	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	deployment := testutil.DeploymentAddress(t)

	mr, _, err := SignManifest(mani, signer, deployment)
	assert.NoError(t, err)

	_, err = verifySignature(mr)
	assert.NoError(t, err)
}

func TestVerifySig_InvalidSig(t *testing.T) {
	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	deployment := testutil.DeploymentAddress(t)

	mr, _, err := SignManifest(mani, signer, deployment)
	assert.NoError(t, err)

	_, otherKmgr := testutil.NewNamedKey(t)
	otherSigner := testutil.Signer(t, otherKmgr)
	otherMr, _, err := SignManifest(mani, otherSigner, deployment)
	assert.NoError(t, err)

	mr.Key = otherMr.Key

	_, err = verifySignature(mr)
	assert.Error(t, err)
}

func TestDoPost(t *testing.T) {

	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	deployment := testutil.DeploymentAddress(t)

	mr, buf, err := SignManifest(mani, signer, deployment)
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotmr, err := unmarshalRequest(bytes.NewReader(buf))
		assert.NoError(t, err)

		assert.Equal(t, mr.Key, gotmr.Key)
		assert.Equal(t, mr.Signature, gotmr.Signature)
		assert.Equal(t, mr.Deployment, gotmr.Deployment)

		pbytes, err := marshalRequest(&types.ManifestRequest{
			Deployment: gotmr.Deployment,
			Manifest:   gotmr.Manifest,
		})
		assert.NoError(t, err)

		key, err := crypto.PubKeyFromBytes(gotmr.Key)
		assert.NoError(t, err)

		sig, err := crypto.SignatureFromBytes(gotmr.Signature)
		assert.NoError(t, err)

		if !key.VerifyBytes(pbytes, sig) {
			t.Error("invalid signature")
		}
	}))
	defer ts.Close()

	err = post(ts.URL, buf)
	assert.NoError(t, err)
}
