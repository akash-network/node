package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"

	testutilcli "github.com/ovrclk/akash/testutil/cli"
	"github.com/ovrclk/akash/x/deployment/types"
)

const key string = types.StoreKey

// TxCreateDeploymentExec is used for testing create deployment tx
func TxCreateDeploymentExec(clientCtx client.Context, from fmt.Stringer, filePath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		filePath,
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(clientCtx, cmdCreate(key), args...)
}

// TxUpdateDeploymentExec is used for testing update deployment tx
func TxUpdateDeploymentExec(clientCtx client.Context, from fmt.Stringer, filePath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		filePath,
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(clientCtx, cmdUpdate(key), args...)
}

// TxCloseDeploymentExec is used for testing close deployment tx
// requires --dseq, --fees
func TxCloseDeploymentExec(clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(clientCtx, cmdClose(key), args...)
}

// TxCloseGroupExec is used for testing close group tx
func TxCloseGroupExec(clientCtx client.Context, groupID types.GroupID, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		fmt.Sprintf("--owner=%s", groupID.Owner),
		fmt.Sprintf("--dseq=%v", groupID.DSeq),
		fmt.Sprintf("--gseq=%v", groupID.GSeq),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(clientCtx, cmdGroupClose(key), args...)
}

// QueryDeploymentsExec is used for testing deployments query
func QueryDeploymentsExec(clientCtx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return testutilcli.ExecTestCLICmd(clientCtx, cmdDeployments(), extraArgs...)
}

// QueryDeploymentExec is used for testing deployment query
func QueryDeploymentExec(clientCtx client.Context, id types.DeploymentID, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", id.Owner),
		fmt.Sprintf("--dseq=%v", id.DSeq),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(clientCtx, cmdDeployment(), args...)
}

// QueryGroupExec is used for testing group query
func QueryGroupExec(clientCtx client.Context, id types.GroupID, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", id.Owner),
		fmt.Sprintf("--dseq=%v", id.DSeq),
		fmt.Sprintf("--gseq=%v", id.GSeq),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(clientCtx, cmdGetGroup(), args...)
}
