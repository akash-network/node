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

// func TestProviderRun_NoNode(t *testing.T) {
// 	info, _ := testutil.NewNamedKey(t)
// 	args := []string{providerCommand().Name(), runCommand().Name(), info.Address.String(), "-k", info.Name}

// 	base := baseCommand()
// 	base.AddCommand(providerCommand())
// 	base.SetArgs(args)
// 	err := base.Execute()
// 	assert.Error(t, err)
// 	assert.Contains(t, err.Error(), "connection refused")
// }

// func TestDeployment_NoNode(t *testing.T) {
// 	path := "deployment.yaml"
// 	info, _ := testutil.NewNamedKey(t)
// 	args := []string{deploymentCommand().Name(), path, "-k", info.Name}

// 	base := baseCommand()
// 	base.AddCommand(deploymentCommand())
// 	base.SetArgs(args)
// 	err := base.Execute()
// 	assert.Error(t, err)
// 	assert.Contains(t, err.Error(), "connection refused")
// }

// func TestMarketplace_NoNode(t *testing.T) {
// 	args := []string{marketplaceCommand().Name()}
// 	base := baseCommand()
// 	base.AddCommand(marketplaceCommand())
// 	base.SetArgs(args)
// 	err := base.Execute()
// 	assert.Error(t, err)
// 	assert.Contains(t, err.Error(), "connection refused")
// }

// func TestSend_NoNode(t *testing.T) {
// 	from, _ := testutil.NewNamedKey(t)
// 	to, _ := testutil.NewNamedKey(t)
// 	amount := "1"
// 	args := []string{sendCommand().Name(), amount, to.Address.String(), "-k", from.Name}
// 	base := baseCommand()
// 	base.AddCommand(sendCommand())
// 	base.SetArgs(args)
// 	err := base.Execute()
// 	assert.Error(t, err)
// 	assert.Contains(t, err.Error(), "connection refused")
// }

// func TestStatus_NoNode(t *testing.T) {
// 	args := []string{statusCommand().Name()}
// 	base := baseCommand()
// 	base.AddCommand(statusCommand())
// 	base.SetArgs(args)
// 	err := base.Execute()
// 	assert.Error(t, err)
// 	assert.Contains(t, err.Error(), "connection refused")
// }
