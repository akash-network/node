package manifest

import (
	"bytes"
	"testing"

	"github.com/ovrclk/akash/provider/session"
	qmocks "github.com/ovrclk/akash/query/mocks"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

func TestVerifyDeploymentTenant(t *testing.T) {
	info, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)
	tenant := info.Address()
	deployment := testutil.Deployment(tenant, 1)
	providerID := testutil.Address(t)
	provider := testutil.Provider(providerID, 4)
	client := &qmocks.Client{}
	client.On("Deployment",
		mock.Anything,
		[]byte(deployment.Address)).Return(deployment, nil)
	sess := session.New(testutil.Logger(), provider, nil, client)
	mani := &types.Manifest{}
	mreq, _, err := SignManifest(mani, signer, deployment.Address)
	require.NoError(t, err)
	err = verifyDeploymentTenant(mreq, sess, info.Address())
	assert.NoError(t, err)
}

func TestVerifyDeploymentTenant_InvalidKey(t *testing.T) {
	info, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)
	tenant := info.Address()
	deployment := testutil.Deployment(tenant, 1)
	providerID := testutil.Address(t)
	provider := testutil.Provider(providerID, 4)
	client := &qmocks.Client{}
	client.On("Deployment",
		mock.Anything,
		[]byte(deployment.Address)).Return(deployment, nil)
	sess := session.New(testutil.Logger(), provider, nil, client)
	mani := &types.Manifest{}
	mreq, _, err := SignManifest(mani, signer, deployment.Address)
	require.NoError(t, err)
	err = verifyDeploymentTenant(mreq, sess, info.Address())
	assert.NoError(t, err)
}

func TestVerifyRequest(t *testing.T) {
	info, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)
	tenant := info.Address()
	mani := &types.Manifest{}
	version, err := Hash(mani)
	require.NoError(t, err)
	deployment := testutil.Deployment(tenant, 1, version)
	providerID := testutil.Address(t)
	provider := testutil.Provider(providerID, 4)
	client := &qmocks.Client{}
	client.On("Deployment",
		mock.Anything,
		[]byte(deployment.Address)).Return(deployment, nil)
	sess := session.New(testutil.Logger(), provider, nil, client)
	mreq, _, err := SignManifest(mani, signer, deployment.Address)
	require.NoError(t, err)
	err = VerifyRequest(mreq, sess)
	assert.NoError(t, err)
}

func TestHash(t *testing.T) {
	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, err = Hash(mani)
	assert.NoError(t, err)
}

func TestVerifyHash(t *testing.T) {
	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	hash, err := Hash(mani)
	require.NoError(t, err)

	otherHash, err := Hash(mani)
	require.NoError(t, err)

	assert.Equal(t, hash, otherHash)
}

func TestVerifyHash_Invalid(t *testing.T) {
	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	hash, err := Hash(mani)
	require.NoError(t, err)

	otherHash, err := Hash(&types.Manifest{
		Groups: []*types.ManifestGroup{
			{
				Name: "otherManifest",
			},
		},
	})
	require.NoError(t, err)

	assert.NotEqual(t, hash, otherHash)
}
