package main

import (
	"os"
	"testing"
	"time"

	"github.com/ovrclk/akash/node"
	"github.com/ovrclk/akash/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func Test_Start(t *testing.T) {

	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	{
		args := []string{startCommand().Name(), "-d", basedir}
		base := baseCommand()
		base.AddCommand(startCommand())
		base.SetArgs(args)
		println("////////// start ignore expected error //////////")
		require.Error(t, base.Execute())
		println("////////// end ignore expected error //////////")
	}

	{
		// init genesis data
		genesispath := basedir + "/config/genesis.json"
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
		startbase.AddCommand(startCommand())
		startbase.SetArgs(startargs)

		errchan := make(chan error)
		quit := make(chan bool)
		defer close(errchan)
		defer close(quit)

		time.AfterFunc(5*time.Second, func() { quit <- true })

		started := false
		run := true
		for run {
			if started == false {
				go func() {
					errchan <- startbase.Execute()
				}()
				started = true
			}
			if <-quit {
				break
			}
		}

		err = emptyErrChannel(errchan)
		require.NoError(t, err)
	}
}

func emptyErrChannel(ch chan error) error {
	select {
	case x, _ := <-ch:
		return x
	default:
		return nil
	}
}
