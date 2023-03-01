package cli

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"

	testutilcli "github.com/akash-network/node/testutil/cli"
)

// TxGenerateServerExec is used for testing create server certificate tx
func TxGenerateServerExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, host string, extraArgs ...string) (sdktest.BufferWriter, error) {
	var args []string

	if len(host) != 0 { // for testing purposes, of passing no arguments
		args = []string{host}
	}
	args = append(args, fmt.Sprintf("--from=%s", from.String()))
	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdGenerateServer(), args...)
}

// TxGenerateClientExec is used for testing create client certificate tx
func TxGenerateClientExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdGenerateClient(), args...)
}

// TxPublishServerExec is used for testing create server certificate tx
func TxPublishServerExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdPublishServer(), args...)
}

// TxPublishClientExec is used for testing create client certificate tx
func TxPublishClientExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdPublishClient(), args...)
}

// TxRevokeServerExec is used for testing create server certificate tx
func TxRevokeServerExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdRevokeServer(), args...)
}

// TxRevokeClientExec is used for testing create client certificate tx
func TxRevokeClientExec(ctx context.Context, clientCtx client.Context, from fmt.Stringer, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
	}

	args = append(args, extraArgs...)
	return testutilcli.ExecTestCLICmd(ctx, clientCtx, cmdRevokeClient(), args...)
}

// QueryCertificatesExec is used for testing certificates query
func QueryCertificatesExec(clientCtx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdGetCertificates(), extraArgs...)
}

// QueryCertificateExec is used for testing certificate query
func QueryCertificateExec(clientCtx client.Context, owner string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", owner),
	}

	args = append(args, extraArgs...)

	return testutilcli.ExecTestCLICmd(context.Background(), clientCtx, cmdGetCertificates(), args...)
}
