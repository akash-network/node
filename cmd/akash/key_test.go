package main

import (
	"testing"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/testutil"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestKeyCreateCommand(t *testing.T) {
	testutil.WithAkashDir(t, func(_ string) {

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
				RunE: session.WithSession(func(session session.Session, cmd *cobra.Command, args []string) error {
					key, err := session.Key()
					require.NoError(t, err)
					require.Equal(t, keyName, key.GetName())
					return nil
				}),
			}
			session.AddFlagKey(cmd, cmd.Flags())

			base.AddCommand(cmd)
			base.SetArgs([]string{"test", "-k", keyName})
			require.NoError(t, base.Execute())
		}
	})
}
