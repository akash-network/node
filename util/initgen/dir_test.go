package initgen_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/ovrclk/akash/node"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/util/initgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/types"
)

func TestDirWriter(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	ctx, err := initgen.NewBuilder().
		WithName("foo").
		WithCount(1).
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

	{
		path := path.Join(basedir, initgen.ConfigDir, initgen.PrivateValidatorFilename)
		assert.FileExists(t, path)

		obj, err := node.PVFromFile(path)
		require.NoError(t, err)
		require.Equal(t, ctx.PrivateValidators()[0].GetPubKey(), obj.GetPubKey())
	}
}

func TestMultiDirWriter(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	ctx, err := initgen.NewBuilder().
		WithName("foo").
		WithCount(2).
		WithPath(basedir).
		Create()
	require.NoError(t, err)

	w, err := initgen.CreateWriter(initgen.TypeDirectory, ctx)
	require.NoError(t, err)
	require.NoError(t, w.Write())

	assert.FileExists(t, path.Join(basedir, "foo-0", initgen.ConfigDir, initgen.GenesisFilename))
	assert.FileExists(t, path.Join(basedir, "foo-0", initgen.ConfigDir, initgen.PrivateValidatorFilename))

	assert.FileExists(t, path.Join(basedir, "foo-1", initgen.ConfigDir, initgen.GenesisFilename))
	assert.FileExists(t, path.Join(basedir, "foo-1", initgen.ConfigDir, initgen.PrivateValidatorFilename))
}
