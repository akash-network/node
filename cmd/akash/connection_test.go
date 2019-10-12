package main

import (
	"testing"

	"github.com/ovrclk/akash/cmd/akash/deployment"
	"github.com/ovrclk/akash/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestProviderCreate_NoNode(t *testing.T) {
	testutil.Shrug(t, 338)
	doTest_NoNode(t, providerCommand(),
		createProviderCommand().Name(), "provider.yml", "-k", "keyname")
}

func TestProviderRun_NoNode(t *testing.T) {
	testutil.Shrug(t, 338)
	hexaddr := testutil.HexAddress(t)
	doTest_NoNode(t, providerCommand(),
		runCommand().Name(), hexaddr, "-k", "keyname")
}

func TestMarketplace_NoNode(t *testing.T) {
	testutil.Shrug(t, 338)
	doTest_NoNode(t, marketplaceCommand())
}

func TestSend_NoNode(t *testing.T) {
	testutil.Shrug(t, 338)
	to := testutil.HexAddress(t)
	doTest_NoNode(t, sendCommand(),
		"1", to, "-k", "keyname")
}

func TestStatus_NoNode(t *testing.T) {
	testutil.Shrug(t, 338)
	doTest_NoNode(t, statusCommand())
}

func doTest_NoNode(t *testing.T, cmd *cobra.Command, args ...string) {
	testutil.WithAkashDir(t, func(_ string) {
		args := append([]string{cmd.Name()}, args...)

		base := baseCommand()
		base.AddCommand(cmd)
		base.SetArgs(args)

		err := base.Execute()
		assert.Error(t, err)
	})
}

func TestCreateDeployment_NoNode(t *testing.T) {
	testutil.Shrug(t, 338)
	path := "deployment.yaml"
	doTest_NoNode(t, deployment.Command(),
		deployment.CreateCmd().Name(), path, "-k", "keyname")
}

func TestCloseDeployment_NoNode(t *testing.T) {
	testutil.Shrug(t, 338)
	dep := testutil.HexDeploymentAddress(t)
	doTest_NoNode(t, deployment.Command(),
		deployment.CloseCmd().Name(), dep, "-k", "keyname")
}
