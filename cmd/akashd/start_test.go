package main

import (
	"os"
	"testing"
	"time"

	"github.com/ovrclk/akash/node"
	"github.com/ovrclk/akash/testutil"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	tmconfig "github.com/tendermint/tendermint/config"
)

func Test_Start_Fail(t *testing.T) {
	testutil.WithTempDir(t, func(basedir string) {
		args := []string{startCommand().Name(), "-d", basedir}
		base := baseCommand()
		base.AddCommand(startCommand())
		base.SetArgs(args)
		require.Error(t, base.Execute())
	})
}

func Test_Start(t *testing.T) {
	testutil.WithTempDir(t, func(basedir string) {
		// init genesis data
		genesispath := basedir + "/config/genesis.json"

		os.Setenv("AKASHD_RPC_LADDR", tmconfig.TestRPCConfig().ListenAddress)

		viper.Reset()

		state := testutil.NewState(t, nil)
		address, _ := testutil.CreateAccount(t, state)
		addr := address.Address.EncodeString()

		args := []string{initCommand().Name(), addr, "-d", basedir}

		base := baseCommand()
		base.AddCommand(initCommand())
		base.SetArgs(args)
		require.NoError(t, base.Execute())

		tmgenesis, err := node.TMGenesisFromFile(genesispath)
		require.NoError(t, err)
		_, err = node.GenesisFromTMGenesis(tmgenesis)
		require.NoError(t, err)

		// run node
		startargs := []string{startCommand().Name(), "-d", basedir}
		startbase := baseCommand()
		startbase.SetArgs(startargs)
		testCtx := newContext(startbase)
		startbase.AddCommand(testStartCommand(testCtx))

		errch := make(chan error, 1)
		go func() {
			errch <- startbase.Execute()
		}()

		select {
		case <-time.After(2 * time.Second):
			// cancel the process
			testCtx.Cancel()
		case err := <-errch:
			require.NoError(t, err)
		}
	})
}

func testStartCommand(ctx Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start node",
		RunE:  testWithContext(ctx, doStartCommand),
	}
	return cmd
}

func testWithContext(ctx Context, fn ctxRunner) cmdRunner {
	return func(cmd *cobra.Command, args []string) error {
		return fn(ctx, cmd, args)
	}
}
