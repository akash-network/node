package initgen_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/ovrclk/photon/util/initgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/types"
	yaml "gopkg.in/yaml.v2"
)

func TestHelmWriter(t *testing.T) {
	basedir, err := ioutil.TempDir("", "photon-test-initgen")
	require.NoError(t, err)
	defer os.RemoveAll(basedir)

	ctx, err := initgen.NewBuilder().
		WithName("foo").
		WithCount(1).
		WithPath(basedir).
		Create()
	require.NoError(t, err)

	w, err := initgen.CreateWriter(initgen.TypeHelm, ctx)
	require.NoError(t, err)
	require.NoError(t, w.Write())

	path := path.Join(basedir, ctx.Name()+".yaml")
	assert.FileExists(t, path)

	buf, err := ioutil.ReadFile(path)
	require.NoError(t, err)

	hobj := new(initgen.HelmConfig)
	require.NoError(t, yaml.Unmarshal(buf, &hobj))

	require.Equal(t, hobj.Node.Name, ctx.Name())

	gobj := new(tmtypes.GenesisDoc)
	require.NoError(t, json.Unmarshal([]byte(hobj.Node.Genesis), gobj))

	require.Equal(t, ctx.Genesis().Validators, gobj.Validators)

	pobj := new(tmtypes.PrivValidatorFS)
	require.NoError(t, json.Unmarshal([]byte(hobj.Node.Validator), pobj))
	require.Equal(t, ctx.PrivateValidators()[0].GetPubKey(), pobj.GetPubKey())
}

func TestMultiHelmWriter(t *testing.T) {
	basedir, err := ioutil.TempDir("", "photon-test-initgen")
	require.NoError(t, err)
	defer os.RemoveAll(basedir)

	ctx, err := initgen.NewBuilder().
		WithName("foo").
		WithCount(2).
		WithPath(basedir).
		Create()
	require.NoError(t, err)

	w, err := initgen.CreateWriter(initgen.TypeHelm, ctx)
	require.NoError(t, err)
	require.NoError(t, w.Write())

	assert.FileExists(t, path.Join(basedir, "foo-0.yaml"))
	assert.FileExists(t, path.Join(basedir, "foo-1.yaml"))
}
