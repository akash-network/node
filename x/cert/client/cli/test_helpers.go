package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
)

// TxCreateServerExec is used for testing create server certificate tx
func TxCreateServerExec(clientCtx client.Context, from fmt.Stringer, host string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		host,
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdCreateServer(), args)
}

// TxCreateClientExec is used for testing create client certificate tx
func TxCreateClientExec(clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdCreateClient(), args)
}
