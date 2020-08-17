package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	"github.com/ovrclk/akash/x/deployment/types"
)

const key string = types.StoreKey

// TxCreateDeployment is used for testing create deployment tx
func TxCreateDeployment(clientCtx client.Context, from fmt.Stringer, filePath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--%s=%s", flags.FlagKeyringBackend, keyring.BackendTest),
		fmt.Sprintf("--from=%s", from.String()),
		filePath,
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdCreate(key), args)
}

// TxUpdateDeployment is used for testing update deployment tx
func TxUpdateDeployment(clientCtx client.Context, from fmt.Stringer, filePath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--%s=%s", flags.FlagKeyringBackend, keyring.BackendTest),
		fmt.Sprintf("--from=%s", from.String()),
		filePath,
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdUpdate(key), args)
}

// TxCloseDeployment is used for testing close deployment tx
func TxCloseDeployment(clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--%s=%s", flags.FlagKeyringBackend, keyring.BackendTest),
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdClose(key), args)
}
