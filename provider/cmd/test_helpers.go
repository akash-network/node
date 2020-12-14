package cmd

import (
	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	testutilcli "github.com/ovrclk/akash/testutil/cli"
)

func ProviderLeaseStatusExec(clientCtx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return testutilcli.ExecTestCLICmd(clientCtx, leaseStatusCmd(), extraArgs...)
}

func ProviderServiceStatusExec(clientCtx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return testutilcli.ExecTestCLICmd(clientCtx, serviceStatusCmd(), extraArgs...)
}

func ProviderStatusExec(clientCtx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return testutilcli.ExecTestCLICmd(clientCtx, statusCmd(), extraArgs...)
}

func ProviderServiceLogs(clientCtx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return testutilcli.ExecTestCLICmd(clientCtx, serviceLogsCmd(), extraArgs...)
}
