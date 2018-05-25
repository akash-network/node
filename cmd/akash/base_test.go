package main

import (
	"testing"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/testutil"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootDir_Env(t *testing.T) {
	testutil.WithAkashDir(t, func(basedir string) {
		assertCommand(t, func(session session.Session, cmd *cobra.Command, args []string) error {
			assert.Equal(t, basedir, session.RootDir())
			return nil
		})
	})
}

func TestRootDir_Flag(t *testing.T) {
	testutil.WithAkashDir(t, func(basedir string) {
		assertCommand(t, func(session session.Session, cmd *cobra.Command, args []string) error {
			assert.Equal(t, basedir, session.RootDir())
			return nil
		}, "-d", basedir)
	})
}

func assertCommand(t *testing.T, fn session.Runner, args ...string) {
	viper.Reset()

	ran := false

	base := baseCommand()

	cmd := &cobra.Command{
		Use: "test",
		RunE: session.WithSession(func(session session.Session, cmd *cobra.Command, args []string) error {
			ran = true
			return fn(session, cmd, args)
		}),
	}

	base.AddCommand(cmd)
	base.SetArgs(append([]string{"test"}, args...))
	require.NoError(t, base.Execute())
	assert.True(t, ran)
}
