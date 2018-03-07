package testutil

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	tmconfig "github.com/tendermint/tendermint/config"
)

func TempDir(t *testing.T) string {
	basedir, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	return basedir
}

func TMConfig(t *testing.T, basedir string) *tmconfig.Config {
	cfg := tmconfig.DefaultConfig()
	cfg.SetRoot(basedir)
	return cfg
}
