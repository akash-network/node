package testutil

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	tmcli "github.com/tendermint/tendermint/libs/cli"

	pcmd "github.com/ovrclk/akash/provider/cmd"
	testutilcli "github.com/ovrclk/akash/testutil/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

/*
TestSendManifest for integration testing
this is similar to cli command exampled below
akash provider send-manifest --owner <address> \
	--dseq 7 --gseq 1 --oseq 1 \
	--provider <address> ./../_run/kube/deployment.yaml \
	--home=/tmp/akash_integration_TestE2EApp_324892307/.akashctl --node=tcp://0.0.0.0:41863
*/
func TestSendManifest(clientCtx client.Context, id mtypes.BidID, sdlPath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", id.Owner),
		fmt.Sprintf("--dseq=%v", id.DSeq),
		fmt.Sprintf("--gseq=%v", id.GSeq),
		fmt.Sprintf("--oseq=%v", id.OSeq),
		fmt.Sprintf("--provider=%s", id.Provider),
	}
	args = append(args, sdlPath)
	args = append(args, extraArgs...)
	fmt.Printf("%v\n", args)
	return testutilcli.ExecTestCLICmd(clientCtx, pcmd.SendManifestCmd(), args...)
}

const (
	TestClusterPublicHostname   = "e2e.test"
	TestClusterNodePortQuantity = 100
)

// RunLocalProvider wraps up the Provider cobra command for testing and supplies
// new default values to the flags.
// prev: akashctl provider run --from=foo --cluster-k8s --gateway-listen-address=localhost:39729 --home=/tmp/akash_integration_TestE2EApp_324892307/.akashctl --node=tcp://0.0.0.0:41863 --keyring-backend test
func RunLocalProvider(clientCtx cosmosclient.Context, chainID, nodeRPC, akashHome, from, gatewayListenAddress string, extraArgs ...string) (sdktest.BufferWriter, error) {
	cmd := pcmd.RunCmd()
	// Flags added because command not being wrapped by the Tendermint's PrepareMainCmd()
	cmd.PersistentFlags().StringP(tmcli.HomeFlag, "", akashHome, "directory for config and data")
	cmd.PersistentFlags().Bool(tmcli.TraceFlag, false, "print out full stack trace on errors")

	args := []string{
		fmt.Sprintf("--%s", pcmd.FlagClusterK8s),
		fmt.Sprintf("--%s=%s", flags.FlagChainID, chainID),
		fmt.Sprintf("--%s=%s", flags.FlagNode, nodeRPC),
		fmt.Sprintf("--%s=%s", flags.FlagHome, akashHome),
		fmt.Sprintf("--from=%s", from),
		fmt.Sprintf("--%s=%s", pcmd.FlagGatewayListenAddress, gatewayListenAddress),
		fmt.Sprintf("--%s=%s", flags.FlagKeyringBackend, keyring.BackendTest),
		fmt.Sprintf("--%s=%s", pcmd.FlagClusterPublicHostname, TestClusterPublicHostname),
		fmt.Sprintf("--%s=%d", pcmd.FlagClusterNodePortQuantity, TestClusterNodePortQuantity),
		fmt.Sprintf("--%s=%s", pcmd.FlagBidPricingStrategy, "randomRange"),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(clientCtx, cmd, args...)
}
