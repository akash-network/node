package testutil

import (
	"fmt"

	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	tmcli "github.com/tendermint/tendermint/libs/cli"

	pcmd "github.com/ovrclk/akash/provider/cmd"
	testutilcli "github.com/ovrclk/akash/testutil/cli"
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
		pcmd.FlagClusterK8s,
		fmt.Sprintf("--%s=%s", flags.FlagChainID, chainID),
		fmt.Sprintf("--%s=%s", flags.FlagNode, nodeRPC),
		fmt.Sprintf("--%s=%s", flags.FlagHome, akashHome),
		fmt.Sprintf("--from=%s", from),
		fmt.Sprintf("--%s=%s", pcmd.FlagGatewayListenAddress, gatewayListenAddress),
		fmt.Sprintf("--%s=%s", flags.FlagKeyringBackend, keyring.BackendTest),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(clientCtx, cmd, args...)
}
