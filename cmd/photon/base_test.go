package main

import (
	"os"
	"testing"

	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/testutil"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootDir_Env(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	os.Setenv("PHOTON_DATA", basedir)

	assertCommand(t, func(ctx context.Context, cmd *cobra.Command, args []string) error {
		assert.Equal(t, basedir, ctx.RootDir())
		return nil
	})
}

func TestRootDir_Flag(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	os.Setenv("PHOTON_DATA", basedir)

	assertCommand(t, func(ctx context.Context, cmd *cobra.Command, args []string) error {
		assert.Equal(t, basedir, ctx.RootDir())
		return nil
	}, "-d", basedir)
}

func assertCommand(t *testing.T, fn context.Runner, args ...string) {
	viper.Reset()

	ran := false

	base := baseCommand()

	cmd := &cobra.Command{
		Use: "test",
		RunE: context.WithContext(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			ran = true
			return fn(ctx, cmd, args)
		}),
	}

	base.AddCommand(cmd)
	base.SetArgs(append([]string{"test"}, args...))
	require.NoError(t, base.Execute())
	assert.True(t, ran)
}
