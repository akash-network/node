package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	"github.com/ovrclk/akash/x/deployment/types"
	tmcli "github.com/tendermint/tendermint/libs/cli"
)

const key string = types.StoreKey

// TxCreateDeploymentExec is used for testing create deployment tx
func TxCreateDeploymentExec(clientCtx client.Context, from fmt.Stringer, filePath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		filePath,
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdCreate(key), args)
}

// TxUpdateDeploymentExec is used for testing update deployment tx
func TxUpdateDeploymentExec(clientCtx client.Context, from fmt.Stringer, filePath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--%s=%s", flags.FlagKeyringBackend, keyring.BackendTest),
		fmt.Sprintf("--from=%s", from.String()),
		filePath,
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdUpdate(key), args)
}

// TxCloseDeploymentExec is used for testing close deployment tx
func TxCloseDeploymentExec(clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--%s=%s", flags.FlagKeyringBackend, keyring.BackendTest),
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdClose(key), args)
}

// QueryDeploymentsExec is used for testing deployments query
func QueryDeploymentsExec(clientCtx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--%s=json", tmcli.OutputFlag),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdDeployments(), args)
}

// QueryDeploymentExec is used for testing deployment query
func QueryDeploymentExec(clientCtx client.Context, id types.DeploymentID, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", id.Owner.String()),
		fmt.Sprintf("--dseq=%v", id.DSeq),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdDeployment(), args)
}

// QueryGroupExec is used for testing group query
func QueryGroupExec(clientCtx client.Context, id types.GroupID, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", id.Owner.String()),
		fmt.Sprintf("--dseq=%v", id.DSeq),
		fmt.Sprintf("--gseq=%v", id.GSeq),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdGetGroup(), args)
}
