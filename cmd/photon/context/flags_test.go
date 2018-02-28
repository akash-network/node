package context_test

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flagFn func(cmd *cobra.Command, flags *pflag.FlagSet)

func TestNode_Env(t *testing.T) {
	const val = "foo.bar:123"

	os.Setenv("PHOTON_NODE", val)

	assertCommand(t, context.AddFlagNode, func(ctx context.Context, cmd *cobra.Command, args []string) error {
		assert.Equal(t, val, ctx.Node())
		return nil
	})
}

func TestNode_Flag(t *testing.T) {
	const val = "foo.bar:123"

	assertCommand(t, context.AddFlagNode, func(ctx context.Context, cmd *cobra.Command, args []string) error {
		assert.Equal(t, val, ctx.Node())
		return nil
	}, "-n", val)
}

func TestKey_Flag(t *testing.T) {
	const val = "foo"

	assertCommand(t, context.AddFlagKey, func(ctx context.Context, cmd *cobra.Command, args []string) error {
		assert.Equal(t, val, ctx.KeyName())
		return nil
	}, "-k", val)
}

func TestFlag_Nonce(t *testing.T) {
	const val uint64 = 10

	assertCommand(t, context.AddFlagNonce, func(ctx context.Context, cmd *cobra.Command, args []string) error {
		nonce, err := ctx.Nonce()
		require.NoError(t, err)
		require.Equal(t, val, nonce)
		return nil
	}, "--"+constants.FlagNonce, strconv.Itoa(int(val)))
}

func assertCommand(t *testing.T, flagfn flagFn, fn context.Runner, args ...string) {
	basedir, err := ioutil.TempDir("", "photon-photon-context")
	require.NoError(t, err)
	defer os.RemoveAll(basedir)

	viper.Reset()

	ran := false

	os.Setenv("PHOTON_DATA", basedir)

	cmd := &cobra.Command{
		Use: "test",
		RunE: context.WithContext(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			ran = true
			require.Equal(t, basedir, ctx.RootDir())
			return fn(ctx, cmd, args)
		}),
	}

	context.SetupBaseCommand(cmd)

	if flagfn != nil {
		flagfn(cmd, cmd.Flags())
	}

	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	assert.True(t, ran)
}
