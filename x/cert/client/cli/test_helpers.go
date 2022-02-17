package cli

import (
	"fmt"
	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
)

// TxCreateServerExec is used for testing create server certificate tx
func TxGenerateServerExec(clientCtx client.Context, from fmt.Stringer, host string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		host,
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return clitestutil.ExecTestCLICmd(clientCtx, cmdGenerateServer(), args)
}

// TxCreateClientExec is used for testing create client certificate tx
func TxGenerateClientExec(clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return clitestutil.ExecTestCLICmd(clientCtx, cmdGenerateClient(), args)
}

// TxCreateServerExec is used for testing create server certificate tx
func TxPublishServerExec(clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return clitestutil.ExecTestCLICmd(clientCtx, cmdPublishServer(), args)
}

// TxCreateClientExec is used for testing create client certificate tx
func TxPublishClientExec(clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return clitestutil.ExecTestCLICmd(clientCtx, cmdPublishClient(), args)
}
