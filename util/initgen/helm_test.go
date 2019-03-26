package initgen_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/ovrclk/akash/node"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/types"
	yaml "gopkg.in/yaml.v2"
)

func TestHelmWriter(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	ctx, err := NewBuilder().
		WithName("foo").
		WithCount(1).
		WithPath(basedir).
		Create()
	require.NoError(t, err)

	w, err := CreateWriter(TypeHelm, ctx)
	require.NoError(t, err)
	require.NoError(t, w.Write())

	path := path.Join(basedir, ctx.Name()+".yaml")
	assert.FileExists(t, path)

	buf, err := ioutil.ReadFile(path)
	require.NoError(t, err)

	hobj := new(helmConfig)
	require.NoError(t, yaml.Unmarshal(buf, &hobj))

	require.Equal(t, hobj.Node.Name, ctx.Name())

	gobj, err := tmtypes.GenesisDocFromJSON([]byte(hobj.Node.Genesis))
	require.NoError(t, err)

	require.Equal(t, ctx.Genesis().Validators, gobj.Validators)

	pobj, err := node.PVFromJSON([]byte(hobj.Node.Validator))
	require.NoError(t, err)
	require.Equal(t, ctx.Nodes()[0].PrivateValidator.GetPubKey(), pobj.GetPubKey())
}

func TestMultiHelmWriter(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	ctx, err := NewBuilder().
		WithName("foo").
		WithCount(2).
		WithPath(basedir).
		Create()
	require.NoError(t, err)

	w, err := CreateWriter(TypeHelm, ctx)
	require.NoError(t, err)
	require.NoError(t, w.Write())

	assert.FileExists(t, path.Join(basedir, "foo-0.yaml"))
	assert.FileExists(t, path.Join(basedir, "foo-1.yaml"))
}
