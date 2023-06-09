package cli

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	testutilcli "github.com/akash-network/node/testutil/cli"
)

const key string = types.StoreKey

// XXX: WHY TF DON'T THESE RETURN OBJECTS

// TxCreateDeploymentExec is used for testing create deployment tx
func TxCreateDeploymentExec(clientCtx client.Context, from fmt.Stringer, filePath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		filePath,
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdCreate(key), args...)
}

// TxUpdateDeploymentExec is used for testing update deployment tx
func TxUpdateDeploymentExec(clientCtx client.Context, from fmt.Stringer, filePath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		filePath,
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdUpdate(key), args...)
}

// TxCloseDeploymentExec is used for testing close deployment tx
// requires --dseq, --fees
func TxCloseDeploymentExec(clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdClose(key), args...)
}

// TxDepositDeploymentExec is used for testing deposit deployment tx
func TxDepositDeploymentExec(clientCtx client.Context, deposit sdk.Coin, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		deposit.String(),
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdDeposit(key), args...)
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

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdGroupClose(key), args...)
}

// QueryDeploymentsExec is used for testing deployments query
func QueryDeploymentsExec(clientCtx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdDeployments(), extraArgs...)
}

// QueryDeploymentExec is used for testing deployment query
func QueryDeploymentExec(clientCtx client.Context, id types.DeploymentID, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", id.Owner),
		fmt.Sprintf("--dseq=%v", id.DSeq),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdDeployment(), args...)
}

// QueryGroupExec is used for testing group query
func QueryGroupExec(clientCtx client.Context, id types.GroupID, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", id.Owner),
		fmt.Sprintf("--dseq=%v", id.DSeq),
		fmt.Sprintf("--gseq=%v", id.GSeq),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdGetGroup(), args...)
}

func TxGrantAuthorizationExec(clientCtx client.Context, granter, grantee sdk.AccAddress, extraArgs ...string) (sdktest.BufferWriter, error) {

	dmin, _ := types.DefaultParams().MinDepositFor("uakt")

	spendLimit := sdk.NewCoin(dmin.Denom, dmin.Amount.MulRaw(3))
	args := []string{
		grantee.String(),
		spendLimit.String(),
		fmt.Sprintf("--from=%s", granter.String()),
	}
	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdGrantAuthorization(), args)
}

func TxRevokeAuthorizationExec(clientCtx client.Context, granter, grantee sdk.AccAddress, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		grantee.String(),
		fmt.Sprintf("--from=%s", granter.String()),
	}
	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdRevokeAuthorization(), args)
}
