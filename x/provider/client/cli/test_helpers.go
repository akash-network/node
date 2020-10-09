package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	"github.com/ovrclk/akash/x/provider/types"
)

const key string = types.StoreKey

// TxCreateProviderExec is used for testing create provider tx
func TxCreateProviderExec(clientCtx client.Context, from fmt.Stringer, filepath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		filepath,
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdCreate(key), args)
}

// TxUpdateProviderExec is used for testing update provider tx
func TxUpdateProviderExec(clientCtx client.Context, from fmt.Stringer, filepath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		filepath,
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdUpdate(key), args)
}

// QueryProvidersExec is used for testing providers query
func QueryProvidersExec(clientCtx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return clitestutil.ExecTestCLICmd(clientCtx, cmdGetProviders(), args)
}

// QueryProviderExec is used for testing provider query
func QueryProviderExec(clientCtx client.Context, owner string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		owner,
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdGetProvider(), args)
}
