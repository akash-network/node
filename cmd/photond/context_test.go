package main

import (
	"os"
	"testing"

	"github.com/ovrclk/photon/testutil"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContext_RootDir_Env(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	os.Setenv("PHOTOND_DATA", basedir)

	assertCommand(t, func(ctx Context, cmd *cobra.Command, args []string) error {
		assert.Equal(t, basedir, ctx.RootDir())
		return nil
	})
}

func TestContext_RootDir_Arg(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	assertCommand(t, func(ctx Context, cmd *cobra.Command, args []string) error {
		assert.Equal(t, basedir, ctx.RootDir())
		return nil
	}, "-d", basedir)
}

func TestContext_EnvOverrides(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	gpath := "/foo/bar/genesis.json"
	vpath := "/foo/bar/private_validator.json"
	seeds := "a,b,c"
	moniker := "foobar"

	os.Setenv("PHOTOND_DATA", basedir)
	os.Setenv("PHOTOND_GENESIS", gpath)
	os.Setenv("PHOTOND_VALIDATOR", vpath)
	os.Setenv("PHOTOND_MONIKER", moniker)
	os.Setenv("PHOTOND_P2P_SEEDS", seeds)

	assertCommand(t, func(ctx Context, cmd *cobra.Command, args []string) error {
		cfg, err := ctx.TMConfig()
		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.Equal(t, basedir, ctx.RootDir())
		assert.Equal(t, gpath, cfg.GenesisFile())
		assert.Equal(t, vpath, cfg.PrivValidatorFile())
		assert.Equal(t, moniker, cfg.Moniker)
		assert.Equal(t, seeds, cfg.P2P.Seeds)
		return nil
	})

}

func assertCommand(t *testing.T, fn ctxRunner, args ...string) {
	viper.Reset()

	ran := false

	base := baseCommand()

	cmd := &cobra.Command{
		Use: "test",
		RunE: withContext(func(ctx Context, cmd *cobra.Command, args []string) error {
			ran = true
			return fn(ctx, cmd, args)
		}),
	}

	base.AddCommand(cmd)
	base.SetArgs(append([]string{"test"}, args...))
	require.NoError(t, base.Execute())
	assert.True(t, ran)
}
