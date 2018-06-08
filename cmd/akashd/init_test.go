package main

import (
	"os"
	"testing"

	"github.com/ovrclk/akash/node"
	"github.com/ovrclk/akash/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Init(t *testing.T) {
	basedir := testutil.TempDir(t)
	genesispath := basedir + "/config/genesis.json"
	defer os.RemoveAll(basedir)
	viper.Reset()

	state, _ := testutil.NewState(t, nil)
	address, _ := testutil.CreateAccount(t, state)
	addr := address.Address.EncodeString()

	args := []string{initCommand().Name(), addr, "-d", basedir}

	base := baseCommand()
	base.AddCommand(initCommand())
	base.SetArgs(args)
	require.NoError(t, base.Execute())

	tmgenesis, err := node.TMGenesisFromFile(genesispath)
	require.NoError(t, err)
	genesis, err := node.GenesisFromTMGenesis(tmgenesis)
	require.NoError(t, err)
	assert.Equal(t, addr, genesis.Accounts[0].Address.EncodeString())
}
