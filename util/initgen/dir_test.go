package initgen_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/util/initgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/p2p"
	tmtypes "github.com/tendermint/tendermint/types"
)

func TestDirWriter(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)
	ctx, err := initgen.NewBuilder().
		WithNames([]string{"foo"}).
		WithPath(basedir).
		Create()
	require.NoError(t, err)

	w, err := initgen.CreateWriter(initgen.TypeDirectory, ctx)
	require.NoError(t, err)
	require.NoError(t, w.Write())

	{
		path := path.Join(basedir, initgen.ConfigDir, initgen.GenesisFilename)
		assert.FileExists(t, path)

		buf, err := ioutil.ReadFile(path)
		require.NoError(t, err)

		obj, err := tmtypes.GenesisDocFromJSON(buf)
		require.NoError(t, err)

		require.Equal(t, ctx.Genesis().Validators, obj.Validators)
	}

	// TODO: Add tests for FilePV
	// {
	// 	path := path.Join(basedir, initgen.ConfigDir, initgen.PrivateValidatorFilename)
	// 	assert.FileExists(t, path)

	// 	obj, err := node.PVFromFile(path)
	// 	require.NoError(t, err)
	// 	require.Equal(t, ctx.Nodes()[0].PrivateValidator.GetPubKey(), obj.GetPubKey())
	// }

	{
		path := path.Join(basedir, initgen.ConfigDir, initgen.NodeKeyFilename)
		assert.FileExists(t, path)

		obj, err := p2p.LoadNodeKey(path)
		require.NoError(t, err)
		require.Equal(t, ctx.Nodes()[0].NodeKey, obj)
	}
}

func TestMultiDirWriter(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	ctx, err := initgen.NewBuilder().
		WithNames([]string{"foo", "bar"}).
		WithPath(basedir).
		Create()
	require.NoError(t, err)

	w, err := initgen.CreateWriter(initgen.TypeDirectory, ctx)
	require.NoError(t, err)
	require.NoError(t, w.Write())

	assert.FileExists(t, path.Join(basedir, "foo", initgen.ConfigDir, initgen.GenesisFilename))
	assert.FileExists(t, path.Join(basedir, "foo", initgen.ConfigDir, initgen.PVKeyFilename))

	assert.FileExists(t, path.Join(basedir, "bar", initgen.ConfigDir, initgen.GenesisFilename))
	assert.FileExists(t, path.Join(basedir, "bar", initgen.ConfigDir, initgen.PVKeyFilename))
}
