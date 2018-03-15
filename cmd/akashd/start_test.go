package main

import (
	"fmt"
	"io/ioutil"
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
		emmptydir := testutil.TempDir(t)
		defer os.RemoveAll(emmptydir)
		args := []string{startCommand().Name(), "-d", emmptydir}
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

		ls(basedir + "/config")

		base := baseCommand()
		base.AddCommand(initCommand())
		base.SetArgs(args)
		require.NoError(t, base.Execute())

		ls(basedir + "/config")

		tmgenesis, err := node.TMGenesisFromFile(genesispath)
		require.NoError(t, err)
		_, err = node.GenesisFromTMGenesis(tmgenesis)
		require.NoError(t, err)

		ls(basedir + "/config")

		// run node
		startargs := []string{startCommand().Name(), "-d", basedir}
		startbase := baseCommand()
		startbase.AddCommand(startCommand())
		startbase.SetArgs(startargs)

		ls(basedir + "/config")

		errchan := make(chan error)
		quit := make(chan bool)
		defer close(errchan)
		defer close(quit)

		time.AfterFunc(3*time.Second, func() { quit <- true })

		started := false
		run := true
		for run {
			if started == false {
				go func() {
					ls(basedir + "/config")
					errchan <- startbase.Execute()
					ls(basedir + "/config")
				}()
				started = true
			}
			if <-quit {
				break
			}
		}

		ls(basedir + "/config")
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

func ls(dir string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		println("err:", err.Error())
	}

	println("Files in ", dir)
	for _, f := range files {
		fmt.Println(f.Name())
	}
}
