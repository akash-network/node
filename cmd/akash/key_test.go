package main

import (
	"os"
	"testing"

	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/testutil"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestKeyCreateCommand(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	os.Setenv("AKASH_DATA", basedir)

	const keyName = "foo"

	{
		viper.Reset()
		base := baseCommand()
		base.AddCommand(keyCommand())
		base.SetArgs([]string{"key", "create", keyName})
		require.NoError(t, base.Execute())
	}

	{
		viper.Reset()
		base := baseCommand()
		cmd := &cobra.Command{
			Use: "test",
			RunE: context.WithContext(func(ctx context.Context, cmd *cobra.Command, args []string) error {
				key, err := ctx.Key()
				require.NoError(t, err)
				require.Equal(t, keyName, key.Name)
				return nil
			}),
		}
		context.AddFlagKey(cmd, cmd.Flags())

		base.AddCommand(cmd)
		base.SetArgs([]string{"test", "-k", keyName})
		require.NoError(t, base.Execute())
	}
}
