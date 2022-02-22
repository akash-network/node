package cli

import (
	"context"
	"fmt"
	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	testutilcli "github.com/ovrclk/akash/testutil/cli"
)

// TxCreateServerExec is used for testing create server certificate tx
func TxGenerateServerExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, host string, extraArgs ...string) (sdktest.BufferWriter, error) {
	var args []string

	if len(host) != 0 { // for testing purposes, of passing no arguments
		args = []string{host}
	}
	args = append(args, fmt.Sprintf("--from=%s", from.String()))
	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdGenerateServer(), args...)
}

// TxCreateClientExec is used for testing create client certificate tx
func TxGenerateClientExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdGenerateClient(), args...)
}

// TxCreateServerExec is used for testing create server certificate tx
func TxPublishServerExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdPublishServer(), args...)
}

// TxCreateClientExec is used for testing create client certificate tx
func TxPublishClientExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdPublishClient(), args...)
}

// TxCreateServerExec is used for testing create server certificate tx
func TxRevokeServerExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdRevokeServer(), args...)
}

// TxCreateClientExec is used for testing create client certificate tx
func TxRevokeClientExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdRevokeClient(), args...)
}
