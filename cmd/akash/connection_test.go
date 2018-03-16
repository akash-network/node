package main

import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
)

func TestProviderCreate_NoNode(t *testing.T) {
	path := "providerfile.yaml"
	info, _ := testutil.NewNamedKey(t)
	args := []string{providerCommand().Name(), createProviderCommand().Name(), path, "-k", info.Name}

	base := baseCommand()
	base.AddCommand(providerCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestProviderRun_NoNode(t *testing.T) {
	info, _ := testutil.NewNamedKey(t)
	args := []string{providerCommand().Name(), runCommand().Name(), info.Address.String(), "-k", info.Name}

	base := baseCommand()
	base.AddCommand(providerCommand())
	base.SetArgs(args)
	err := base.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}
