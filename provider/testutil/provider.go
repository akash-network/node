package testutil

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	tmcli "github.com/tendermint/tendermint/libs/cli"

	pcmd "github.com/ovrclk/akash/provider/cmd"
	testutilcli "github.com/ovrclk/akash/testutil/cli"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

const (
	TestClusterPublicHostname   = "e2e.test"
	TestClusterNodePortQuantity = 100
)

var cmdLock = make(chan struct{}, 1)

func init() {
	releaseCmdLock()
}

func takeCmdLock() {
	<-cmdLock
}

func releaseCmdLock() {
	cmdLock <- struct{}{}
}

/*
TestSendManifest for integration testing
this is similar to cli command exampled below
akash provider send-manifest --owner <address> \
	--dseq 7 \
	--provider <address> ./../_run/kube/deployment.yaml \
	--home=/tmp/akash_integration_TestE2EApp_324892307/.akashctl --node=tcp://0.0.0.0:41863
*/
func TestSendManifest(clientCtx client.Context, id mtypes.BidID, sdlPath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--dseq=%v", id.DSeq),
		fmt.Sprintf("--provider=%s", id.Provider),
	}
	args = append(args, sdlPath)
	args = append(args, extraArgs...)
	fmt.Printf("%v\n", args)

	takeCmdLock()
	cobraCmd := pcmd.SendManifestCmd()
	releaseCmdLock()

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cobraCmd, args...)
}

func TestLeaseShell(clientCtx client.Context, extraArgs []string, lID mtypes.LeaseID, replicaIndex int, tty bool, stdin bool, serviceName string, cmd ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--provider=%s", lID.Provider),
		fmt.Sprintf("--replica-index=%d", replicaIndex),
		fmt.Sprintf("--dseq=%v", lID.DSeq),
		fmt.Sprintf("--gseq=%v", lID.GSeq),
	}
	if tty {
		args = append(args, "--tty")
	}
	if stdin {
		args = append(args, "--stdin")
	}
	args = append(args, extraArgs...)
	args = append(args, serviceName)
	args = append(args, cmd...)
	fmt.Printf("%v\n", args)

	takeCmdLock()
	cobraCmd := pcmd.LeaseShellCmd()
	releaseCmdLock()

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cobraCmd, args...)
}

func TestMigrateHostname(clientCtx client.Context, leaseID mtypes.LeaseID, dseq uint64, hostname string, cmd ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--provider=%s", leaseID.Provider),
		fmt.Sprintf("--dseq=%v", dseq),
	}
	args = append(args, cmd...)
	args = append(args, hostname)
	fmt.Printf("%v\n", args)

	takeCmdLock()
	cobraCmd := pcmd.MigrateHostnamesCmd()
	releaseCmdLock()

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cobraCmd, args...)
}

func TestJwtServerAuthenticate(clientCtx client.Context, provider, from string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--provider=%s", provider),
		fmt.Sprintf("--from=%s", from),
	}

	takeCmdLock()
	cobraCmd := pcmd.JwtServerAuthenticateCmd()
	releaseCmdLock()

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cobraCmd, args...)
}

// RunLocalProvider wraps up the Provider cobra command for testing and supplies
// new default values to the flags.
// prev: akashctl provider run --from=foo --cluster-k8s --gateway-listen-address=localhost:39729 --home=/tmp/akash_integration_TestE2EApp_324892307/.akashctl --node=tcp://0.0.0.0:41863 --keyring-backend test
func RunLocalProvider(ctx context.Context, clientCtx cosmosclient.Context, chainID, nodeRPC, akashHome, from, gatewayListenAddress, jwtGatewayListenAddress string, extraArgs ...string) (sdktest.BufferWriter,
	error) {
	takeCmdLock()
	cmd := pcmd.RunCmd()
	releaseCmdLock()
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
		fmt.Sprintf("--%s=%s", pcmd.FlagJWTGatewayListenAddress, jwtGatewayListenAddress),
		fmt.Sprintf("--%s=%s", flags.FlagKeyringBackend, keyring.BackendTest),
		fmt.Sprintf("--%s=%s", pcmd.FlagClusterPublicHostname, TestClusterPublicHostname),
		fmt.Sprintf("--%s=%d", pcmd.FlagClusterNodePortQuantity, TestClusterNodePortQuantity),
		fmt.Sprintf("--%s=%s", pcmd.FlagBidPricingStrategy, "randomRange"),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmd, args...)
}

func RunLocalHostnameOperator(ctx context.Context, clientCtx cosmosclient.Context) (sdktest.BufferWriter, error) {
	takeCmdLock()
	cmd := pcmd.HostnameOperatorCmd()
	releaseCmdLock()
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmd)
}
