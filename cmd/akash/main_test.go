package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHomeFlag tests that the --home flag is working.
// This test has been added because the --home flag keeps breaking with cosmos-sdk upgrades.
func TestHomeFlag(t *testing.T) {
	// create a temp directory to store the init configuration
	tmpDir, err := ioutil.TempDir("", "akash-home")
	require.NoError(t, err)

	// Run init genesis command, it should create "config/genesis.json" in the given home directory.
	// $ akash --home=tmpDir init test-node
	os.Args = []string{"akash", fmt.Sprintf("--home=%s", tmpDir), "init", "test-node"}
	main()

	// if the genesis.json file gets created in tmpDir that means --home flag is working correctly.
	_, err = os.Stat(path.Join(tmpDir, "config", "genesis.json"))
	require.NoError(t, os.RemoveAll(tmpDir)) // do cleanup before failing on err
	require.NoError(t, err)
}
