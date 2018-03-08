package initgen_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/util/initgen"
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

		obj := new(tmtypes.GenesisDoc)
		require.NoError(t, json.Unmarshal(buf, obj))

		require.Equal(t, ctx.Genesis().Validators, obj.Validators)
	}

	{
		path := path.Join(basedir, initgen.ConfigDir, initgen.PrivateValidatorFilename)
		assert.FileExists(t, path)

		buf, err := ioutil.ReadFile(path)
		require.NoError(t, err)

		obj := new(tmtypes.PrivValidatorFS)
		require.NoError(t, json.Unmarshal(buf, obj))
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
